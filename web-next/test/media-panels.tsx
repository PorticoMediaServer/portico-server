import {useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {Icon, IconSprite} from '../src/kit/Icon';
import {PlaybackInfo, type PlaybackInfoCopy, type PlaybackInfoProjection} from '../src/kit/media/PlaybackInfo';
import {PlayOnButton, PlayOnPanel} from '../src/kit/player/PlayOnPanel';
import type {PlayOnSnapshot} from '../src/platform/playback/play-on';
import '../src/kit/tokens.css';
import '../src/kit/fonts.css';
import '../src/bootstrap.css';

if (!import.meta.env.DEV) throw new Error('Media panel comparison requires the development server.');

// Render-only golden-frame input. This is not a server/session/output producer or a player host.
const copy: PlaybackInfoCopy = {
  title:'Playback info', close:'Close playback info', back:'Back', route:'Route', inFile:'In the file', playingHere:'Playing here',
  routes:{original:'Original',repackaged:'Repackaged',converted:'Converted'},
  sections:{picture:'Picture',sound:'Sound',subtitles:'Subtitles'},
  fields:{container:'Container',codec:'Codec',hdr:'HDR',bitDepth:'Bit depth',resolution:'Resolution',frameRate:'Frame rate',screen:'Screen',format:'Format',channels:'Channels',sampleRate:'Sample rate',output:'Output',track:'Track',shown:'Shown'},
  routeName:value=>`Route: ${value}`,originalValue:value=>`In the file: ${value}`,
};
const projection: PlaybackInfoProjection = {
  itemTitle:'Gladiator',deviceName:'Chrome on Windows',route:'converted',
  explanation:"This browser can't play Dolby Vision or Dolby Digital Plus, so the server converts both.",network:'Home network',bitrate:'21 Mbps',
  sections:[
    {kind:'picture',route:'converted',rows:[
      {field:'container',source:'MKV',delivered:'MP4',changed:true},
      {field:'codec',source:'HEVC Main 10',delivered:'HEVC Main 10',changed:false},
      {field:'hdr',source:'Dolby Vision profile 5',delivered:'HDR10, converted from Dolby Vision profile 5',changed:true},
      {field:'bitDepth',source:'10-bit',delivered:'10-bit',changed:false},
      {field:'resolution',source:'3840 × 2160',delivered:'3840 × 2160',changed:false},
      {field:'frameRate',source:'23.976 fps',delivered:'23.976 fps',changed:false},
    ],losses:["Dolby Vision isn't available in this browser."]},
    {kind:'sound',route:'converted',rows:[
      {field:'format',source:'Dolby Digital Plus with Atmos',delivered:'AAC',changed:true},
      {field:'channels',source:'5.1',delivered:'5.1',changed:false},
      {field:'sampleRate',source:'48 kHz',delivered:'48 kHz',changed:false},
      {field:'output',delivered:'Decoded by this browser',changed:false},
    ],losses:["Atmos isn't available in this browser."]},
    {kind:'subtitles',route:'original',rows:[
      {field:'track',source:'English, SRT',delivered:'English, SRT',changed:false},
      {field:'shown',delivered:'Drawn by this browser',changed:false},
    ],losses:[]},
  ],
};
function Comparison() {
  const [open,setOpen]=useState(false),returnFocusRef=useRef<HTMLButtonElement>(null);
  const playOn=new URLSearchParams(location.search).get('panel')==='play-on';
  const snapshot:PlayOnSnapshot={cast:{available:true,phase:'off',positionSeconds:0,durationSeconds:0,paused:false,audioTracks:[],textTracks:[]},airPlayAvailable:false,panel:open?'play-on':null,disposed:false};
  return <><IconSprite /><div data-comparison-background="" style={{position:'fixed',inset:0,pointerEvents:'none'}} />
    {playOn ? <><div style={{position:'fixed',right:82,top:24}}><PlayOnButton available controller={{openPlayOn:()=>setOpen(true)}} buttonRef={returnFocusRef} /></div>
      <PlayOnPanel controller={{closePanel:()=>setOpen(false),startAirPlay(){},async startCast(){return false;}}} snapshot={snapshot} phone={false} anchorRef={returnFocusRef} reportError={error=>{throw error;}} /></> : <>
      <button ref={returnFocusRef} type="button" aria-label="More" onClick={()=>setOpen(true)} style={{position:'fixed',left:40,top:40,width:48,height:48,display:'grid',placeItems:'center'}}><Icon name="i-more" size={22} /></button>
      <PlaybackInfo projection={projection} copy={copy} open={open} onOpenChange={setOpen} returnFocusRef={returnFocusRef} />
    </>}
  </>;
}
createRoot(document.getElementById('root')!).render(<Comparison />);
