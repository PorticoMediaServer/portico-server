/*
 * Portico Cast receiver.
 *
 * The page is served by the Portico server it talks to (and, unchanged, at
 * cast.getportico.tv for every server). The receiver framework script from
 * Google and its documented CAF player dependencies are the external resources.
 *
 * Pairing: a sender asks its server for a one-use code (POST /v1/cast/bootstrap)
 * and delivers it over the `urn:x-cast:tv.getportico.cast` channel (a Cast device
 * has no keyboard); this page redeems it (POST /v1/cast/redeem) for two
 * credentials:
 *
 *   deviceToken  device-scoped, kept in localStorage, accepted only by
 *                POST /v1/cast/reconnect, rotated on every reconnect; used here
 *                only to renew an expired access token mid-session
 *   accessToken  an ordinary short-lived viewer bearer
 *
 * Control (SEC-14, CAST-04, D-FEAT-10): the sender that paired is the
 * controller. Only it may load, play, seek, change tracks or read status; any
 * sender may pause or stop (anyone in the room can do that with the remote).
 * A different sender that pairs while something is playing doesn't interrupt
 * it: the TV shows who is asking, the controller is asked, and the TV switches
 * only if the controller allows it, stops, or has left (then after 10 seconds).
 * The same profile on the same server switches straight away. The old
 * pairing is kept until the new one is adopted, and nothing is sent to other
 * senders but "busy".
 *
 * Sender protocol:
 *   pair  {displayName}                         → paired | pair-pending | pair-failed {code: 'tv_busy' | …}
 *   load  {title, subtitle, seriesTitle, season, episode, kind, artworkPath, subtitleTrackId}
 *   controller receives takeover-requested {requester} and replies takeover {allow}
 *   a replaced controller receives {type: 'replaced'}
 *
 * Playback uses Playback v1, the session contract every Portico client plays through:
 * POST /v1/playback/sessions, the presentation URL it answers with, timeline reports
 * (progress and lease renewal), PATCH for a seek the server must produce, DELETE to end.
 * CAF owns media loading, its standard player draws playback (V-11), and the
 * overlays in index.html cover idle, preparing, errors and takeover requests.
 * Pairing credentials never enter media URLs or sender messages.
 */
(function () {
  'use strict';

  var STORAGE_KEY = 'portico.cast.deviceToken';
  var PROTOCOL = '1.0';
  var PROGRESS_INTERVAL_MS = 10000;
  var NAMESPACE = 'urn:x-cast:tv.getportico.cast';
  var ORIGIN_KEY = 'portico.cast.serverOrigin';
  var TAKEOVER_ASK_MS = 30000;     // controller present: no answer means no
  var TAKEOVER_ABSENT_MS = 10000;  // controller gone: the TV says who is taking over, then switches
  var ERROR_VISIBLE_MS = 20000;

  // Which Portico server this receiver talks to. Served by a server, that is the server itself.
  // Served from the shared page at cast.getportico.tv, the sender names the viewer's server when
  // it pairs; only an HTTPS origin is accepted, because a Cast device will not load anything else.
  var base = '';
  function serverOrigin(value) {
    if (!value) { return ''; }
    try {
      var url = new URL(String(value));
      if (url.protocol !== 'https:' || url.username || url.password) { return null; }
      return url.origin === window.location.origin ? '' : url.origin;
    } catch (e) { return null; }
  }
  try { base = serverOrigin(window.localStorage.getItem(ORIGIN_KEY)) || ''; } catch (e) { base = ''; }
  var broadcast = function () {};

  // --- Presentation ---------------------------------------------------------

  function $(id) { return document.getElementById(id); }
  var ui = $('ui');
  var status = $('status');
  var errorTimer = null, noticeTimer = null;
  var viewGeneration = 0, viewLoading = false, pictureReady = false, viewBuffering = false;
  var currentArtwork = '', preparingSince = null, preparingTimer = null, observedRemoteKeys = false, activeTrackKey = null;

  // The view owns drawing only. No credential, session, grant or control callback crosses this port.
  function viewCall(method, args) {
    var view = window.porticoCastView;
    if (!view || typeof view[method] !== 'function') return false;
    try { view[method].apply(view, args || []); return true; } catch (e) { return false; }
  }
  // The composed receiver loads its generated canonical catalogue before this engine.
  function copy(key, params) {
    var catalogue = window.porticoCastText;
    return catalogue && typeof catalogue.t === 'function' ? catalogue.t('cast.receiver.' + key, params) : '';
  }
  function remoteKeysAvailable() {
    try { if (castContext && castContext.getDeviceCapabilities().dpad_input_supported === true) return true; } catch (e) { /* unknown */ }
    return observedRemoteKeys;
  }
  function stopPreparing() { clearTimeout(preparingTimer); preparingTimer = null; preparingSince = null; }
  function preparing() {
    if (preparingSince !== null) return;
    preparingSince = Date.now();
    var generation = viewGeneration;
    function tick() {
      if (generation !== viewGeneration || preparingSince === null) return;
      viewCall('preparing', [Math.max(0, Date.now() - preparingSince)]);
      preparingTimer = setTimeout(tick, 1000);
    }
    tick();
  }
  function resetViewPlayback() {
    stopPreparing(); viewGeneration++; viewLoading = false; pictureReady = false; viewBuffering = false;
    currentArtwork = ''; activeTrackKey = null;
  }
  function clockFacts() {
    var facts = {};
    if (!playback || !manager) return facts;
    try { var at = manager.getCurrentTimeSec(); if (typeof at === 'number' && Number.isFinite(at) && at >= 0) facts.positionSeconds = at; } catch (e) { /* unknown */ }
    try { var length = typeof manager.getDurationSec === 'function' ? manager.getDurationSec() : undefined; if (typeof length === 'number' && Number.isFinite(length) && length >= 0) facts.durationSeconds = length; } catch (e) { /* unknown */ }
    return facts;
  }
  function playingView(revealOverlay) {
    if (!pictureReady || !playback || !manager) return;
    var state = viewBuffering ? 'BUFFERING' : manager.getPlayerState();
    if (['PLAYING','PAUSED','BUFFERING'].indexOf(state) < 0) return;
    // Observe selected tracks rather than treating a requested track change as applied.
    try {
      var audio = manager.getAudioTracksManager(), textTracks = manager.getTextTracksManager();
      var key = JSON.stringify([typeof audio.getActiveId === 'function' ? audio.getActiveId() : null, typeof textTracks.getActiveIds === 'function' ? textTracks.getActiveIds() : null]);
      if (activeTrackKey !== null && key !== activeTrackKey) revealOverlay = true;
      activeTrackKey = key;
    } catch (e) { /* no track observation */ }
    var snapshot = Object.assign({meta: meta ? Object.assign({},meta) : null, state: state.toLowerCase(), remoteKeysAvailable: remoteKeysAvailable()}, clockFacts());
    if (currentArtwork) snapshot.artworkUrl = currentArtwork;
    if (revealOverlay === true) snapshot.revealOverlay = true;
    viewCall('playing', [snapshot]);
  }

  function show(state) { if (ui && ui.setAttribute) ui.setAttribute('data-state', state); }
  function text(id, value) { var el = $(id); if (el) el.textContent = value || ''; }
  function visible(id, on) { var el = $(id); if (el) el.hidden = !on; }
  function say(message, tone) {
    if (viewCall('pairingStatus', [message || '', tone])) return;
    if (!status) return;
    status.textContent = message || '';
    if (tone) { status.setAttribute('data-tone', tone); } else { status.removeAttribute('data-tone'); }
  }
  /** An approved notice over whatever is on screen. Transient recovery is silent. */
  function notice(message, ms) {
    clearTimeout(noticeTimer);
    if (viewCall('notice', [message || ''])) {
      if (message && ms) noticeTimer = setTimeout(function () { viewCall('notice', ['']); }, ms);
      return;
    }
    text('notice', message); visible('notice', !!message);
    if (message && ms) noticeTimer = setTimeout(function () { visible('notice', false); }, ms);
  }
  function showIdle() { clearTimeout(errorTimer); resetViewPlayback(); if (!viewCall('idle')) show('idle'); }
  function showLoading(meta) {
    clearTimeout(errorTimer);
    resetViewPlayback(); viewLoading = true;
    if (window.porticoCastView && typeof window.porticoCastView.loading === 'function' && !(meta && meta.title)) { viewLoading = false; return; }
    if (viewCall('loading', [meta ? Object.assign({},meta) : null])) return;
    text('loading-eyebrow', copy('gettingReady'));
    text('loading-title', meta && meta.title || '');
    text('loading-subtitle', meta && (meta.subtitle || meta.seriesTitle) || '');
    visible('loading-art', false);
    show('loading');
  }
  function showArtwork(url, square) {
    if (!url) return;
    currentArtwork = url;
    if (viewCall('artwork', [url, !!square])) return;
    var art = $('loading-art');
    if (!art || !url) return;
    art.src = url;
    if (art.classList) art.classList.toggle('square', !!square);
    art.hidden = false;
  }
  function showPlaying() { clearTimeout(errorTimer); if (window.porticoCastView && typeof window.porticoCastView.playing === 'function') { playingView(); return; } show('playing'); }
  function errorKind(code) {
    code = String(code || '');
    if (/not_found|unavailable|forbidden|permission/.test(code)) return 'unavailable';
    if (/busy|capacity|cap$|occupied|timeout/.test(code)) return 'busy';
    if (/network|fetch|http_5|unreach|abort/.test(code)) return 'unreachable';
    if (/decode|stalled|playback_failed/.test(code)) return 'decode';
    return 'generic';
  }
  /** A visible error card: the TV always says what went wrong (CAST-01). Never shows raw codes. */
  function showError(code) {
    var resumePicture = pictureReady && !!playback;
    stopPreparing(); viewGeneration++; pictureReady = false; viewLoading = false;
    if (!viewCall('error', [errorKind(code)])) {
      text('error-title', copy('error.' + errorKind(code) + '.title'));
      text('error-body', copy('error.' + errorKind(code) + '.body'));
      show('error');
    }
    clearTimeout(errorTimer);
    // The card stays 20 seconds, then returns to what was on screen: the picture
    // when something is still playing, otherwise idle.
    errorTimer = setTimeout(function () {
      if (!playback) { showIdle(); return; }
      var playingState = manager ? manager.getPlayerState() : 'IDLE';
      if (resumePicture && ['PLAYING','PAUSED','BUFFERING'].indexOf(playingState) >= 0) { pictureReady = true; showPlaying(); return; }
      // Nothing is playing: the failed occurrence ends with its card instead of leaving a
      // session (and any transcode behind it) open under an idle screen, and the controller
      // is told the title failed here so it can offer the retry the card names.
      if (controllerSender) send(controllerSender, {type: 'load-failed', code: 'playback_failed'});
      serial(function () { return stopPlayback().catch(function () {}); }).then(broadcast).catch(function () {});
    }, ERROR_VISIBLE_MS);
  }

  // --- Credentials and requests --------------------------------------------

  var session = null; // {accessToken, expiresAt}
  var scope = null;   // {accountId, profileId, authority, capabilities}
  // The Playback v1 session playing here: {id, revision, itemId, generation, serverSeek}.
  var playback = null;
  var shuttingDown = false, activeRequests = new Set();
  var mediaAddresses = new Map(); // bounded receiver-private addresses; never placed in CAF status
  function mediaAddress(request) { return request && request.media ? mediaAddresses.get(request.media.contentId) || '' : ''; }
  function queuePath(id) { return '/v1/queues/' + encodeURIComponent(id); }
  function queueRef(value) { return value && typeof value.queueId === 'string' && typeof value.entryId === 'string' && /^[A-Za-z0-9._:-]{1,128}$/.test(value.queueId) && /^[A-Za-z0-9._:-]{1,128}$/.test(value.entryId) ? {queueId:value.queueId,entryId:value.entryId} : null; }
  var manager = null, castContext = null, profiled = false;
  // CAF routes PlayerManager's own calls through the same interceptors as the
  // media channel. Only these exact engine-created objects may bypass sender
  // gates; an external JSON request cannot reproduce their private identity.
  var localRequests = new WeakSet(), localRequestId = 0;
  function loadMedia(request) { localRequests.add(request); return manager.load(request); }
  function mediaCommand(type, fields) {
    var request=Object.assign({type:type,requestId:++localRequestId},fields);
    localRequests.add(request);manager.sendLocalMediaRequest(request);
  }

  var reportTimer = null, reportSeq = 0, reportFailures = 0, recoveries = 0, recovering = false, bufferingTimer = null;
  var commands = Promise.resolve();
  function serial(work) { var result = commands.then(work); commands = result.catch(function () {}); return result; }
  function token(bytes) { var a = new Uint8Array(bytes || 24); window.crypto.getRandomValues(a); return Array.from(a, function (x) { return x.toString(16).padStart(2, '0'); }).join(''); }
  function position() { return manager ? manager.getCurrentTimeSec() || 0 : 0; }
  function paused() { return !manager || manager.getPlayerState() === 'PAUSED'; }
  function sessionPath(id) { return '/v1/playback/sessions/' + encodeURIComponent(id); }
  function wait(ms) { return new Promise(function (resolve) { setTimeout(resolve, ms); }); }

  function readToken() {
    try { return window.localStorage.getItem(STORAGE_KEY) || ''; } catch (e) { return ''; }
  }
  function writeToken(value) {
    try {
      if (value) { window.localStorage.setItem(STORAGE_KEY, value); }
      else { window.localStorage.removeItem(STORAGE_KEY); }
    } catch (e) { /* a receiver with storage disabled simply re-pairs each boot */ }
  }

  // deviceId is stable for the life of the stored pairing. A receiver that has
  // been unpaired generates a new one, so an old device row is never reused.
  function deviceId() {
    var key = 'portico.cast.deviceId';
    var value = '';
    try { value = window.localStorage.getItem(key) || ''; } catch (e) { /* ignore */ }
    if (!value) {
      var bytes = new Uint8Array(16);
      (window.crypto || window.msCrypto).getRandomValues(bytes);
      value = 'cast-';
      for (var i = 0; i < bytes.length; i++) { value += bytes[i].toString(16).padStart(2, '0'); }
      try { window.localStorage.setItem(key, value); } catch (e) { /* ignore */ }
    }
    return value;
  }

  /** One request, bounded in time and size. Resolves {status, body} for 2xx; throws with
   * {code, status, current} otherwise (`current` is a 412's session). */
  async function request(path, options) {
    if(shuttingDown)throw new Error('receiver_shutdown');
    var headers = Object.assign({'Content-Type': 'application/json'}, options.headers || {});
    if (options.bearer) headers.Authorization = 'Bearer ' + options.bearer;
    var abort = new AbortController(), timer = setTimeout(function () { abort.abort(); }, 15000);
    var active={abort:abort,timer:timer};activeRequests.add(active);
    try {
      var response = await fetch((options.origin === undefined ? base : options.origin) + path, {method: options.method || 'POST', headers: headers, signal: abort.signal, body: options.body === undefined ? undefined : JSON.stringify(options.body)});
      var text = '';
      if (response.body) {
        var reader = response.body.getReader(), decoder = new TextDecoder(), size = 0;
        try { for (;;) { var chunk = await reader.read(); if (chunk.done) break; size += chunk.value.byteLength; if (size > 1048576) throw new Error('response_too_large'); text += decoder.decode(chunk.value, {stream: true}); } text += decoder.decode(); } finally { reader.releaseLock(); }
      }
      var payload = text ? JSON.parse(text) : {};
      if (!response.ok) {
        var failure = payload.error || {};
        var error = new Error(failure.code || 'http_' + response.status); error.code = error.message; error.status = response.status; error.current = failure.current;
        throw error;
      }
      return {status: response.status, body: payload};
    } finally { activeRequests.delete(active);clearTimeout(timer);abort.abort(); }
  }
  function post(path, body, bearer, origin) { return request(path, {body: body, bearer: bearer, origin: origin}).then(function (r) { return r.body; }); }
  /** A request with the receiver's bearer. An expired bearer is renewed once through the device
   * token; a transport failure or 5xx is retried once with the identical request (the same
   * Idempotency-Key, If-Match or timeline sequence), which the server treats as the same request. */
  var PAIRING_ENDED = ['cast_device_revoked', 'cast_device_not_found', 'cast_device_expired'];
  async function api(path, options) {
    options = options || {};
    var attempt = function () { return request(path, Object.assign({}, options, {bearer: session.accessToken})); };
    try { return await attempt(); }
    catch (e) {
      if (e.status === 401 && readToken()) {
        var renewed;
        // The server's own answer that this TV's pairing is over is final; anything else that
        // stops a renewal (no network, a failing server) says nothing about the pairing.
        try { renewed = await post('/v1/cast/reconnect', {protocolVersion: PROTOCOL, deviceToken: readToken(), deviceId: deviceId()}); }
        catch (refused) { if (PAIRING_ENDED.indexOf(refused.code) >= 0) refused.pairingEnded = true; throw refused; }
        if (!renewed.scope || renewed.scope.accountId !== scope.accountId || renewed.scope.profileId !== scope.profileId) { var changed = new Error('pairing_scope_changed'); changed.pairingEnded = true; throw changed; }
        session = renewed.session; scope = renewed.scope; writeToken(renewed.deviceToken);
        return attempt();
      }
      if (e.status && e.status < 500) throw e;
      return attempt();
    }
  }
  function buildProfile(context) {
    var caps = {};
    try { caps = context ? context.getDeviceCapabilities() || {} : {}; } catch (_) { /* no output facts */ }
    function supports(codec, width, height, fps) { try { var parts=codec.split('; codecs=');return !!context && context.canDisplayType(parts[0],parts[1]?parts[1].replace(/"/g,''):'',width,height,fps); } catch (_) { return false; } }
    var client = {family:'cast',form:'cast',platform:'cast',app:'portico-cast'};
    try {
      var info = context && typeof context.getDeviceInformation === 'function' ? context.getDeviceInformation() : null;
      if (info && typeof info['model-name'] === 'string' && info['model-name'].trim() && info['model-name'].trim().toLowerCase()!=='unknown') client.model=info['model-name'].trim().slice(0,64);
    } catch (_) { /* a codec probe is not a device identity */ }
    var fourK = supports('video/mp4; codecs="hev1.2.4.L153.B0"', 3840, 2160, 60);
    var ranges = ['sdr'];
    if (caps.is_hdr_supported === true) ranges.push('hdr10');
    var width = fourK ? 3840 : 1920, height = fourK ? 2160 : 1080;
    // Every Chromecast since the 2nd generation decodes H.264 High@4.2 1080p60; probe it rather than assume 30 fps (CAST-08).
    var h264At60 = supports('video/mp4; codecs="avc1.64002A"', 1920, 1080, 60);
    var video = [{codec:'h264',bitDepths:[8],maxWidth:1920,maxHeight:1080,maxLevel:h264At60?42:41,maxFrameRate:h264At60?60:30,dynamicRanges:['sdr'],evidence:h264At60?'probed':'declared'},{codec:'vp8',bitDepths:[8],maxWidth:1920,maxHeight:1080,maxFrameRate:30,dynamicRanges:['sdr'],evidence:'declared'}];
    [['hevc','hev1.2.4.L153.B0'],['vp9','vp09.02.10.10'],['av1','av01.0.08M.10']].forEach(function (pair) {
      if (supports('video/mp4; codecs="' + pair[1] + '"', width, height, 60)) video.push({codec:pair[0],bitDepths:[8,10],maxWidth:width,maxHeight:height,maxFrameRate:60,dynamicRanges:ranges.filter(function (x) {return x !== 'dolby_vision' || pair[0] === 'hevc';}),dolbyVisionProfiles:[],evidence:'probed'});
    });
    var hevc = video.filter(function(v) { return v.codec === 'hevc'; })[0];
    if (hevc && caps.is_dv_supported === true) {
      hevc.dolbyVisionProfiles = [5,8].filter(function(profile) { return supports('video/mp4; codecs="dvhe.0' + profile + '.07"', width, height, 30); });
      if (hevc.dolbyVisionProfiles.length) { hevc.dynamicRanges.push('dolby_vision'); ranges.push('dolby_vision'); }
    }
    // Do not turn a MIME/codec answer into a fabricated decoded channel count. The SDK's
    // passthrough flag is separate from ordinary E-AC-3 acceptance, notably on newer Streamers.
    var audio = [];
    [['aac','audio/mp4; codecs="mp4a.40.2"'],['mp3','audio/mpeg'],['opus','audio/ogg; codecs="opus"'],['vorbis','audio/ogg; codecs="vorbis"'],['flac','audio/flac'],['pcm','audio/wav; codecs="1"']].forEach(function(pair) {
      var probed = supports(pair[1]);
      if (probed || pair[0] === 'aac' || pair[0] === 'mp3') audio.push({codec:pair[0],maxChannels:2,evidence:probed?'probed':'declared'});
    });
    var passthrough = [];
    [['ac3','ac-3',6],['eac3','ec-3',8]].forEach(function (pair) {
      if (supports('audio/mp4; codecs="' + pair[1] + '"')) {
        var atmos = pair[0] === 'eac3' && caps.is_dolby_atmos_supported === true;
        passthrough.push({codec:pair[0],variants:pair[0] === 'ac3' ? ['ac3'] : atmos ? ['eac3','eac3_joc'] : ['eac3'],maxChannels:pair[2],evidence:'probed',objectAudio:atmos ? ['atmos'] : []});
      }
    });
    if (caps.display_supported === false) passthrough = [];
    var surround=caps.display_supported!==false&&passthrough.length>0;
    var fmp4Audio=audio.filter(function(a){return ({aac:'mp4a.40.2',mp3:'mp4a.69',flac:'fLaC',opus:'Opus'})[a.codec]&&supports('audio/mp4; codecs="'+({aac:'mp4a.40.2',mp3:'mp4a.69',flac:'fLaC',opus:'Opus'})[a.codec]+'"');}).map(function(a){return a.codec;}).concat(passthrough.map(function(a){return a.codec;}));
    var hlsAudio=audio.filter(function(a){return ['aac','mp3'].indexOf(a.codec)>=0;}).map(function(a){return a.codec;}).concat(passthrough.map(function(a){return a.codec;}));
    var names=video.map(function (v) {return v.codec;});
    var transports=[{transport:'direct',containers:['mp4','m4v'],video:names.filter(function (c) {return c!=='vp9'&&c!=='vp8';}),audio:hlsAudio},{transport:'direct',containers:['webm'],video:names.filter(function(c){return c==='vp8'||c==='vp9'||c==='av1';}),audio:audio.filter(function(a){return ['opus','vorbis'].indexOf(a.codec)>=0;}).map(function(a){return a.codec;})},{transport:'direct',containers:audio.reduce(function(all,a){return all.concat(({aac:['m4a','aac'],mp3:['mp3'],flac:['flac'],opus:['ogg','opus'],vorbis:['ogg'],pcm:['wav']})[a.codec]||[]);},[]),video:[],audio:audio.map(function(a){return a.codec;})},{transport:'hls_ts',containers:['mpegts'],video:['h264'],audio:hlsAudio},{transport:'hls_fmp4',containers:['fmp4'],video:names.filter(function (c) {return c!=='vp9'&&c!=='vp8';}),audio:fmp4Audio}];
    // The capability document (version 2): the device, and what its one player engine can do.
    // Display presence and codec acceptance do not identify the active audio route.
    return {version:2,client:client,engineSetting:'automatic',display:{dynamicRanges:ranges},audioOutput:{maxChannels:surround?Math.max.apply(null,passthrough.map(function(a){return a.maxChannels;})):2},engines:{native:{name:'caf-shaka',evidence:'mixed',video:video,audio:audio,audioPassthrough:passthrough,transports:transports,subtitleRendering:['text']}}};
  }
  /** CAF's type probe has no channel/rate arguments. Where the receiver also exposes the
   * MediaCapabilities API, ask exact decoded configurations; silence/timeout stays stereo. */
  async function probeAudioOutput(profile) {
    var capabilities = typeof navigator !== 'undefined' && navigator.mediaCapabilities;
    if (!capabilities || typeof capabilities.decodingInfo !== 'function') return profile;
    await Promise.all(profile.engines.native.audio.map(async function(codec) {
      var sample = {aac:'mp4a.40.2',flac:'fLaC',opus:'Opus',pcm:'lpcm'}[codec.codec];
      if (!sample) return;
      async function accepts(channels, rate) {
        var timer;
        try { return (await Promise.race([capabilities.decodingInfo({type:'media-source',audio:{contentType:'audio/mp4; codecs="'+sample+'"',channels:String(channels),samplerate:rate,bitrate:8000000}}),new Promise(function(resolve){timer=setTimeout(function(){resolve(null);},1500);})]))?.supported === true; }
        catch (_) { return false; } finally { clearTimeout(timer); }
      }
      for (var channels of [8,6,2]) { if (await accepts(channels,48000)) { codec.maxChannels=channels;codec.evidence='probed';break; } }
      for (var rate of [192000,96000,48000,44100]) { if (await accepts(codec.maxChannels,rate)) { codec.maxSampleRate=rate;break; } }
    }));
    profile.audioOutput.maxChannels=Math.max(profile.audioOutput.maxChannels,...profile.engines.native.audio.map(function(a){return a.maxChannels;}));
    return profile;
  }
  /** What this TV decodes, published for the session's device before anything plays. */
  async function prepareProfile() {
    if (profiled) return;
    var context = window.cast && cast.framework.CastReceiverContext.getInstance();
    await api('/v1/me/devices/current/capabilities', {method: 'PUT', body: await probeAudioOutput(buildProfile(context))});
    profiled = true;
  }

  function adopt(result) {
    session = result.session;
    scope = result.scope;
    writeToken(result.deviceToken);
    profiled = false;
    say('');
    return prepareProfile().then(function () { return result; });
  }

  // A reconnect whose answer was lost left the old token in storage; the server accepts it once
  // more, for a short while, with this receiver's deviceId, so one retry recovers it.
  function reconnect() {
    var stored = readToken();
    if (!stored) { return Promise.reject(new Error('no_stored_pairing')); }
    var ask = function () { return post('/v1/cast/reconnect', {protocolVersion: PROTOCOL, deviceToken: stored, deviceId: deviceId()}); };
    return ask().catch(function (e) { if (e && e.status) throw e; return ask(); }).then(adopt);
  }

  /** Redeem a code with a server without touching the current pairing (kept until the new one is adopted). */
  function redeemAt(origin, code) {
    return post('/v1/cast/redeem', {protocolVersion:PROTOCOL,code:code,deviceId:deviceId()}, undefined, origin);
  }
  /** Make a redeemed pairing current: stop what's playing, forget the old credential, adopt the new one. */
  async function switchTo(result, origin) {
    await stopPlayback();
    base = origin;
    try { if (base) window.localStorage.setItem(ORIGIN_KEY, base); else window.localStorage.removeItem(ORIGIN_KEY); } catch (_) { /* ignore */ }
    profiled=false;
    return adopt(result);
  }
  /** Pairing by typing a code: only offered when the page is opened in an ordinary browser. */
  async function redeem(code) {
    var result = await redeemAt(base, code);
    return switchTo(result, base);
  }

  // A Cast device (Chromecast, Google TV) has no keyboard: the code box is only for this page opened
  // in an ordinary browser, for diagnosis. Never ask a TV to type (CAST-01, V-11).
  var ua = (typeof navigator !== 'undefined' && navigator.userAgent) || '';
  var castDevice = /CrKey|Android|Google ?TV|Fuchsia|AFT/i.test(ua);
  var browserPair = $('browser-pair');
  viewCall('browserPair', [!castDevice]);
  if (!castDevice && browserPair) {
    browserPair.hidden = false;
    browserPair.addEventListener('submit', function (event) {
      if (event && event.preventDefault) event.preventDefault();
      var input = $('code-input'), button = $('pair');
      var code = ((input && input.value) || '').trim().toUpperCase();
      if (code.length < 4) { say(copy('pair.enterCode'), 'error'); return; }
      if (button) button.disabled = true;
      say(copy('pair.connecting'));
      serial(function(){return redeem(code);}).then(function () {
        if (button) button.disabled = false;
        if (input) input.value = '';
        say(copy('pair.connected'));
      }).catch(function (error) {
        if (button) button.disabled = false;
        say(pairingMessage(error), 'error');
      });
    });
  }

  function pairingMessage(error) {
    switch (error && error.code) {
      case 'cast_code_expired': return copy('pair.expired');
      case 'cast_code_consumed': return copy('pair.consumed');
      case 'cast_code_revoked': return copy('pair.canceled');
      case 'cast_code_not_found': return copy('pair.invalid');
      case 'rate_limited': return copy('pair.rateLimited');
      case 'tv_busy': return copy('pair.busy');
      default: return copy('pair.failed');
    }
  }

  // --- Playback --------------------------------------------------------------

  var meta = null; // genuine controller or authorized catalogue display identity
  function cleanText(value, max) { return typeof value === 'string' ? value.replace(/[\u0000-\u001f\u007f]/g, ' ').trim().slice(0, max || 200) : ''; }
  function displayMeta(message) {
    if (!message || typeof message !== 'object') return null;
    var kind = cleanText(message.kind, 32);
    var season = Number.isSafeInteger(message.season) ? message.season : undefined;
    var episode = Number.isSafeInteger(message.episode) ? message.episode : undefined;
    var artworkPath = typeof message.artworkPath === 'string' && /^\/v[12]\/[^\s\\]{1,500}$/.test(message.artworkPath) ? message.artworkPath : '';
    var m = {title: cleanText(message.title), subtitle: cleanText(message.subtitle), seriesTitle: cleanText(message.seriesTitle), kind: kind, season: season, episode: episode, artworkPath: artworkPath};
    return m.title || m.subtitle || m.artworkPath ? m : null;
  }
  /** Artwork is protected: fetch it with the receiver's own bearer and hand CAF a data URL. */
  async function artworkUrl(path) {
    if (!path || !session || typeof FileReader === 'undefined') return '';
    var abort = new AbortController(), timer = setTimeout(function () { abort.abort(); }, 8000);
    try {
      var response = await fetch(base + path, {headers: {Authorization: 'Bearer ' + session.accessToken}, signal: abort.signal});
      if (!response.ok) return '';
      var blob = await response.blob();
      if (!/^image\//.test(blob.type) || blob.size > 3145728) return '';
      return await new Promise(function (resolve) { var reader = new FileReader(); reader.onload = function () { resolve(String(reader.result || '')); }; reader.onerror = function () { resolve(''); }; reader.readAsDataURL(blob); });
    } catch (e) { return ''; } finally { clearTimeout(timer); }
  }
  /** Kinds the drawing gives the square cover and the "Now playing" screen (design frames 4 and 9). */
  var AUDIO_KINDS = {song:1, track:1, album:1, audiobook:1, audiobook_file:1, chapter:1};
  function audioKind(kind) { return AUDIO_KINDS[kind] === 1; }
  function castMetadata(m, image) {
    if (!m) return undefined;
    var images = image ? [{url: image}] : [];
    if (m.kind === 'episode') return {metadataType: 2, title: m.title, seriesTitle: m.seriesTitle || m.subtitle, season: m.season, episode: m.episode, images: images};
    if (m.kind === 'movie') return {metadataType: 1, title: m.title, subtitle: m.subtitle, images: images};
    // Structured catalogue fields are facts; the old joined subtitle is not an artist.
    if (audioKind(m.kind)) return Object.assign({metadataType: 3, title: m.title, images: images},m.artist?{artist:m.artist}:{},m.albumTitle?{albumName:m.albumTitle}:{});
    return {metadataType: 0, title: m.title, subtitle: m.subtitle, images: images};
  }
  /** A same-origin path the catalogue itself gave us; never a sender-supplied address. */
  function cataloguePath(value) { return typeof value === 'string' && /^\/(?!\/)[^\s\\]{1,500}$/.test(value) ? value : ''; }
  /** A song's own record has no cover, and neither has an audiobook chapter: the album's or the
   * book's square cover is the drawn artwork (frames 4 and 9). The related id comes from the
   * current item's own projection, read with this TV's credential, and only its own kind counts. */
  async function relatedCover(id, kind) {
    if (typeof id !== 'string' || !id || id.length > 128 || !kind) return '';
    var related;
    try { related = (await api('/v1/items/' + encodeURIComponent(id), {method: 'GET'})).body; }
    catch (e) { return ''; }
    if (!related || related.id !== id || related.missing || related.kind !== kind) return '';
    return cataloguePath(related.posterUrl);
  }

  // GET /v1/items/{id} is the existing ScopeRead catalogue projection. Display
  // enrichment never blocks media or gains authority over a playback session.
  async function refreshCatalogueDisplay(owner, contentId) {
    var generation=viewGeneration, origin=base, account=scope&&scope.accountId, profile=scope&&scope.profileId;
    function current() { return playback&&playback.id===owner.id&&playback.generation===owner.presentation.generation&&playback.itemId===owner.itemId&&viewGeneration===generation&&base===origin&&scope&&scope.accountId===account&&scope.profileId===profile; }
    function publish() {
      if(!current())return;
      if(pictureReady)playingView();
      else if(meta&&meta.title){
        viewLoading=true;viewCall('loading',[Object.assign({},meta)]);
        // A catalogue enrichment is still this occurrence: loading clears the
        // view, so restore its genuine current cover even if no new URL arrived.
        if(currentArtwork)showArtwork(currentArtwork,audioKind(meta.kind));
      }
      try {
        var media=typeof manager.getMediaInformation==='function'?manager.getMediaInformation():null;
        if(media&&media.contentId===contentId&&typeof manager.setMediaInformation==='function')manager.setMediaInformation(Object.assign({},media,{metadata:castMetadata(meta,currentArtwork)}),false);
      } catch(e) { /* A display update never changes media/session authority. */ }
    }
    try {
      var item=(await api('/v1/items/'+encodeURIComponent(owner.itemId),{method:'GET'})).body;
      if(!current()||!item||item.id!==owner.itemId||item.missing)return;
      var year=item.kind==='movie'&&Number.isSafeInteger(item.year)&&item.year>0&&item.year<=9999?String(item.year):undefined;
      var identity=displayMeta({title:item.title,kind:item.kind,subtitle:year,artworkPath:item.posterUrl});
      if(!identity)return;
      if(item.song&&typeof item.song==='object'){
        var artist=cleanText(item.song.artist),album=cleanText(item.song.albumTitle);
        if(artist)identity.artist=artist;if(album)identity.albumTitle=album;
      }
      if(item.episode&&typeof item.episode==='object'){
        identity.seriesTitle=cleanText(item.episode.showTitle);
        if(Number.isSafeInteger(item.episode.seasonNumber))identity.season=item.episode.seasonNumber;
        if(Number.isSafeInteger(item.episode.number))identity.episode=item.episode.number;
      }
      // An audiobook's screen shows the chapter, then the book, then the author (frame 9).
      if(item.bookFile&&typeof item.bookFile==='object'){
        var author=cleanText(item.author),book=cleanText(item.bookFile.bookTitle);
        if(author)identity.artist=author;if(book)identity.albumTitle=book;
      }
      meta=identity;publish();
      var coverPath=identity.artworkPath;
      if(!coverPath&&audioKind(item.kind)){
        var coverId='',coverKind='';
        if(item.song&&typeof item.song==='object'){coverId=item.song.albumId;coverKind='album';}
        else if(item.bookFile&&typeof item.bookFile==='object'){coverId=item.bookFile.bookId;coverKind='book';}
        coverPath=await relatedCover(coverId,coverKind);
        if(!current())return;
        if(coverPath){identity.artworkPath=coverPath;meta=identity;publish();}
      }
      if(coverPath){
        var image=await artworkUrl(coverPath);
        if(current()&&image){
          currentArtwork=image;
          // loading(meta) resets the view's art. Publish that identity before
          // its artwork, while CAF metadata receives the same genuine image.
          publish();
        }
      }
    } catch(e) { /* Missing/refused metadata remains absent; no notice or old-title fallback. */ }
  }

  // Playback v1 (the same session contract every Portico client plays through): start a session
  // for the item, load its presentation URL, report the timeline (progress, lease renewal and
  // observation in one POST), and end it. Play and pause are the player's own and are reported;
  // a seek goes through the server only when it must produce the stream again. Media URLs are
  // accepted only from the server's answer. Commands share one serialized lane.
  var directTypes = {mp3:'audio/mpeg',flac:'audio/flac',m4a:'audio/mp4',m4b:'audio/mp4',aac:'audio/aac',ogg:'audio/ogg',opus:'audio/ogg',wav:'audio/wav',webm:'video/webm',mp4:'video/mp4',m4v:'video/mp4',mov:'video/mp4'};
  /** Whether a seek must go through the server: only when something is transcoded or burned in. */
  function serverSeek(session) {
    var d = session.presentation.decision || {};
    return [d.video, d.audio, d.subtitles].some(function (x) { return !!x && (x.action === 'transcode' || x.action === 'burn'); });
  }
  function track(session) {
    return {id: session.id, revision: session.revision, itemId: session.itemId || '', generation: session.presentation.generation, serverSeek: serverSeek(session), queue: queueRef(session.queue)};
  }
  /** CAF's load request for a session's presentation. */
  function loadRequest(session, image, autoplay) {
    var p = session.presentation;
    if (typeof p.url !== 'string' || !/^\/v1\/media\/[^\\\s]+$/.test(p.url)) throw new Error('invalid_media_url');
    var container = String(p.container || '');
    // The server may deliver an audio conversion through the presentation URL while
    // its general delivery decision still describes the source container. Only the
    // playable audio plan for this exact URL can supply the delivered audio type.
    var audioOnly = p.decision && !p.decision.video && !!p.decision.audio;
    var render = p.audioRender;
    if (p.mode !== 'stream' && audioOnly && render && (render.mode === 'direct' || render.mode === 'converted') && render.url === p.url) {
      container = String(render.container || '');
    }
    var contentId = 'portico-' + token(16);
    mediaAddresses.set(contentId, base + p.url);
    while (mediaAddresses.size > 8) mediaAddresses.delete(mediaAddresses.keys().next().value);
    var media = {contentId: contentId, streamType: 'BUFFERED'};
    if (p.mode === 'stream') {
      media.contentType = 'application/x-mpegurl';
      media.hlsSegmentFormat = media.hlsVideoSegmentFormat = container === 'fmp4_hls' ? 'FMP4' : 'TS';
    } else {
      if (!directTypes[container]) throw new Error('unsupported_media_container');
      media.contentType = audioOnly && ['mp4','mov','m4v'].indexOf(container)>=0 ? 'audio/mp4' : audioOnly && container==='webm' ? 'audio/webm' : directTypes[container];
    }
    // Title and artwork for CAF's standard player, the Google TV ambient UI and the Google Home app (CAST-01 §2).
    var metadata = castMetadata(meta, image);
    if (metadata) media.metadata = metadata;
    var load = {media: media, currentTime: Math.max(0, (p.startPositionMs || 0) / 1000), autoplay: autoplay !== false};
    // Only the actual current media enters CAF. The server owns the next item;
    // preloadTime remains a hint and never establishes that bytes were prefetched.
    if (queueRef(session.queue)) load.queueData = {items:[{media:media,autoplay:load.autoplay,startTime:load.currentTime,preloadTime:10}],startIndex:0};
    return load;
  }

  async function stopPlayback(ended) {
    clearTimeout(reportTimer); clearTimeout(bufferingTimer);
    if (!playback) return;
    var id = playback.id, at = Math.round(position() * 1000);
    playback = null; meta = null;
    if (manager) mediaCommand('STOP');
    mediaAddresses.clear();
    showIdle();
    // A session that has already ended (404, 410) is simply gone.
    await api(sessionPath(id), {method: 'DELETE', body: ended ? undefined : {positionMs: at}}).catch(function (e) { if (!e.status || e.status >= 500) throw e; });
  }
  /** Starts a v1 session for the item and answers CAF's load request for it. A session already
   * playing here is replaced in the same request, so this TV holds one playback slot. */
  async function resolvePlayback(itemId, startSeconds, identity, subtitleTrackId, queue) {
    if (!session || !manager || !/^[A-Za-z0-9_-]{1,128}$/.test(itemId)) throw new Error('playback_unavailable');
    clearTimeout(reportTimer); clearTimeout(bufferingTimer);
    var replaces = playback && playback.id;
    playback = null;
    meta = identity || null;
    showLoading(meta);
    var viewLoad = viewGeneration;
    var art = meta && meta.artworkPath ? artworkUrl(meta.artworkPath).then(function (url) { if (url && viewLoad === viewGeneration) showArtwork(url, meta && audioKind(meta.kind)); return url; }) : Promise.resolve('');
    await prepareProfile();
    var body = {itemId: itemId, state: 'playing'};
    if (Number.isFinite(startSeconds)) body.startPositionMs = Math.round(Math.max(0, startSeconds) * 1000); else body.startFrom = 'resume';
    if (replaces) body.replacesSessionId = replaces;
    if (queueRef(queue)) { body.queue = queueRef(queue); delete body.itemId; }
    // The sender's subtitle choice ("off", or a track id). This TV draws text only; the server
    // delivers a styled or image track burned into the picture.
    if (subtitleTrackId === 'off') body.subtitles = {trackId: null};
    else if (typeof subtitleTrackId === 'string' && /^[A-Za-z0-9._:-]{1,128}$/.test(subtitleTrackId)) body.subtitles = {trackId: subtitleTrackId};
    var key = token(16), deadline = Date.now() + 110000, answer;
    // 202: the server is still preparing; the identical request is repeated until it answers.
    for (;;) {
      answer = await api('/v1/playback/sessions', {body: body, headers: {'Idempotency-Key': key}});
      if (answer.status !== 202) break;
      preparing();
      var delay = Math.min(10000, Math.max(250, Number(answer.body.retryAfterMs) || 1000));
      if (Date.now() + delay > deadline) throw new Error('preparation_timeout');
      await wait(delay);
    }
    stopPreparing();
    var started = answer.body;
    if (!started || !started.presentation) throw new Error('playback_not_ready');
    var artTimer, image;
    try { image = await Promise.race([art, new Promise(function(resolve){artTimer=setTimeout(function(){resolve('');},1500);})]); }
    finally { clearTimeout(artTimer); }
    var load;
    try { load = loadRequest(started, image, true); }
    catch (e) { await api(sessionPath(started.id), {method:'DELETE'}).catch(function(){}); throw e; }
    playback = track(started);
    reportSeq = 0; reportFailures = 0;
    return load;
  }
  function startPlayback(itemId,startSeconds,identity,subtitleTrackId,queue) { recoveries=0; return serial(async function(){try{var load=await resolvePlayback(itemId,startSeconds,identity,subtitleTrackId,queue);await loadMedia(load);showPlaying();scheduleReport(0);refreshCatalogueDisplay({id:playback.id,itemId:playback.itemId,presentation:{generation:playback.generation}},load.media.contentId);return playback;}catch(e){if(playback)await stopPlayback().catch(function(){});showError(e&&(e.code||e.message));throw e;}}); }

  // V1 music is progressive: the next server occurrence starts only after the
  // actual current media finishes. CAF preloadTime is a hint, not a buffer or
  // gapless promise; there is no private progressive PRELOAD or held response.
  async function advanceQueue() {
    if (!playback || !playback.queue) return false;
    var owner = playback;
    var head = (await api(queuePath(owner.queue.queueId),{method:'GET'})).body;
    if (playback !== owner || !head.next || !head.next.available) return false;
    var answer = await api(queuePath(owner.queue.queueId) + ':advance',{body:{reason:'completion'},headers:{'If-Match':head.revision,'Idempotency-Key':token(16)}});
    if (playback !== owner || !answer.body.session) return false;
    var current=answer.body.session;
    playback=track(current);meta=null;reportSeq=0;reportFailures=0;recoveries=0;
    showLoading(null); // Never label the new occurrence with the old title or artwork.
    var load;
    try { load=loadRequest(current,'',true);await loadMedia(load); }
    catch(e) { await stopPlayback().catch(function(){});showError(e&&(e.code||e.message));throw e; }
    showPlaying();scheduleReport(0);refreshCatalogueDisplay(current,load.media.contentId);broadcast();return true;
  }

  // --- Timeline: progress, lease renewal and observation in one report -------

  function timelineState() {
    var s = manager ? manager.getPlayerState() : 'IDLE';
    return s === 'PLAYING' ? 'playing' : s === 'BUFFERING' ? 'buffering' : 'paused';
  }
  async function report(state) {
    if (!playback) return;
    var id = playback.id;
    try {
      await api(sessionPath(id) + '/timeline', {body: {seq: ++reportSeq, generation: playback.generation, state: state || timelineState(), positionMs: Math.round(Math.max(0, position()) * 1000), rate: 1}});
      reportFailures = 0; notice('');
    } catch (e) {
      // The server no longer has this session (stopped elsewhere, moved, ended by an administrator),
      // or no longer accepts this TV's pairing: with no sender connected, nothing else would stop it.
      if (e.status === 404 || e.status === 410 || e.code === 'session_ended' || e.pairingEnded) { if (playback && playback.id === id) { playback = null; meta = null; if (manager) mediaCommand('STOP'); showIdle(); broadcast(); } return; }
      reportFailures++; // b4: transient engine events are silent.
      throw e;
    }
  }
  /** The next report: 10 s while playing, 30 s paused, sooner after a failure (the lease is 120 s). */
  function scheduleReport(ms) {
    clearTimeout(reportTimer);
    if (!playback) return;
    if (ms === undefined) ms = reportFailures ? [1000, 2000, 5000, 10000][Math.min(reportFailures, 4) - 1] : paused() ? 30000 : 10000;
    reportTimer = setTimeout(function () { serial(function () { return report(); }).catch(function () {}).finally(function () { scheduleReport(); }); }, ms);
  }
  /** Reload the media after the server produced a new presentation (a server seek, a recovery). */
  async function adoptPresentation(updated, autoplay) {
    if (!playback || updated.id !== playback.id) return;
    var fresh = updated.presentation.generation !== playback.generation;
    playback = track(updated);
    if (!fresh) return;
    await loadMedia(loadRequest(updated, '', autoplay));
    showPlaying();
  }
  /** PATCH the session. A 412 carries the server's session: adopt it and apply the change to it. */
  async function change(body) {
    if (!playback) return;
    var patch = function () { return api(sessionPath(playback.id), {method: 'PATCH', body: body, headers: {'If-Match': playback.revision, 'Content-Type': 'application/merge-patch+json'}}); };
    var answer;
    try { answer = await patch(); }
    catch (e) {
      if (e.status !== 412 || !e.current || !e.current.presentation) throw e;
      await adoptPresentation(e.current, !paused());
      answer = await patch();
    }
    await adoptPresentation(answer.body, !paused());
  }
  function seekTo(seconds) {
    return serial(async function () {
      if (!playback) return;
      var at = Math.max(0, seconds);
      if (!playback.serverSeek) { mediaCommand('SEEK',{currentTime:at}); return; }
      await change({seek: {positionMs: Math.round(at * 1000), id: token(16)}});
    });
  }
  function recover(code) {
    if(recovering||!playback)return;
    // Up to three silent recoveries; after them the drawn card says this TV can't play the title
    // (frame 13), rather than leaving a broken picture up with nobody able to stop it.
    if(recoveries>=3){showError(code);return;}
    recovering=true;recoveries++;broadcast();
    serial(async function(){var item=playback.itemId,at=position(),wasPaused=paused(),identity=meta;
      // The server remembers that this way of delivering the file failed here and re-plans the
      // same session another way, from where the picture stopped.
      try { await change({failure:{code:code,detail:''},positionMs:Math.round(Math.max(0,at)*1000)}); return; }
      catch (e) { if (e.code!=='unsupported_media'||code!=='stalled_without_data') throw (e.code==='unsupported_media'?new Error('playback_failed'):e); }
      // Nothing else to try and the stream only stalled: load the same title again.
      var load=await resolvePlayback(item,at,identity);load.autoplay=!wasPaused;await loadMedia(load);showPlaying();scheduleReport(0);
    }).then(function(){notice('');}).catch(function(e){notice('');showError(code==='decode_error'?'decode':e&&(e.code||e.message));}).finally(function(){recovering=false;broadcast();});
  }

  // --- Control: who may drive this TV (SEC-14, CAST-04, D-FEAT-10) ------------

  var controllerToken = '';
  var controllerSender = null;   // the sender that paired in this session
  var controllerPresent = false; // still connected
  var pendingPair = null;        // {senderId, requester, origin, result, timer}

  function send(senderId, body) { try { if (castContext && senderId) castContext.sendCustomMessage(NAMESPACE, senderId, body); } catch (e) { /* the sender left */ } }
  function requesterName(message, senderId) {
    var name = cleanText(message && message.displayName, 64);
    if (name) return name;
    // The SDK's own sender fact, not an inferred model: a sender that gave no
    // description but is an iPhone or iPad is named as one (frame 15).
    try {
      var sender = castContext && typeof castContext.getSender === 'function' ? castContext.getSender(senderId) : null;
      if (sender && /iPhone|iPad/.test(String(sender.userAgent || ''))) return copy('iPhoneOrIPad');
    } catch (e) { /* the sender left */ }
    return copy('someone');
  }
  function hideTakeover() { if (!viewCall('takeover', [null])) visible('takeover', false); }
  function takeoverView(controllerHere, remaining) {
    return viewCall('takeover', [{requester: pendingPair.requester, controllerPresent: controllerHere, timeoutMs: remaining, remoteKeysAvailable: remoteKeysAvailable()}]);
  }
  function askTakeover(requester, controllerHere) {
    var timeoutMs = controllerHere ? TAKEOVER_ASK_MS : TAKEOVER_ABSENT_MS;
    if (pendingPair) {
      pendingPair.viewDeadline = Date.now() + timeoutMs;
      pendingPair.viewControllerPresent = controllerHere;
      if (takeoverView(controllerHere, timeoutMs)) return;
    }
    text('takeover-title', copy('takeover.title', {requester:requester}));
    text('takeover-body', controllerHere ? copy('takeover.present') : copy('takeover.absent'));
    visible('takeover', true);
  }
  function discardPair(p) {
    if(p&&p.result&&p.result.device&&p.result.device.id&&p.result.session)request('/v1/cast/devices/'+encodeURIComponent(p.result.device.id),{method:'DELETE',origin:p.origin,bearer:p.result.session.accessToken,headers:{'If-Match':p.result.device.generation}}).catch(function(){});
  }
  function denyPending(code) {
    var p = pendingPair; if (!p) return;
    pendingPair = null; clearTimeout(p.timer); hideTakeover();discardPair(p);
    send(p.senderId, {type: 'pair-failed', code: code || 'tv_busy', message: pairingMessage({code: code || 'tv_busy'})});
  }
  function allowPending() {
    var p = pendingPair; if (!p) return;
    pendingPair = null; clearTimeout(p.timer); hideTakeover();
    serial(function () { return takeControl(p.senderId, p.origin, p.result); }).catch(function (error) {
      send(p.senderId, {type: 'pair-failed', code: error.code || '', message: pairingMessage(error)});
    });
  }
  async function takeControl(senderId, origin, result) {
    var previous = controllerSender;
    await switchTo(result, origin);
    controllerSender = senderId; controllerPresent = true;
    if (previous && previous !== senderId) send(previous, {type: 'replaced'});
    controllerToken=token(24);
    send(senderId, {type: 'paired', controlToken:controllerToken});
  }
  /** A pair request: redeem first (keeping the current pairing), then decide whether to switch now or ask. */
  function handlePair(senderId, message) {
    var named = serverOrigin(message.origin);
    if (named === null) { send(senderId, {type: 'pair-failed', code: 'origin_refused', message: pairingMessage({code:'origin_refused'})}); return; }
    var requester = requesterName(message, senderId);
    var code = String(message.code || '').trim().toUpperCase();
    redeemAt(named, code).then(function (result) {
      var sameViewer = !!(scope && result.scope && result.scope.accountId === scope.accountId && result.scope.profileId===scope.profileId && result.scope.authority===scope.authority && named === base);
      var busy = !!playback && controllerSender !== senderId && !sameViewer;
      if (!busy) return serial(function () { return takeControl(senderId, named, result); });
      if (pendingPair) denyPending('tv_busy');
      pendingPair = {senderId: senderId, requester: requester, origin: named, result: result, timer: null};
      send(senderId, {type: 'pair-pending'});
      var here = !!controllerSender && controllerPresent;
      askTakeover(requester, here);
      if (here) {
        send(controllerSender, {type: 'takeover-requested', requester: requester});
        pendingPair.timer = setTimeout(function () { denyPending('tv_busy'); }, TAKEOVER_ASK_MS);
      } else {
        pendingPair.timer = setTimeout(allowPending, TAKEOVER_ABSENT_MS);
      }
    }).catch(function (error) {
      send(senderId, {type: 'pair-failed', code: error.code || '', message: pairingMessage(error)});
    });
  }
  // The TV remote (where a Google TV delivers keys to the page): Select allows, Back declines.
  if (window.addEventListener) window.addEventListener('keydown', function (event) {
    if (['Enter',' ','Escape','Backspace','GoBack','BrowserBack','ArrowUp','ArrowDown','ArrowLeft','ArrowRight'].indexOf(event.key) >= 0 && !observedRemoteKeys) {
      observedRemoteKeys = true;
      if (pendingPair) takeoverView(pendingPair.viewControllerPresent, Math.max(0, pendingPair.viewDeadline - Date.now()));
      playingView();
    }
    if (!pendingPair) return;
    if (event.key === 'Enter' || event.key === ' ') allowPending();
    else if (event.key === 'Escape' || event.key === 'Backspace' || event.key === 'GoBack' || event.key === 'BrowserBack') denyPending('tv_busy');
  });
  function isController(senderId) { return !!controllerSender && senderId === controllerSender; }
  function statusBody() {
    var status = {
      type: 'status', paired: !!session, itemId: playback ? playback.itemId || '' : '',
      audioTracks: manager ? manager.getAudioTracksManager().getTracks().map(function(t){return {id:t.trackId,language:t.language||'',name:t.name||''};}) : [],
      textTracks: manager ? manager.getTextTracksManager().getTracks().map(function(t){return {id:t.trackId,language:t.language||'',name:t.name||''};}) : [],
      paused: paused(), state: recovering?'recovering':playback?(paused()?'paused':'playing'):'idle'
    };
    // A bookkeeping fallback zero is not a receiver clock fact.
    return Object.assign(status, clockFacts());
  }

  // --- Cast framework wiring ------------------------------------------------

  function attachCastFramework() {
    if (!window.cast || !window.cast.framework) { return; }
    var context = cast.framework.CastReceiverContext.getInstance();
    castContext=context; manager = context.getPlayerManager();
    // CAF's documented resolver keeps grant-bearing URLs out of its public media/queue status.
    manager.setMediaUrlResolver(mediaAddress);
    var types=cast.framework.messages.MessageType;
    // Media-channel commands from a sender that isn't the controller: pause and stop are allowed
    // (anyone in the room can do that with the remote); load, play and seek are not.
    function fromOther(request) { return !request || !isController(request.senderId); }
    // A LOAD carries only a Portico item id in customData. It never carries a
    // media URL, a bearer or a media grant: the receiver resolves everything
    // itself with its own credential.
    manager.setMessageInterceptor(types.LOAD, function (request) {
      if(request&&localRequests.has(request))return request;
      var custom = (request && request.media && request.media.customData) || {};
      if (!custom.itemId || fromOther(request)) { return null; }
      var identity = displayMeta(custom) || (request.media.metadata ? displayMeta({title: request.media.metadata.title, subtitle: request.media.metadata.subtitle || request.media.metadata.seriesTitle}) : null);
      return serial(function(){recoveries=0;return resolvePlayback(custom.itemId,custom.startSeconds,identity);}).then(function(load){showPlaying();scheduleReport(0);return load;}).catch(function(e){showError(e&&(e.code||e.message));return null;});
    });
    // Play and pause are the player's own; the timeline reports them. A seek the server must
    // produce again is sent to it and the new presentation loaded here instead.
    // Progressive audio cannot use CAF's adaptive preload. No speculative URL
    // or server preparation is produced; ordinary completion owns the transition.
    if (types.PRELOAD) manager.setMessageInterceptor(types.PRELOAD, function(){return null;});
    // Standard queue mutation commands must pass the same controller gate as LOAD.
    ['GET_STATUS','QUEUE_GET_ITEMS','QUEUE_GET_ITEM_IDS'].forEach(function(name){if(types[name])manager.setMessageInterceptor(types[name],function(request){return fromOther(request)?null:request;});});
    // CAF maps QUEUE_NEXT/PREV to QUEUE_UPDATE (jump +/-1); those enum values
    // are not accepted by setMessageInterceptor, even though the SDK exposes them.
    ['QUEUE_INSERT','QUEUE_UPDATE','QUEUE_REMOVE','QUEUE_REORDER','QUEUE_LOAD'].forEach(function(name){if(types[name])manager.setMessageInterceptor(types[name],function(){return null;});});
    manager.setMessageInterceptor(types.PAUSE,function(request){return request;});
    manager.setMessageInterceptor(types.PLAY,function(request){return request&&localRequests.has(request)||!fromOther(request)?request:null;});
    manager.setMessageInterceptor(types.SEEK,function(request){
      if(request&&localRequests.has(request))return request;
      if (fromOther(request) || !playback || !Number.isFinite(request.currentTime)) return null;
      if (!playback.serverSeek) return request;
      seekTo(request.currentTime).catch(function(){notice(copy('notice.seekFailed'), 5000);});
      return null;
    });
    manager.setMessageInterceptor(types.STOP,function(request){if(request&&localRequests.has(request))return request;return serial(stopPlayback).then(function(){return request;}).catch(function(){return null;});});
    // A Cast device has no keyboard, so the sender delivers the pairing code it was issued
    // and then drives playback over this channel. What plays is always the library of the
    // person who paired, and only they drive it.
    context.addCustomMessageListener(NAMESPACE, function (event) {
      var message = event.data || {};
      if (typeof message === 'string') { try { message = JSON.parse(message); } catch (e) { message = {}; } }
      var senderId = event.senderId;
      var reply = function (body) { send(senderId, body); };
      if(message.type==='status'&&session&&controllerToken&&typeof message.controlToken==='string'&&message.controlToken.length===48&&message.controlToken===controllerToken){
        controllerSender=senderId;controllerPresent=true;
        if(pendingPair){clearTimeout(pendingPair.timer);askTakeover(pendingPair.requester,true);pendingPair.timer=setTimeout(function(){denyPending('tv_busy');},TAKEOVER_ASK_MS);}
      }
      if (message.type === 'pair') { handlePair(senderId, message); return; }
      if (message.type === 'takeover') {
        if (isController(senderId) && pendingPair) { if (message.allow === true) allowPending(); else denyPending('tv_busy'); }
        return;
      }
      if (!isController(senderId)) {
        // Nothing about what's playing, and no control, for a sender that didn't pair.
        if (message.type === 'pause' && playback) { mediaCommand('PAUSE'); return; }
        if (message.type === 'stop' && playback) { serial(stopPlayback).catch(function(){}); return; }
        reply(controllerSender ? {type: 'busy'} : {type: 'pair-required'});
        return;
      }
      controllerPresent=true;
      switch (message.type) {
        case 'load':
          startPlayback(String(message.itemId || ''), typeof message.startSeconds === 'number' ? message.startSeconds : undefined, displayMeta(message), message.subtitleTrackId, queueRef(message.queue)).then(function () {
            if (typeof message.requestId === 'string' && /^[A-Za-z0-9._:-]{1,128}$/.test(message.requestId)) reply({type:'loaded',requestId:message.requestId,itemId:playback ? playback.itemId : ''});
            broadcast();
          }).catch(function (error) {
            reply({ type: 'load-failed', requestId: typeof message.requestId==='string'?message.requestId.slice(0,128):undefined, code: error.code || error.message || '' });
          });
          break;
        case 'play': if (playback) mediaCommand('PLAY'); break;
        case 'pause': if (playback) mediaCommand('PAUSE'); break;
        case 'seek': if(Number.isFinite(message.positionSeconds)) seekTo(message.positionSeconds).catch(function(){notice(copy('notice.seekFailed'), 5000);}); break;
        case 'audio': if(Number.isSafeInteger(message.trackId)) manager.getAudioTracksManager().setActiveById(message.trackId); break;
        case 'subtitles': if(Array.isArray(message.trackIds)&&message.trackIds.length<=1&&message.trackIds.every(Number.isSafeInteger)) manager.getTextTracksManager().setActiveByIds(message.trackIds); break;
        case 'stop': serial(stopPlayback).catch(function(){notice(copy('notice.stopFailed'), 5000);}); break;
        case 'status': break;
        default: return;
      }
      broadcast();
    });
    // Status goes only to the controller (SEC-14).
    broadcast = function () { if (controllerSender && controllerPresent) send(controllerSender, statusBody()); };
    var events=cast.framework.events.EventType;
    manager.addEventListener(events.ERROR,function(e){var code=Number(e.detailedErrorCode);if(code>=300&&code<500)recover('decode_error');});
    manager.addEventListener(events.BUFFERING,function(e){viewBuffering=e.isBuffering===true;playingView();clearTimeout(bufferingTimer);if(e.isBuffering)bufferingTimer=setTimeout(function(){recover('stalled_without_data');},20000);});
    // The title is over whether or not its last report gets through: the TV must not stay on a
    // finished title, still reporting, with nobody connected to stop it.
    manager.addEventListener(events.MEDIA_FINISHED,function(event){
      if(!event||event.endedReason!==cast.framework.events.EndedReason.END_OF_STREAM)return;
      var owner=playback;
      if(!owner)return;
      serial(async function(){
        // Duplicate/stale completion queued before a new occurrence cannot end it.
        if(playback!==owner)return;
        if(await advanceQueue())return;
        try{await report('ended');}finally{notice('');await stopPlayback(true);}
      }).catch(function(){serial(function(){if(playback===owner)return stopPlayback(true);}).catch(function(){});});
    });
    if(events.PLAYER_LOADING)manager.addEventListener(events.PLAYER_LOADING,function(){
      pictureReady=false;viewBuffering=false;activeTrackKey=null;
      if(!viewLoading){if(meta&&meta.title)showLoading(meta);else viewCall('splash');}
    });
    // LOADED_DATA is the SDK's first-frame-loaded event; PLAYING confirms playback start.
    [events.LOADED_DATA,events.PLAYING].filter(Boolean).forEach(function(name){manager.addEventListener(name,function(){pictureReady=true;viewLoading=false;stopPreparing();playingView(name===events.PLAYING);scheduleReport(0);broadcast();});});
    // A state change or a completed seek is reported at once.
    [events.PAUSE,events.SEEKED].filter(Boolean).forEach(function(name){manager.addEventListener(name,function(){playingView(true);scheduleReport(0);broadcast();});});
    [events.TIME_UPDATE,events.DURATION_CHANGE,events.MEDIA_STATUS].filter(Boolean).forEach(function(name){manager.addEventListener(name,function(){playingView();});});
    context.addEventListener(cast.framework.system.EventType.SENDER_DISCONNECTED,function(e){
      if (e && e.senderId && e.senderId === controllerSender) {
        controllerPresent = false;
        // Someone was waiting for the controller's answer: with the controller gone, the TV says who is taking over, then switches.
        if (pendingPair) { clearTimeout(pendingPair.timer); askTakeover(pendingPair.requester, false); pendingPair.timer = setTimeout(allowPending, TAKEOVER_ABSENT_MS); }
      }
      if (pendingPair && e && e.senderId === pendingPair.senderId) { clearTimeout(pendingPair.timer); discardPair(pendingPair);pendingPair = null; hideTakeover(); }
      // Nobody is connected: the TV has no controller, but it goes on playing. A phone that sleeps or
      // leaves Wi-Fi drops its connection; only Stop, the end of the title or the server ends playback.
      if(context.getSenders().length===0){controllerPresent=false;}
    });
    var statusInterval=setInterval(function(){if(playback){broadcast();playingView();}},2000);
    if(cast.framework.system.EventType.SHUTDOWN)context.addEventListener(cast.framework.system.EventType.SHUTDOWN,function(){
      shuttingDown=true;activeRequests.forEach(function(active){clearTimeout(active.timer);active.abort.abort();});activeRequests.clear();
      resetViewPlayback();clearInterval(statusInterval);clearTimeout(reportTimer);clearTimeout(bufferingTimer);clearTimeout(errorTimer);clearTimeout(noticeTimer);if(pendingPair)clearTimeout(pendingPair.timer);
    });
    // The standard idle timeout closes the app when nothing is playing and nobody is connected (CAST-01 §5).
    context.start({ customNamespaces: (function () { var n = {}; n[NAMESPACE] = cast.framework.system.MessageType.JSON; return n; })() });
  }

  showIdle();
  attachCastFramework();

  // Exposed for the server's own smoke test page and for manual diagnosis.
  window.porticoCastReceiver = {
    buildProfile: buildProfile,
    probeAudioOutput: probeAudioOutput,
    redeem: redeem,
    reconnect: reconnect,
    startPlayback: startPlayback,
    state: function () { return { paired: !!session, scope: scope, playbackId: playback && playback.id, controller: controllerSender, pending: pendingPair ? pendingPair.senderId : null }; }
  };
})();
