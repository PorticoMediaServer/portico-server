import {useEffect,useMemo,useRef,useSyncExternalStore} from 'react';
import {LyricsService,activeLyricLine,lyricSeekPosition} from '@core/lyrics.ts';
import type {HttpLocalApi,PlaybackService} from '@core/index.ts';
import type {ContentScope} from '@core/library-content.ts';
import {Text,Button} from '../ui';

export function Lyrics({api,scope,service,libraryId,className}:{api:HttpLocalApi;scope:ContentScope;service:PlaybackService;libraryId:string;/** The column beside the cover on Now Playing; without it the lyrics are a short strip. */className?:string}) {
 const playback=useSyncExternalStore(service.subscribe,service.getSnapshot);
 const reader=useMemo(()=>playback.session&&playback.itemId?new LyricsService(api,scope,{libraryId,itemId:playback.itemId,sessionId:playback.session.id,sessionGeneration:playback.session.generation}):null,[api,scope.serverId,scope.viewerId,libraryId,playback.itemId,playback.session?.id,playback.session?.generation]);
 const phase=useRef(playback.phase);phase.current=playback.phase;
 useEffect(()=>{if(!reader)return;const refresh=()=>{if(!['ended','error','idle'].includes(phase.current))void reader.refresh();};refresh();const timer=setInterval(refresh,30000);return()=>{clearInterval(timer);reader.dispose();};},[reader]);
 const data=useSyncExternalStore(reader?.subscribe??(()=>()=>{}),reader?.getSnapshot??(()=>null));
 const selected=data?.data?.selection.resource,doc=selected?.document,source=data?.data?.source;
 if(!reader)return null;
 // Beside the cover: a track without lyrics takes no column, so the cover stays centred.
 if(className&&!doc&&!data?.error&&!data?.data?.resources.filter(r=>!r.deleted).length)return null;
 const current=doc?activeLyricLine(doc,playback.positionSeconds,selected?.offsetMs??0,source?.startSeconds??0):-1;
 return <section aria-label="Lyrics" className={className} style={className?undefined:{maxHeight:'30vh',overflowY:'auto',width:'min(600px,90vw)',textAlign:'center',padding:12}}>
  {data?.error?<><Text variant="caption">{data.error}</Text><Button size="sm" variant="ghost" label="Retry lyrics" onClick={()=>void reader.refresh()}/></>:null}
  {!doc&&!data?.busy?<Text variant="caption" tone="secondary">{data?.data?.resources.length?'Choose lyrics':'No lyrics available for this track.'}</Text>:null}
  {!doc?data?.data?.resources.filter(r=>!r.deleted).map(r=><Button key={r.id} size="sm" variant="ghost" label={r.provenance.label||r.language} onClick={()=>void reader.choose(r)}/>):null}
  {doc?.lines.map((line,i)=>{const seek=source?lyricSeekPosition(line,selected?.offsetMs??0,source):null;const active=current>=0&&line.atMs===doc.lines[current].atMs;return <button key={i} type="button" disabled={seek===null} aria-current={active?'true':undefined} onClick={()=>{if(seek!==null)service.seek(seek);}} style={{display:'block',width:'100%',background:'transparent',border:0,color:'inherit',font:'inherit',whiteSpace:'pre-line',padding:'8px 12px',opacity:active?1:.55,fontWeight:active?700:400,cursor:seek===null?'default':'pointer'}}>{line.text||'♪'}</button>;})}
 </section>;
}
