import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {parsePreferenceSnapshot, type PreferenceSnapshot} from '@core/preferences.ts';
import {subtitlePreferences} from '@core/preference-effects.ts';
import {SubtitleSurface, type TextTrackLike} from '../src/platform/playback/subtitle-surface.ts';

type Cue = {start: number; end: number; text: string; align: string; snapToLines: boolean; line: number | string};
function host() {
  const cues: Cue[] = [];
  const track: TextTrackLike & {cueList: Cue[]} = {
    mode: 'disabled', cueList: cues,
    get cues() { return this.mode === 'disabled' ? null : cues; },
    addCue(c: unknown) { cues.push(c as Cue); },
    removeCue(c: unknown) { cues.splice(cues.indexOf(c as Cue), 1); },
  };
  const other = {kind: 'subtitles', mode: 'showing'};
  let made = 0;
  const listeners = new Map<string, () => void>();
  return {
    track, cues, other, made: () => made, listeners,
    addTextTrack() { made++; return track; },
    textTracks: Object.assign([track as unknown as {kind: string; mode: string}, other], {addEventListener: (type: string, fn: () => void) => { listeners.set(type, fn); }, removeEventListener: (type: string) => { listeners.delete(type); }}),
  };
}
const cue = (start: number, end: number, text: string): Cue => ({start, end, text, align: '', snapToLines: false, line: 'auto'});
const lines = [{startUs: 1_000_000, endUs: 3_000_000, text: 'Hello <there> & you'}, {startUs: '5000000', endUs: '7000000', text: 'Sign', top: true, italic: true}];

test('one track, owned by the surface, shows the cues shifted by the viewer’s timing and the stream’s offset', () => {
  const h = host();
  const surface = new SubtitleSurface(h, {cue, lift: false});
  surface.show(lines, 0.5);
  assert.equal(h.made(), 1);
  assert.equal(h.track.mode, 'showing');
  assert.deepEqual(h.cues.map(c => [c.start, c.end]), [[1.5, 3.5], [5.5, 7.5]]);
  assert.equal(h.cues[0]!.text, 'Hello &lt;there&gt; &amp; you', 'cue text is text, never markup');
  assert.equal(h.cues[1]!.text, '<i>Sign</i>');
  assert.equal(h.cues[1]!.line, 0, 'a cue authored at the top stays there');
  // The stream sits 1.4 s later on the element's clock: the lines move, once, without duplicates.
  surface.timeline(1.4);
  assert.deepEqual(h.cues.map(c => c.start), [2.9, 6.9]);
  surface.timeline(1.4);
  assert.equal(h.cues.length, 2);
  assert.equal(h.made(), 1, 'the track is reused');
});

test('nothing is shown while the stream has not said where it sits, and clearing takes the lines off', () => {
  const h = host();
  const surface = new SubtitleSurface(h, {cue, lift: false});
  surface.reset();
  surface.show(lines, 0);
  assert.equal(h.track.mode, 'disabled');
  assert.equal(h.cues.length, 0);
  surface.timeline(0);
  assert.equal(h.cues.length, 2);
  surface.clear();
  assert.equal(h.track.mode, 'disabled');
  assert.equal(h.cues.length, 0);
});

test('where the cue box cannot be lifted with CSS, cues sit above the controls; an engine’s own renditions stay off', () => {
  const h = host();
  const surface = new SubtitleSurface(h, {cue, lift: true});
  surface.show(lines, 0);
  assert.equal(h.cues[0]!.line, -4);
  assert.equal(h.cues[1]!.line, 0);
  surface.guard(true);
  assert.equal(h.other.mode, 'disabled');
  h.other.mode = 'showing';
  h.listeners.get('change')!();
  assert.equal(h.other.mode, 'disabled');
  surface.dispose();
  assert.equal(h.listeners.size, 0);
  assert.equal(h.cues.length, 0);
});

test('drawn caption preferences apply to the native cues and system opt-in removes app styling', () => {
  const h=host(), attrs=new Map<string,string>(), styles:{textContent:string;removed:boolean;remove():void}[]=[];
  const styled={...h,setAttribute:(key:string,value:string)=>attrs.set(key,value),removeAttribute:(key:string)=>attrs.delete(key),ownerDocument:{
    createElement:()=>{const style={textContent:'',removed:false,remove(){this.removed=true;}};styles.push(style);return style;},head:{append(){}}
  }};
  const surface=new SubtitleSurface(styled as any,{cue,lift:false});
  const preferences={size:'large',color:'yellow',background:'outline',position:'raised',font:'mono',keepStyling:false,useSystemCaptionStyle:false} as const;
  surface.applyPreferences(preferences);surface.show(lines,0);
  assert.match(styles[0]!.textContent,/font-size:125%/);assert.match(styles[0]!.textContent,/color:#ff0/);assert.match(styles[0]!.textContent,/font-family:monospace/);
  assert.ok(attrs.has('data-portico-caption-style'));assert.equal(h.cues[1]!.text,'Sign');assert.equal(h.cues[1]!.line,-6);
  surface.applyPreferences({...preferences,useSystemCaptionStyle:true,keepStyling:true});
  assert.equal(styles[0]!.removed,true);assert.equal(attrs.has('data-portico-caption-style'),false,'browser native cue defaults are left to the actual user agent');
  assert.equal(h.cues[1]!.text,'<i>Sign</i>');assert.equal(h.cues[1]!.line,0,'authored top cue is preserved');
  surface.dispose();assert.equal(h.cues.length,0);
});

test('confirmed canonical preference snapshots restyle the same live track without losing its clock', () => {
  const published=JSON.parse(readFileSync(new URL('../../server/testdata/contracts/get-bootstrap.json',import.meta.url),'utf8')).response.preferences;
  let confirmed: PreferenceSnapshot=parsePreferenceSnapshot(published);
  const update=(values:Record<string,string|boolean>)=>{
    const raw=structuredClone(published);
    for(const [key,value] of Object.entries(values)) {
      const field=raw.registry.fields.find((field:{key:string})=>field.key===key);
      assert.ok(field,'the real producer must publish every preference used by this test');
      const document=[...raw.documents].reverse().find(document=>field.scopes.includes(document.scope));
      assert.ok(document,'the real snapshot must carry an eligible preference document');
      document.values[key]=value;document.revision++;document.activeRevision=document.revision;
      raw.effective[key]=value;raw.effectiveSource[key]=document.scope;
    }
    confirmed=parsePreferenceSnapshot(raw);
  };
  const h=host(),attrs=new Map<string,string>(),styles:{textContent:string;remove():void}[]=[];
  const styled={...h,setAttribute:(key:string,value:string)=>attrs.set(key,value),removeAttribute:(key:string)=>attrs.delete(key),ownerDocument:{
    createElement:()=>{const style={textContent:'',remove(){}};styles.push(style);return style;},head:{append(){}}
  }};
  const surface=new SubtitleSurface(styled as any,{cue,lift:false});
  surface.applyPreferences(subtitlePreferences(confirmed));surface.show(lines,.5);surface.timeline(1.25);
  const track=h.track,key=attrs.get('data-portico-caption-style');
  update({'playback.subtitleSize':'extra-large','playback.subtitleColor':'pink','playback.subtitleBackground':'opaque','playback.subtitleFont':'rounded','playback.subtitlePosition':'top','playback.keepSubtitleStyling':false});
  surface.applyPreferences(subtitlePreferences(confirmed));
  assert.equal(h.track,track);assert.equal(h.made(),1);assert.equal(attrs.get('data-portico-caption-style'),key);
  assert.deepEqual(h.cues.map(c=>[c.start,c.end]),[[2.75,4.75],[6.75,8.75]]);
  assert.equal(h.cues[1]!.text,'Sign');assert.equal(h.cues[0]!.line,0);
  assert.match(styles.at(-1)!.textContent,/font-size:150%/);assert.match(styles.at(-1)!.textContent,/color:#ff80c0/);
  assert.match(styles.at(-1)!.textContent,/background-color:#000/);assert.match(styles.at(-1)!.textContent,/font-family:ui-rounded,system-ui,sans-serif/);
  update({'playback.subtitleSize':'small','playback.subtitleColor':'cyan','playback.subtitleBackground':'shadow','playback.subtitleFont':'serif','playback.subtitlePosition':'raised'});
  surface.applyPreferences(subtitlePreferences(confirmed));
  assert.equal(h.cues[0]!.line,-6);assert.equal(h.cues[1]!.text,'<i>Sign</i>');assert.equal(h.cues[1]!.line,0);
  assert.match(styles.at(-1)!.textContent,/font-size:80%/);assert.match(styles.at(-1)!.textContent,/color:#0ff/);
  assert.match(styles.at(-1)!.textContent,/font-family:serif/);assert.match(styles.at(-1)!.textContent,/text-shadow:0 2px 3px #000/);
  update({'playback.useSystemCaptionStyle':true,'playback.subtitlePosition':'top'});
  surface.applyPreferences(subtitlePreferences(confirmed));
  assert.equal(attrs.has('data-portico-caption-style'),false,'system opt-in delegates to the user agent; it cannot read OS caption settings');
  assert.equal(h.cues[0]!.line,'auto');assert.equal(h.made(),1);
  surface.dispose();assert.equal(h.cues.length,0);
});
