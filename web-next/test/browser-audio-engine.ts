import {BrowserEqualizer, eqFrequencies, flatEqualizer} from '../src/platform/playback/audio/equalizer';
import {EqualizerStore} from '../src/platform/playback/audio/equalizer-store';
import {Deck, linkDecks} from '../src/platform/playback/audio/engine';
import {qualifyBrowserDecoders} from '../src/platform/playback/audio/qualification';
import {WEBCODECS_SKIP, browserEngine} from '../src/platform/playback/audio/track';
import {DecodeAudioRender} from '../src/platform/playback/DecodeAudioRender';
import {ByteCache} from '../src/platform/playback/audio/bytes';
import flacUrl from '../../fixtures/audio-gapless/album-flac-01.flac?url';
import flacPlan from '../../fixtures/audio-gapless/album-flac-01.json';

const check = (ok: unknown, message: string) => { if (!ok) throw new Error(message); };
const rms = (data: Float32Array, from = 24000) => Math.sqrt(data.subarray(from).reduce((sum, value) => sum + value * value, 0) / (data.length - from));

export async function runBrowserAudioChecks() {
  const report: Record<string, unknown> = {userAgent: navigator.userAgent};
  // Render actual Web Audio nodes. Compare the audio, not a second implementation of a biquad.
  const response: number[] = [];
  for (let band = 0; band < eqFrequencies.length; band++) {
    const context = new OfflineAudioContext(1, 48000, 48000), eq = new BrowserEqualizer(context);
    const bandsDb = eqFrequencies.map((_, i) => i === band ? 6 : 0);
    eq.apply({enabled:true,preampDb:-3,bandsDb}); eq.output.connect(context.destination);
    const oscillator = context.createOscillator(); oscillator.frequency.value = eqFrequencies[band]!;
    const attenuation = context.createGain(); attenuation.gain.value = 0.05;
    oscillator.connect(attenuation); attenuation.connect(eq.input); oscillator.start();
    const data = (await context.startRendering()).getChannelData(0);
    const db = 20 * Math.log10(rms(data) / (0.05 / Math.SQRT2));
    check(Math.abs(db - 3) < 0.16, `band ${band}: ${db} dB instead of 3`);
    response.push(db); eq.dispose();
  }
  report.eqNetDbAtCentres = response;

  const context = new OfflineAudioContext(6, 4096, 48000), eq = new BrowserEqualizer(context);
  eq.apply(flatEqualizer); eq.output.connect(context.destination);
  const source = context.createBufferSource(), input = context.createBuffer(6, 4096, 48000);
  for (let ch = 0; ch < 6; ch++) { const data = input.getChannelData(ch); for (let i = 0; i < data.length; i++) data[i] = (ch + 1) / 16 * Math.sin(2 * Math.PI * (ch + 1) * 100 * i / 48000); }
  source.buffer = input; source.connect(eq.input); source.start();
  const bypass = await context.startRendering();
  for (let ch = 0; ch < 6; ch++) check(bypass.getChannelData(ch).every((value, i) => value === input.getChannelData(ch)[i]), `off EQ changed channel ${ch}`);
  report.sixChannelOffEqExact = true; eq.dispose();

  // Actual scheduler -> AudioBufferSourceNodes -> EQ bypass -> OfflineAudioContext, adjacent frames.
  const joined = new OfflineAudioContext(2, 24000, 48000), graph = new BrowserEqualizer(joined);
  graph.output.connect(joined.destination);
  const pcm = (frames: number, value: number) => {
    let at = 0;
    return {sampleRate:48000,channels:2,close(){},async read(max: number) {
      const count = Math.min(max, frames - at); if (!count) return;
      at += count; return {frames:count, channels:[new Float32Array(count).fill(value),new Float32Array(count).fill(-value)]};
    }};
  };
  const a = new Deck(joined, graph.input, pcm(10001,0.125), {frames:10001,gain:1});
  const b = new Deck(joined, graph.input, pcm(13999,0.25), {frames:13999,gain:1});
  await a.pump(1); await b.pump(1); a.start(0);
  const edge = linkDecks(joined,a,b,{crossfadeSeconds:0,sameAlbum:true,earliest:0});
  check(edge.start === 10001, 'nonexact scheduled edge');
  const audio = await joined.startRendering();
  for (let ch = 0; ch < 2; ch++) check(audio.getChannelData(ch).every((value,i) => value === (i < 10001 ? 0.125 : 0.25) * (ch ? -1 : 1)), 'gap or overlap at actual render boundary');
  report.gaplessJoinFrame = edge.start; a.close(); b.close(); graph.dispose();

  // Real IndexedDB transactions: output-specific state survives closing and re-opening the DB.
  const database = 'portico-audio-conformance-' + Date.now() + '-' + Math.random().toString(36).slice(2);
  let store = new EqualizerStore(indexedDB,database);
  const saved = {enabled:true,preampDb:-4,bandsDb:[1,2,3,4,0,-1,-2,-3,-4,-5]};
  try {
    await store.save('output-a',saved); await store.close(); store = new EqualizerStore(indexedDB,database);
    check(JSON.stringify(await store.load('output-a')) === JSON.stringify(saved), 'output state did not survive re-open');
    check((await store.load('output-b')).enabled === false, 'EQ leaked between outputs');
    let rejected = false; try { await store.save('output-b',{...saved,bandsDb:[NaN]}); } catch { rejected = true; }
    check(rejected, 'invalid equalizer settings accepted');
    const db = await new Promise<IDBDatabase>((resolve,reject) => { const r=indexedDB.open(database);r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error); });
    const tx=db.transaction(['outputs','bands'],'readonly');
    const rows=await new Promise<number>((resolve,reject)=>{const r=tx.objectStore('bands').count();r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error);});
    db.close();check(rows===10,'band rows missing');report.persistedBandRows=rows;
  } finally { await store.close(); await new Promise<void>((resolve,reject)=>{const r=indexedDB.deleteDatabase(database);r.onsuccess=()=>resolve();r.onerror=()=>reject(r.error);}); }
  await qualifyBrowserDecoders(); report.qualifiedDecoderSkips = {...WEBCODECS_SKIP[browserEngine()]};
  // The actual renderer consumes a real generated FLAC fixture and reports only confirmed facts.
  // Silence the output; this is decoder/graph/clock evidence, never physical speaker/DAC proof.
  const bytes = new Uint8Array(await (await fetch(flacUrl)).arrayBuffer()), cache = new ByteCache();
  cache.put('browser-proof:0',bytes);
  const element = document.createElement('video'); element.muted = true;
  const snapshot:any = {session:{id:'browser-proof-session',generation:1,audio:{...flacPlan,version:2,mode:'direct',id:'browser-proof',url:flacUrl,bytes:bytes.length}},
    phase:'ready',intent:'paused',intentId:1,positionSeconds:0,audioEffects:{rendering:true,rate:1,settings:{gapless:false,crossfadeSeconds:0,normalization:'off'},notice:''}};
  let ready = false, failure = '';
  const service:any = {getSnapshot:()=>snapshot,ready:()=>{ready=true;},audioRenderFailed:(message:string)=>{failure=message;},
    audioEffectsNotice(){},audioRenderStatus(){},setDeviceAudioProcessing(){},engineProgress(){},fact(){},pause(){}};
  const render = new DecodeAudioRender(service,path=>path,element,()=>false,cache);
  try {
    check(render.audioFacts() === undefined,'facts published before decoded renderer exists');
    render.apply(snapshot);
    const until=performance.now()+5000;
    while(!ready&&!failure&&performance.now()<until) await new Promise(resolve=>setTimeout(resolve,20));
    check(ready&&!failure,'actual FLAC renderer did not start: '+failure);
    const off=render.audioFacts()!;check(off.bitPerfect===false&&!off.eqEnabled&&off.sourceRate===44100&&off.deviceRate===null,'renderer invented output facts');
    await render.setEqualizer({...flatEqualizer,enabled:true,bandsDb:[0,0,0,0,0,3,0,0,0,0]});
    check(render.audioFacts()!.eqEnabled,'persisted choice never reached actual renderer');
    await render.setEqualizer(flatEqualizer);check(!render.audioFacts()!.eqEnabled,'EQ off did not reach graph');
    report.actualRendererFacts=render.audioFacts();
  } finally {
    render.dispose(); cache.clear();
    await new Promise<void>((resolve,reject)=>{const request=indexedDB.deleteDatabase('portico-audio-output-v1');request.onsuccess=()=>resolve();request.onerror=()=>reject(request.error);});
  }
  return report;
}
