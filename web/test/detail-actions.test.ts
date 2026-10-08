import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as presentation from '@core/presentation/index.ts';
import * as recommendationsModule from '@core/recommendations.ts';
import * as homeModule from '@core/home.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';
const ui = Object.fromEntries(['AnchoredMenu','Artwork','Backdrop','Button','Card','Inset','Menu','Notice','Page','Section','Shelf','Skeleton','StateView','Surface','Text','Input','ListRow','Loading','MenuHeading','MenuRow','MenuSeparator','TitleHero','Dialog','KeyValue','Icon'].map(x=>[x,x]));
function nodes(tree:any):any[]{if(!tree||typeof tree!=='object')return[];if(Array.isArray(tree))return tree.flatMap(nodes);return[tree,...nodes(tree.props?.children),...nodes(tree.props?.action),...nodes(tree.props?.actions),...nodes(tree.props?.secondary)];}
const tick=()=>new Promise(r=>setTimeout(r,0));
test('Detail routes advertised commands to the exact existing picker or source section',async()=>{
 const h=hooks();const anchor={x:10,y:20,width:100,height:30};let scrolled=0,focused=0,played=0;
 const item={id:'item',libraryId:'library',kind:'movie',title:'Movie',duration:60,progressSeconds:0,sources:[{id:'source',container:'mkv',videoCodec:'h264'}]};
 const data={item,actions:['add_to_playlist','add_to_collection'].map(id=>({id,enabled:true})),metadata:{genres:[],ratings:[],credits:[]},related:{rows:[]}};
 const app=await componentModule(new URL('../src/screens/detail/Detail.tsx',import.meta.url),{
  react:h.react,'@tanstack/react-router':{useNavigate:()=>()=>{},useParams:()=>({itemId:'item'}),useSearch:()=>({library:'library'})},
  '@core/recommendations.ts':recommendationsModule,'@core/home.ts':homeModule,'@core/presentation/index.ts':{...presentation,formatDuration:()=> '1m'},'../../app/i18n':{currentI18n:()=>defaultI18n},'../../app/downloads':{useDownloads:()=>({state:{unavailable:true},ask:()=>{}})},'../../app/errors':{ErrorNotice:'ErrorNotice',ErrorState:'ErrorState'},'../../app/detail':{useDetail:()=>({snapshot:{phase:'ready',data,pending:[]},mutate:()=>{}}),useCreditPages:(_i,_g,initial)=>({credits:initial,more:()=>{},done:true}),useItemRecommendations:()=>({phase:'loading',data:null,retry:()=>{}})},
  '../../app/not-interested':{announceNotInterested:()=>{},markNotInterested:async()=>({undo:async()=>{}}),withoutHidden:(r:any)=>r},'../../app/not-interested-notice':{useNotInterested:()=>({hidden:new Set(),notice:null})},'@core/recommendation-feedback.ts':{isRecommendationRow:()=>false},'../../app/continue-watching':{announceRemoval:()=>{},removeFromContinueWatching:async()=>({undo:async()=>{}})},'../../app/metadata-editor':{useMetadataEditor:()=>undefined},'../../app/viewer-scope':{useViewerScope:()=>({})},'../../app/session':{useSession:()=>({owner:false})},
  '../../player/PlayerContext':{usePlayerActions:()=>({play:()=>played++,more:()=>{throw new Error('lost command');}})},
  '../../ui':{...ui,anchorOf:()=>anchor,cx:()=>''},'../shared/EntryActions':{PlaylistPicker:'PlaylistPicker',CollectionPicker:'CollectionPicker'},'../shared/Sections':{},'./Detail.module.css':{default:{}},
 });
 const render=()=>h.render(()=>app.DetailScreen());let tree=render();
 for(const n of nodes(tree))if(n.props?.ref)n.props.ref.current=n.props['aria-label']==='Playback info'?{scrollIntoView:()=>scrolled++,focus:()=>focused++}:{};
 const more=nodes(tree).find(n=>n.type==='Menu');
 for(const [command,picker] of [['addToPlaylist','PlaylistPicker'],['addToCollection','CollectionPicker']]){
  more.props.onSelect(command);tree=render();const menu=nodes(tree).find(n=>n.type==='AnchoredMenu');assert.equal(menu.props.open,true);assert.deepEqual(menu.props.anchor,anchor);
  const chosen=nodes(menu).find(n=>n.type===picker);assert.equal(chosen.props.itemId,'item');chosen.props.onDone('Done.');assert.equal(nodes(render()).find(n=>n.type==='AnchoredMenu').props.open,false);
 }
 // A command the menu does not offer does nothing, and never plays.
 more.props.onSelect('info');assert.equal(played,0);
 assert.ok(!nodes(render()).some(n=>n.type==='Dialog'));
 // Add to playlist lives in the entry menu now (Spec — Title Pages §1: Watchlist, Mark watched, ⋯).
 assert.ok(!nodes(render()).some(n=>n.type==='Button'&&n.props['aria-label']==='Add to playlist'));
});
for (const outcome of ['added', 'unchanged', 'failed', 'missing']) test(`collection picker pages, guards duplicates and checks ${outcome} outcome on retry`,async()=>{
 const h=hooks();const resource=(id:string,actions=['entries'])=>({id,name:id,kind:'collection',actions});
 let snap:any={resources:[resource('editable'),resource('readonly',[])],history:[null],nextCursor:'next',loading:false};
 let selected=0,mutated=0,retried=0,paged=0,release!:(v:boolean)=>void;const done:string[]=[];
 const service={getSnapshot:()=>snap,select:async()=>{selected++;},mutate:async()=>{mutated++;return new Promise<boolean>(r=>release=r);},retry:async()=>{retried++;snap={...snap,result:{deleted:false,...(outcome==='missing'?{}:{entries:{added:outcome==='added'?['item']:[],unchanged:outcome==='unchanged'?['item']:[],failed:outcome==='failed'?[{itemId:'item',code:'item_unavailable'}]:[]}})}};return true;},next:async()=>{paged++;},previous:async()=>{paged++;},refresh:async()=>{}};
 const app=await componentModule(new URL('../src/screens/shared/EntryActions.tsx',import.meta.url),{
  react:h.react,'@tanstack/react-router':{},'../../app/together':{},'../../app/downloads':{},'../../app/i18n':{useI18n:()=>defaultI18n,currentI18n:()=>defaultI18n},'../../app/delete-media':{},'@core/saved.ts':{},'@core/personal-saved.ts':{},'@core/queue-controller.ts':{},
  '../../app/viewer-scope':{useViewerScope:()=>({})},'../../app/session':{useSession:()=>({api:{},session:{}})},'../../app/not-interested':{announceNotInterested:()=>{},markNotInterested:async()=>({undo:async()=>{}}),withoutHidden:(r:any)=>r},'../../app/not-interested-notice':{useNotInterested:()=>({hidden:new Set(),notice:null})},'@core/recommendation-feedback.ts':{isRecommendationRow:()=>false},'../../app/continue-watching':{announceRemoval:()=>{},removeFromContinueWatching:async()=>({undo:async()=>{}})},'../../app/metadata-editor':{},'../../app/detail':{},'@core/presentation/index.ts':{viewerScope:()=>({})},'../../app/content':{useService:()=>({service,snapshot:snap})},'../../app/errors':{errorText:(e:any)=>e?.message==='Network failure'?'The collection couldn’t be changed.':'unexpected'},'../../app/open':{},'../../player/engine':{},'../../ui':ui,'./Sections':{useAppendedPages:(scope:string,cursor:string|null,page:readonly unknown[])=>page},'./playlist-add':{addItemsToPlaylist:async()=>({ok:0,failed:[]}),addItemsToPlaylistViaJob:async()=>({ok:0,failed:[]})},'./bulk-job':{runBulkJobs:async()=>({ok:0,failed:[],jobs:0})},'./container-watched':{containerKindFor:()=>undefined,useContainerWatched:()=>({state:null,loading:false,saving:false,error:null,refresh:()=>{},setWatched:async()=>false})},
 });
 const render=()=>h.render(()=>app.CollectionPicker({itemId:'item',onBack:()=>{},onDone:(v:string)=>done.push(v)}));
 let all=nodes(render());assert.ok(!all.some(n=>n.props?.label==='readonly'));
 all.find(n=>n.props?.label==='Next').props.onClick();all.find(n=>n.props?.label==='Previous').props.onClick();assert.equal(paged,2);
 const button=all.find(n=>n.props?.label==='editable');button.props.onClick();button.props.onClick();await tick();assert.equal(selected,1);assert.equal(mutated,1);
 snap={...snap,retryPending:true,mutationError:{message:'Network failure'}};release(false);await tick();all=nodes(render());assert.deepEqual(done,[]);
 const notice=all.find(n=>n.type==='Notice');assert.equal(notice.props.children[0],'The collection couldn’t be changed.');notice.props.action.onClick();await tick();assert.equal(retried,1);assert.deepEqual(done,outcome==='added'||outcome==='unchanged'?['Added to editable.']:[]);assert.equal(mutated,1);if(outcome==='failed'||outcome==='missing')assert.ok(nodes(render()).some(n=>n.type==='Notice'));
});
