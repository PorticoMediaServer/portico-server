import {DecodeAudioRender} from './DecodeAudioRender';
import {channelMediaTime,channelSourceTime} from '@core/playback/channel-clock';
import { webPlaybackResourceKey, webMediaCompleted } from './web-playback-binding';
import { mapAudioTracks } from './player-audio-mapping';
import type { AudioBinding } from '@core/audio-selection';
import { useEffect, type RefObject } from 'react';
import type Hls from 'hls.js';
import type { HttpLocalApi, PlaybackService, PlaybackSnapshot } from '@core/index.ts';
/** Keep seek requests separate from observed media time, including ungenerated HLS ranges. */
export function useWebPlaybackAdapter(
  service: PlaybackService,
  api: HttpLocalApi,
  video: RefObject<HTMLVideoElement | null>,
  rate: RefObject<number>,
  notice: (message: string) => void,
  resourceKey: string
) {
  useEffect(() => {
    const node = video.current;
    if (!node) return;
    const el: HTMLVideoElement = node;
    let render:DecodeAudioRender|undefined;
    let active = true,
      sessionId = '',
      loadRevision = 0,
      hls: Hls | undefined,
      pending:
        | {
            intentId: number;
            sessionId: string;
            revision: number;
            position: number;
            assigned: boolean;
          }
        | undefined;
    let routeBlocked = false, networkFailures = 0, meaningfulPosition = 0, mediaRecoveries = 0, lastProgressAt = Date.now(), watchdogPosition = -1;
    let routeResume: {position:number;assigned:boolean;startedAt?:number} | undefined;
    const current = () => service.getSnapshot();
    const ownsResource = () => active && video.current === el && webPlaybackResourceKey(current()) === resourceKey;
    let audioBinding: AudioBinding | undefined,
      audioEpoch: number | undefined,
      audioSequence = 0,
      audioCommand = 0,
      audioUrl = '',
      nativeAudioUnavailable = false,
      lastSwitched: { groupId: string; name: string; url: string } | undefined;
    function applyAudio(next: PlaybackSnapshot) {
      const a = next.audio,
        engine = hls;
      if (!a.plan || !a.binding) return;
      if (!engine) {
        if (nativeAudioUnavailable && audioBinding !== a.binding) {
          audioBinding = a.binding;
          nativeAudioUnavailable = false;
          const epoch = service.attachAudioEngine(a.binding);
          if (epoch !== null) service.audioMapped(a.binding, epoch, []);
          nativeAudioUnavailable = true;
        }
        return;
      }
      if (audioBinding !== a.binding) {
        audioBinding = a.binding;
        audioCommand = 0;
        audioSequence = 0;
        audioEpoch = service.attachAudioEngine(a.binding) ?? undefined;
        queueMicrotask(() => {
          if (active && audioBinding === a.binding) {
            applyAudio(current());
            observeAudio();
          }
        });
        return;
      }
      if (audioEpoch === undefined) return;
      const mapping = mapAudioTracks(a.plan, engine.audioTracks, audioUrl);
      if (!engine.audioTracks.length) {
        if (a.availability === 'available') service.audioMapped(a.binding, audioEpoch, []);
        return;
      }
      if (!mapping) {
        if (a.availability !== 'unavailable') service.audioMapped(a.binding, audioEpoch, []);
        return;
      }
      service.audioMapped(a.binding, audioEpoch, [...mapping.keys()]);
      const pending = service.getSnapshot().audio.pending;
      if (!pending || pending.revision === audioCommand) return;
      audioCommand = pending.revision;
      const index = mapping.get(pending.renditionId);
      if (index === undefined) return;
      if (
        lastSwitched &&
        engine.audioTrack === index &&
        lastSwitched.groupId === engine.audioTracks[index].groupId &&
        lastSwitched.name === engine.audioTracks[index].name &&
        lastSwitched.url === engine.audioTracks[index].url
      ) {
        observeAudio();
        return;
      }
      try {
        engine.audioTrack = index;
      } catch {
        service.audioFailed(
          a.binding,
          audioEpoch,
          pending.revision,
          'This audio track could not be selected. Try again.'
        );
      }
    }
    function observeAudio() {
      const a = current().audio,
        engine = hls;
      if (
        !a.plan ||
        !a.binding ||
        !engine ||
        audioEpoch === undefined ||
        audioBinding !== a.binding ||
        !lastSwitched
      )
        return;
      const mapping = mapAudioTracks(a.plan, engine.audioTracks, audioUrl);
      if (!mapping) return;
      const track = engine.audioTracks[engine.audioTrack];
      if (
        !track ||
        track.groupId !== lastSwitched.groupId ||
        track.name !== lastSwitched.name ||
        track.url !== lastSwitched.url
      )
        return;
      const id = [...mapping].find(([, index]) => index === engine.audioTrack)?.[0];
      if (!id) return;
      const pending = a.pending;
      service.audioObserved(a.binding, audioEpoch, ++audioSequence, id);
      if (pending?.renditionId === id)
        service.audioApplied(a.binding, audioEpoch, pending.revision, id);
    }

    function destroyHls() {
      const previous = hls,
        binding = audioBinding,
        epoch = audioEpoch;
      hls = undefined;
      audioBinding = undefined;
      audioEpoch = undefined;
      lastSwitched = undefined;
      if (binding && epoch !== undefined) service.detachAudioEngine(binding, epoch);
      previous?.destroy();
    }
    function loadMedia(next: PlaybackSnapshot) {
      const session = next.session!;
      if (routeBlocked) return;
      sessionId = session.id;
      const revision = ++loadRevision,
        intentId = next.intentId,
        url = api.mediaUrl(session.streamUrl);
      destroyHls();
      nativeAudioUnavailable = false;
      el.dataset.porticoSubtitleOffset='0';
      el.dispatchEvent(new Event('portico-subtitle-timeline'));
      el.pause();
      el.removeAttribute('src');
      el.load();
      const valid = () =>
        ownsResource() &&
        loadRevision === revision &&
        current().intentId === intentId &&
        current().session?.id === session.id &&
        current().session?.generation === session.generation &&
        current().phase !== 'error';
      if (session.mode !== 'hls') {
        el.src = url;
        el.load();
        return;
      }
      void import('hls.js')
        .then(({ default: Engine }) => {
          if (!valid()) return;
          if (!Engine.isSupported()) {
            nativeAudioUnavailable = true;
            applyAudio(current());
            if (el.canPlayType('application/vnd.apple.mpegurl')) {
              el.src = url;
              el.load();
            } else service.fail(intentId, 'This browser cannot play converted streams yet.');
            return;
          }
          const origin = new URL(url).origin;
          const policy = {
            default: {
              maxTimeToFirstByteMs: 8000,
              maxLoadTimeMs: 15000,
              timeoutRetry: { maxNumRetry: 2, retryDelayMs: 500, maxRetryDelayMs: 2000 },
              errorRetry: { maxNumRetry: 2, retryDelayMs: 500, maxRetryDelayMs: 2000 },
            },
          };
          const memory = (navigator as Navigator & {deviceMemory?: number}).deviceMemory;
          const lowMemory = typeof memory === 'number' && memory <= 2;
          const engine = new Engine({
            debug: false,
            enableWorker: true,
            lowLatencyMode: false,
            startPosition: routeResume&&!next.linear?Math.max(0,routeResume.position):0,
            // PERF-26: smaller buffers on low-memory devices (TV sticks), and never fetch a
            // rendition larger than the player shows.
            maxBufferLength: lowMemory ? 20 : 30,
            maxMaxBufferLength: lowMemory ? 30 : 60,
            maxBufferSize: (lowMemory ? 16 : 32) * 1024 * 1024,
            backBufferLength: lowMemory ? 8 : 30,
            capLevelToPlayerSize: true,
            manifestLoadPolicy: policy,
            playlistLoadPolicy: policy,
            fragLoadPolicy: policy,
            xhrSetup: (xhr, requestUrl) => {
              if (new URL(requestUrl).origin !== origin)
                throw new Error('Unexpected stream origin');
              xhr.withCredentials = false;
            },
          });
          hls = engine;
          // Do not display source-relative cues until hls.js supplies its PTS mapping.
          el.dataset.porticoSubtitleOffset='NaN';
          el.dispatchEvent(new Event('portico-subtitle-timeline'));
          engine.subtitleDisplay = false;
          engine.on(Engine.Events.SUBTITLE_TRACKS_UPDATED, () => { if(valid()) engine.subtitleTrack=-1; });
          engine.on(Engine.Events.FRAG_CHANGED, (_event, data) => {
            if(!valid()) return;
            // hls.js exposes the presentation/playlist mapping for the active
            // fragment. Shift subtitle cues, never the player's source clock.
            const pts=data.frag.startPTS, start=data.frag.start;
            if(typeof pts==='number' && Number.isFinite(pts) && Number.isFinite(start)) {
              const offset=String(pts-start);
              if(el.dataset.porticoSubtitleOffset!==offset) {
                el.dataset.porticoSubtitleOffset=offset;
                el.dispatchEvent(new Event('portico-subtitle-timeline'));
              }
            }
          });
          audioUrl = url;
          engine.on(Engine.Events.AUDIO_TRACKS_UPDATED, () => {
            if (valid() && hls === engine) {
              applyAudio(current());
              observeAudio();
            }
          });
          engine.on(Engine.Events.AUDIO_TRACK_SWITCHED, (_event, data) => {
            if (!valid() || hls !== engine) return;
            lastSwitched = { groupId: data.groupId, name: data.name, url: data.url };
            applyAudio(current());
            observeAudio();
          });
          engine.on(Engine.Events.ERROR, (_event, data) => {
            if (!valid() || hls !== engine) return;
            let preparing=false,failedAudio=false;try{const body=typeof data.response?.data==='string'?JSON.parse(data.response.data):data.response?.data;preparing=data.response?.code===503&&body?.error?.code==='segment_preparing';failedAudio=data.response?.code===422&&body?.error?.code==='audio_rendition_unavailable';}catch{}
            if(failedAudio){const a=current().audio;const mapping=a.plan?mapAudioTracks(a.plan,engine.audioTracks,audioUrl):null;const fallback=a.plan?mapping?.get(a.plan.defaultRenditionId):undefined;if(a.binding&&audioEpoch!==undefined&&a.pending)service.audioFailed(a.binding,audioEpoch,a.pending.revision,'This track is unavailable. Playing the default audio.',true);if(fallback!==undefined){engine.audioTrack=fallback;engine.startLoad(current().positionSeconds);notice('This audio track is unavailable. Playing the default audio.');return;}}
            if(preparing){recover(()=>{if(hls===engine)engine.startLoad(current().pendingSeek?.positionSeconds??current().positionSeconds);},'This segment is still being prepared. Your playback position is retained.');return;}
            if (!data.fatal) return;
            if (data.type === Engine.ErrorTypes.NETWORK_ERROR) {
              // The server answering "no" is a fact; the server not answering is weather.
              const status = data.response?.code;
              if (status === 401 || status === 403 || status === 404 || status === 410) {
                destroyHls();
                service.fail(intentId, 'This stream is no longer available. Your playback position is retained.');
                return;
              }
              recover(() => { if (hls === engine) engine.startLoad(); }, 'The server could not be reached. Your playback position is retained.');
              return;
            }
            if (data.type === Engine.ErrorTypes.MEDIA_ERROR && mediaRecoveries < 2) {
              // The second attempt also swaps the audio codec hint, which is what rescues a
              // stream whose AAC profile the browser's demuxer guessed wrong.
              if (mediaRecoveries++ === 1) engine.swapAudioCodec();
              engine.recoverMediaError();
              return;
            }
            // The engine cannot play what it was given. That is not yet the viewer's problem:
            // the server is told, and the title comes back through the next route up.
            destroyHls();
            service.engineFailed(intentId, data.type === Engine.ErrorTypes.MEDIA_ERROR ? 'decode_error' : 'engine_error', String(data.details ?? '').slice(0, 120), 'This stream could not be played. Try again.');
          });
          engine.attachMedia(el);
          engine.loadSource(url);
        })
        .catch(() => {
          if (valid()) service.fail(intentId, 'The streaming player could not load. Try again.');
        });
    }
    function observedPosition():number|null {
      const linear=current().linear;if(!linear)return el.currentTime;
      let date:number|null=null;
      if(hls)date=hls.playingDate?.getTime()??null;
      else {const start=(el as HTMLVideoElement&{getStartDate?:()=>Date}).getStartDate?.().getTime();if(start!==undefined&&Number.isFinite(start))date=start+el.currentTime*1000;}
      return channelSourceTime(linear.media.originMs,date);
    }
    function mediaPosition(position:number):number|null {
      const linear=current().linear;if(!linear)return position;
      if(hls){const level=hls.levels[hls.currentLevel]??hls.levels[hls.loadLevel]??hls.levels[0];return channelMediaTime(linear.media.originMs,position,level?.details?.fragments??[]);}
      const start=(el as HTMLVideoElement&{getStartDate?:()=>Date}).getStartDate?.().getTime();
      return start!==undefined&&Number.isFinite(start)?(linear.media.originMs+position*1000-start)/1000:null;
    }
    function channelFact(){
      const s=current();if(routeBlocked||routeResume||!ownsResource()||!s.linear||el.seeking||s.pendingSeek||s.failedSeek)return;
      const position=observedPosition();if(position!==null)service.fact(s.intentId,position,el.ended?'ended':el.paused?'paused':'playing');
    }
    function matches() {
      const s = current();
      return (
        ownsResource() &&
        pending &&
        s.intentId === pending.intentId &&
        s.session?.id === pending.sessionId &&
        s.pendingSeek?.revision === pending.revision
      );
    }
    function finish() {
      if (!matches() || !pending?.assigned || el.seeking || el.readyState < 2) return;
      const request = pending;
      const observed=observedPosition();if(observed===null||Math.abs(observed-request.position)>.5)return;
      service.seekApplied(
        request.intentId,
        request.revision,
        observed,
        el.paused ? 'paused' : 'playing'
      );
    }
    function attempt() {
      if (!matches() || !pending || pending.assigned || el.readyState < 1) return;
      const request = pending;
      const target=mediaPosition(request.position);if(target===null)return;
      if (
        !el.seeking &&
        el.readyState >= 2 &&
        Math.abs((observedPosition()??Infinity) - request.position) <= 0.05
      ) {
        request.assigned = true;
        finish();
        return;
      }
      let available = current().session?.mode === 'direct';
      for (let i = 0; i < el.seekable.length; i++)
        if (target >= el.seekable.start(i) && target <= el.seekable.end(i)) {
          available = true;
          break;
        }
      if (!available) return;
      request.assigned = true;
      try {
        el.currentTime = target;
      } catch {
        service.seekFailed(
          request.intentId,
          request.revision,
          'That position could not be reached. Try seeking again.'
        );
        return;
      }
      queueMicrotask(finish);
    }
    function apply(next: PlaybackSnapshot) {
      if (!active) return;
      // Ownership/error fences precede the transport-resume branch. A delayed
      // media event cannot revive a failed or replaced presentation.
      if(!ownsResource()||!next.session||next.phase==='error'){
        pending=undefined;routeResume=undefined;render?.dispose();render=undefined;
        if(sessionId){sessionId='';loadRevision++;destroyHls();el.pause();el.removeAttribute('src');el.load();}
        return;
      }
      if (routeBlocked) { render?.dispose();render=undefined;el.pause(); return; }
      if (routeResume && next.pendingSeek) { routeResume=undefined; el.dataset.porticoRoute='ready'; }
      if (routeResume&&!next.audioEffects.rendering) { resumeRoutePosition(); return; }
      // Service publication can precede React replacing the keyed DOM node. Fence the old
      // resource immediately; only the next mounted adapter may load the new presentation.
      if (webPlaybackResourceKey(next) !== resourceKey || video.current !== el) {
        render?.dispose();render=undefined;
        pending = undefined;
        if (sessionId) { sessionId='';loadRevision++;destroyHls();el.pause();el.removeAttribute('src');el.load(); }
        return;
      }
      if (!next.session) {
        render?.dispose();render=undefined;
        pending = undefined;
        if (sessionId) {
          sessionId = '';
          loadRevision++;
          destroyHls();
          el.pause();
          el.removeAttribute('src');
          el.load();
        }
        return;
      }
      if(next.audioEffects.rendering){
        if(!render){destroyHls();el.pause();el.removeAttribute('src');el.load();try{render=new DecodeAudioRender(service,path=>api.mediaUrl(path),el,(current,next)=>!!current&&!!next&&(service.getQueueController()?.sameAlbum?.(current,next)??false));}catch{service.audioRenderFailed('Web Audio processing is not supported by this browser.');return;}}
        const retained=routeResume?.position;routeResume=undefined;el.dataset.porticoRoute='ready';render.apply(retained===undefined?next:{...next,positionSeconds:retained});return;
      }
      if(render){render.dispose();render=undefined;sessionId='';}
      if (sessionId !== next.session.id) {
        pending = undefined;
        loadMedia(next);
      }
      applyAudio(next);
      if (next.pendingSeek) {
        if (
          !pending ||
          pending.revision !== next.pendingSeek.revision ||
          pending.intentId !== next.intentId ||
          pending.sessionId !== sessionId
        )
          pending = {
            intentId: next.intentId,
            sessionId,
            revision: next.pendingSeek.revision,
            position: next.pendingSeek.positionSeconds,
            assigned: false,
          };
        el.pause();
        attempt();
        return;
      }
      pending = undefined;
      if (next.phase==='ended') { el.pause(); return; }
      if (!next.channel && webMediaCompleted(el)) {
        service.fact(next.intentId,el.currentTime,'ended');
        return;
      }
      if (el.readyState < 1) return;
      el.playbackRate = rate.current;
      if (next.intent === 'paused'||next.failedSeek||next.seekError) el.pause();
      else if (el.paused) {
        const id = next.intentId, revision=loadRevision;
        void el.play().catch(() => {
          if (ownsResource() && loadRevision===revision && !routeBlocked && !routeResume && current().intentId === id) {
            notice('Press play to start playback.');
            service.pause();
          }
        });
      }
    }
    function metadata() {
      if (!ownsResource() || routeBlocked || el.readyState<1 || !current().session || current().phase === 'error') return;
      if(routeResume){resumeRoutePosition();return;}
      service.ready(current().intentId);
      attempt();
    }
    function progress() {
      if(Math.abs(el.currentTime-watchdogPosition)>.05){watchdogPosition=el.currentTime;lastProgressAt=Date.now();}
      if(el.paused||el.seeking||current().intent==='paused')lastProgressAt=Date.now();
      if(!routeBlocked&&!routeResume&&!el.paused&&el.readyState<3&&Date.now()-lastProgressAt>8000){lastProgressAt=Date.now();recover(()=>{if(hls)hls.startLoad(current().pendingSeek?.positionSeconds??current().positionSeconds);else reloadAtPosition();},'Playback could not resume. Your position is retained.');}

      if(routeBlocked)return;
      if(routeResume){resumeRoutePosition();return;}
      const position=observedPosition();
      // Progress is movement, not a record: after a seek backwards the old maximum would
      // otherwise hold every recovery open until playback passed it again.
      if(position!==null&&Math.abs(position-meaningfulPosition)>1&&!el.seeking){meaningfulPosition=position;networkFailures=0;mediaRecoveries=0;if(recovery&&bufferedAhead()>2)recovered();}
      attempt();
      finish();
    }
    function sought() {
      if(routeResume){resumeRoutePosition();return;}
      if (!matches() || !pending?.assigned || el.seeking) return;
      if (Math.abs((observedPosition()??Infinity) - pending.position) > 0.5) {
        const request = pending;
        service.seekFailed(
          request.intentId,
          request.revision,
          'That position could not be reached. Try seeking again.'
        );
      } else finish();
    }
    function resumeRoutePosition() {
      const resume=routeResume;if(!resume||routeBlocked||!ownsResource()||current().phase==='error')return;
      if(resume.startedAt!==undefined&&Date.now()-resume.startedAt>10000){
        // Ten seconds without the media coming back is a reason to try again, not to give up.
        const position=resume.position;routeResume=undefined;
        recover(()=>{if(routeResume)return;const n=current();if(!n.session)return;routeResume={position,startedAt:Date.now()} as NonNullable<typeof routeResume>;sessionId='';loadMedia(n);apply(n);},'The server could not be reached. Your playback position is retained.');return;
      }
      if(el.readyState<1)return;
      if(current().pendingSeek){routeResume=undefined;el.dataset.porticoRoute='ready';apply(current());return;}
      const target=mediaPosition(resume.position);if(target===null||!Number.isFinite(target))return;
      const observed=observedPosition();
      if(!el.seeking&&el.readyState>=2&&observed!==null&&Math.abs(observed-resume.position)<=.5){
        routeResume=undefined;el.dataset.porticoRoute='ready';service.ready(current().intentId);apply(current());return;
      }
      if(resume.assigned)return;
      let available=current().session?.mode==='direct';for(let i=0;i<el.seekable.length;i++)if(target>=el.seekable.start(i)&&target<=el.seekable.end(i))available=true;
      if(!available)return;
      try{resume.assigned=true;el.currentTime=target;}catch{resume.assigned=false;}
    }
    function routeChanged(){
      const route=api.getRouteSnapshot();if(!route||!ownsResource())return;
      if(route.phase!=='ready'){
        if(!routeBlocked){const s=current(),position=render?.transportPosition()??observedPosition();routeResume={position:s.pendingSeek?.positionSeconds??(position!==null&&Number.isFinite(position)?position:s.positionSeconds),assigned:false};}
        routeBlocked=true;render?.dispose();render=undefined;if(current().audioEffects.prepared)service.audioPreparationFailed('Network changed; the prepared audio edge was cancelled.');el.dataset.porticoRoute='verifying';loadRevision++;destroyHls();el.pause();el.removeAttribute('src');el.load();return;
      }
      if(routeBlocked){routeBlocked=false;el.dataset.porticoRoute='rebinding';if(routeResume)routeResume.startedAt=Date.now();const s=current();if(s.session){if(s.pendingSeek){routeResume=undefined;el.dataset.porticoRoute='ready';}if(!s.audioEffects.rendering)loadMedia(s);apply(s);}}
    }
    // A home server sleeps, reboots and sits behind bad Wi-Fi. Media that stops arriving is
    // retried quietly for as long as there is buffered media to play; only when the buffer is
    // empty does the viewer see anything, and what they see is "Reconnecting", not an error.
    // Playback ends only on a definite refusal, or after a long stretch with nothing to play.
    const RECOVERY_GIVE_UP_MS=90000;
    let recovery:{attempts:number;emptySince:number;retry:()=>void;message:string;timer?:ReturnType<typeof setTimeout>}|undefined;
    function bufferedAhead():number {
      if(el.error)return 0;
      const t=el.currentTime;
      for(let i=0;i<el.buffered.length;i++)if(el.buffered.start(i)<=t+.25&&el.buffered.end(i)>t)return el.buffered.end(i)-t;
      return 0;
    }
    function recovered(){
      if(!recovery)return;
      clearTimeout(recovery.timer);recovery=undefined;networkFailures=0;service.recovering(current().intentId,false);
    }
    function recoveryTick(){
      const r=recovery;if(!r)return;
      if(!ownsResource()||current().phase==='error'||!current().session){clearTimeout(r.timer);recovery=undefined;return;}
      if(bufferedAhead()<.5){
        r.emptySince||=Date.now();service.recovering(current().intentId,true);
        if(Date.now()-r.emptySince>RECOVERY_GIVE_UP_MS){clearTimeout(r.timer);recovery=undefined;destroyHls();service.fail(current().intentId,r.message);return;}
      }else{
        // Media is there again. The give-up clock measures one starvation, not the sum of
        // every one in a long film; and a paused player cannot prove recovery by advancing,
        // so a refilled buffer is the proof.
        r.emptySince=0;
        if(bufferedAhead()>2&&(el.paused||!el.error)&&r.attempts>0){recovered();return;}
      }
      if(api.getRouteConnection())void api.recoverRoute().catch(()=>{});
      if(!routeBlocked)r.retry();
      r.attempts++;r.timer=setTimeout(recoveryTick,Math.min(1000*2**r.attempts,10000));
    }
    function recover(retry:()=>void,message:string){
      if(recovery){recovery.retry=retry;recovery.message=message;return;}
      recovery={attempts:0,emptySince:0,retry,message};
      recovery.timer=setTimeout(recoveryTick,1000);
    }
    function reloadAtPosition(){
      const s=current();if(!s.session||routeResume)return;
      // An element that has just been reset reports zero, which is finite and wrong: the
      // service's own settled position is the floor for where playback comes back.
      const observed=observedPosition(),position=observed!==null&&Number.isFinite(observed)&&observed>0.5?observed:Math.max(s.positionSeconds,meaningfulPosition);
      routeResume={position:s.pendingSeek?.positionSeconds??position,startedAt:Date.now()} as NonNullable<typeof routeResume>;
      // apply() returns at the resume branch before it would load anything, so the reload is
      // explicit here, as it is when a route comes back.
      sessionId='';loadMedia(s);apply(s);
    }
    function nativeError(){
      if(!ownsResource()||routeBlocked)return;
      // A browser reports an unreachable server as a network error, or as "source not supported"
      // when the reload itself gets no answer. Media that has already played did not become
      // unsupported, so both mean "try again", quietly, from where playback was.
      const code=el.error?.code;
      if(code===MediaError.MEDIA_ERR_NETWORK||(code===MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED&&(meaningfulPosition>0||recovery))){
        recover(reloadAtPosition,'The server could not be reached. Your playback position is retained.');return;
      }
      // Decode and format errors are the engine refusing this route, not the title being
      // unplayable: report it and let the server choose the next route up.
      service.engineFailed(current().intentId,code===MediaError.MEDIA_ERR_DECODE?'decode_error':'source_not_supported',String(el.error?.message??'').slice(0,120),'This media could not be played. Your playback position is retained.');
    }
    const detachRoute=api.subscribeRoute(routeChanged);
    routeChanged();
    // The moment the buffer runs dry during a recovery is when the viewer is first told.
    function recoveryStarved(){if(recovery&&ownsResource()&&bufferedAhead()<.5){recovery.emptySince||=Date.now();service.recovering(current().intentId,true);}}
    el.addEventListener('error',nativeError);
    el.addEventListener('waiting',recoveryStarved);
    el.addEventListener('timeupdate',channelFact);
    el.addEventListener('pause',channelFact);
    el.addEventListener('ended',channelFact);
    el.addEventListener('loadeddata',progress);
    el.addEventListener('loadedmetadata', metadata);
    el.addEventListener('progress', progress);
    el.addEventListener('durationchange', progress);
    el.addEventListener('canplay', progress);
    el.addEventListener('seeked', sought);
    const retry = setInterval(progress, 250),
      detach = service.attachAdapter({ apply });
    return () => {
      active = false;
      render?.dispose();
      loadRevision++;
      pending = undefined;
      destroyHls();
      detachRoute();
      el.removeEventListener('error',nativeError);
      el.removeEventListener('waiting',recoveryStarved);
      if(recovery){clearTimeout(recovery.timer);recovery=undefined;}
      clearInterval(retry);
      detach();
      el.removeEventListener('timeupdate',channelFact);
      el.removeEventListener('pause',channelFact);
      el.removeEventListener('ended',channelFact);
      el.removeEventListener('loadeddata',progress);
      el.removeEventListener('loadedmetadata', metadata);
      el.removeEventListener('progress', progress);
      el.removeEventListener('durationchange', progress);
      el.removeEventListener('canplay', progress);
      el.removeEventListener('seeked', sought);
      el.pause();
      el.removeAttribute('src');
      el.load();
    };
  }, [api, service, video, rate, notice, resourceKey]);
}
