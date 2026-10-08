import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {defaultI18n} from '@i18n';
import * as presentation from '@core/presentation/index.ts';
import * as recommendationsModule from '@core/recommendations.ts';
import * as homeModule from '@core/home.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';
const ui = Object.fromEntries(['AnchoredMenu','Artwork','Backdrop','Button','Card','Inset','Menu','Notice','Page','Section','Shelf','Skeleton','StateView','Surface','Text','Input','ListRow','Loading','MenuHeading','MenuRow','MenuSeparator','TitleHero','Dialog','KeyValue','Icon'].map(x=>[x,x]));
function nodes(tree:any):any[]{if(!tree||typeof tree!=='object')return[];if(Array.isArray(tree))return tree.flatMap(nodes);return[tree,...nodes(tree.props?.children),...nodes(tree.props?.action),...nodes(tree.props?.actions),...nodes(tree.props?.secondary)];}
const credit = (id:string,name:string,role:string,department:string,personId?:string)=>({id,name,role,department,provider:'tmdb',...(personId?{personId}:{})});
async function detailApp(credits:any[], service?:any){
 const h=hooks();
 const item={id:'item',libraryId:'library',kind:'movie',title:'Movie',duration:60,progressSeconds:0};
 const data={item,actions:[],metadata:{genres:[],ratings:[],credits},related:{rows:[]}};
 const ready={snapshot:{phase:'ready',data,pending:[]},mutate:()=>{},retry:()=>{}};
 const app=await componentModule(new URL('../src/screens/detail/Detail.tsx',import.meta.url),{
  react:h.react,'@tanstack/react-router':{useNavigate:()=>()=>{},useParams:()=>({itemId:'item'}),useSearch:()=>({library:'library'})},
  '@core/recommendations.ts':recommendationsModule,'@core/home.ts':homeModule,'@core/presentation/index.ts':{...presentation,formatDuration:()=>'1m'},'../../app/i18n':{currentI18n:()=>defaultI18n},'../../app/downloads':{useDownloads:()=>({state:{unavailable:true},ask:()=>{}})},'../../app/errors':{ErrorNotice:'ErrorNotice',ErrorState:'ErrorState'},  '../../app/detail':{useDetail:()=>service??ready,useCreditPages:(_i,_g,initial)=>({credits:initial,more:()=>{},done:true}),useItemRecommendations:()=>({phase:'loading',data:null,retry:()=>{}})},
  '../../app/not-interested':{announceNotInterested:()=>{},markNotInterested:async()=>({undo:async()=>{}}),withoutHidden:(r:any)=>r},'../../app/not-interested-notice':{useNotInterested:()=>({hidden:new Set(),notice:null})},'@core/recommendation-feedback.ts':{isRecommendationRow:()=>false},'../../app/continue-watching':{announceRemoval:()=>{},removeFromContinueWatching:async()=>({undo:async()=>{}})},'../../app/metadata-editor':{useMetadataEditor:()=>undefined},'../../app/viewer-scope':{useViewerScope:()=>({})},'../../app/session':{useSession:()=>({owner:false})},
  '../../player/PlayerContext':{usePlayerActions:()=>({play:()=>{},more:()=>{}})},
  '../../ui':{...ui,anchorOf:()=>undefined,cx:()=>''},'../shared/EntryActions':{PlaylistPicker:'PlaylistPicker',CollectionPicker:'CollectionPicker'},'../shared/Sections':{},'./Detail.module.css':{default:{}},
 });
 return {h,app,render:()=>h.render(()=>app.DetailScreen())};
}
// WEB-DETAIL-05: crew shelf order is Directing, Writing, Screenplay, Creator, Composer, Producer.
test('keyCrew keeps one row per person in credit order, at most 8, and drops other departments',async()=>{
 const {app}=await detailApp([]);
 const credits=[
  credit('c11','Property Pete','Property Master','Art','p11'),
  credit('c6','Producer Pam','Producer','Production','p6'),
  credit('c12','Editor Ed','Editor','Editing','p12'),
  credit('c1','Director Dan','Director','Directing','p1'),
  credit('c4','Creator Cat','Creator','Creator','p4'),
  credit('c2','Writer Wendy','Writer','Writing','p2'),
  credit('c5','Composer Cole','Original Music Composer','Music','p5'),
  credit('c3','Screenplay Sam','Screenplay','Writing','p3'),
  credit('c8','Second Sean','Director','Directing','p8'),
  credit('c7','Exec Erin','Executive Producer','Production','p7'),
  credit('c10','Extra Producer','Co-Producer','Production','p10'),
  credit('c1b','Director Dan','Director','Directing','p1'),
 ];
 const names=app.keyCrew(credits).map((c:any)=>c.name);
 assert.deepEqual(names,['Director Dan','Second Sean','Writer Wendy','Screenplay Sam','Creator Cat','Composer Cole','Producer Pam','Exec Erin']);
 assert.ok(!names.includes('Property Pete')&&!names.includes('Editor Ed'),'property masters and editors are crew, not key crew');
 assert.equal(app.crewRank('Art','Property Master'),-1);
 assert.equal(app.crewRank('Directing','Director'),0);
 assert.equal(app.crewRank('Writing','Screenplay'),1);
 assert.equal(app.crewRank('Writing','Writer'),1);
 assert.equal(app.crewRank('Creator','Creator'),3);
 assert.equal(app.crewRank('Music','Original Music Composer'),4);
 assert.equal(app.crewRank('Production','Executive Producer'),5);
});
test('crew shelf shows key crew with an All crew control that opens everyone in a dialog',async()=>{
 const credits=[
  credit('a1','Actor Ann','Hero','Acting','p0'),
  credit('c1','Director Dan','Director','Directing','p1'),
  credit('c2','Writer Wendy','Writer','Writing','p2'),
  credit('c3','Screenplay Sam','Screenplay','Writing','p3'),
  credit('c4','Creator Cat','Creator','Creator','p4'),
  credit('c5','Composer Cole','Original Music Composer','Music','p5'),
  credit('c6','Producer Pam','Producer','Production','p6'),
  credit('c7','Exec Erin','Executive Producer','Production','p7'),
  credit('c8','Second Sean','Director','Directing','p8'),
  credit('c9','Second Writer','Screenplay','Writing','p9'),
  credit('c10','Extra Producer','Co-Producer','Production','p10'),
  credit('c11','Property Pete','Property Master','Art','p11'),
 ];
 const {app,render}=await detailApp(credits);
 const crewShelf=()=>nodes(render()).find(n=>n.type==='Shelf'&&n.props?.title==='Crew');
 const cards=(shelf:any)=>nodes([shelf]).filter(n=>n.type==='Card');
 let shelf=crewShelf();
 assert.equal(cards(shelf).length,8);
 assert.ok(!cards(shelf).some((n:any)=>n.props?.title==='Property Pete'));
 assert.equal(shelf.props.action.label,'All crew');
 // All crew opens the whole cast and crew in a dialog; the shelf keeps its key crew.
 shelf.props.action.onClick();
 const tree=nodes(render());
 const dialog=tree.find((n:any)=>n.type==='Dialog');
 assert.ok(dialog,'everyone is listed in a dialog');
 assert.equal(dialog.props.title,defaultI18n.t('title.allPeople'));
 assert.equal(cards(crewShelf()).length,8);
});
test('a loading-to-error transition renders the error state: every hook runs before the error early-return',async()=>{
 const {render}=await detailApp([],{snapshot:{phase:'error',error:{code:'unreadable',message:'Nope',retryable:true},data:undefined,pending:[]},mutate:()=>{},retry:()=>{}});
 const tree=nodes(render());
 assert.ok(tree.some(n=>n.type==='ErrorState'),'the not-found/error view renders');
 assert.ok(tree.some((n:any)=>n.type==='Button'&&n.props?.label===defaultI18n.t('action.back')),'Back is offered');
 // Static guard: a hook below the early return would throw ("fewer hooks than
 // expected") the moment a loading page turns into this error view.
 const text=readFileSync(new URL('../src/screens/detail/Detail.tsx',import.meta.url),'utf8');
 const body=text.slice(text.indexOf('export function DetailScreen()'),text.indexOf('\nexport function qualityLabel'));
 const early=body.indexOf("if ((snapshot.phase === 'error' && !data) || missing)");
 assert.ok(early>0,'the error early-return exists');
 assert.ok(body.lastIndexOf('const keyCrewList',early)>0&&body.lastIndexOf('const [allPeople',early)>0,'crew hooks run before the early return');
 assert.doesNotMatch(body.slice(early),/[^a-zA-Z]use[A-Z]\w*\(/,'no hook call sits below the early return');
});
