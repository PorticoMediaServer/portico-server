import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as presentation from '@core/presentation/index.ts';
import * as recommendationsModule from '@core/recommendations.ts';
import * as homeModule from '@core/home.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';
const ui = Object.fromEntries(['AnchoredMenu','Artwork','Backdrop','Button','Card','Inset','Menu','Notice','Page','Section','Shelf','Skeleton','StateView','Surface','Text','Input','ListRow','Loading','MenuHeading','MenuRow','MenuSeparator','TitleHero','Dialog','KeyValue','Icon'].map(x=>[x,x]));
function nodes(tree:any):any[]{if(!tree||typeof tree!=='object')return[];if(Array.isArray(tree))return tree.flatMap(nodes);return[tree,...nodes(tree.props?.children),...nodes(tree.props?.action),...nodes(tree.props?.actions),...nodes(tree.props?.secondary)];}
// A row is shown with three titles or more (Spec — Title Pages §2).
const three=(id:string)=>[1,2,3].map(n=>({id:`${id}-${n}`,kind:'movie',title:`${id} ${n}`}));
async function detailApp(recommendations:any){
 const h=hooks();
 const item={id:'item',libraryId:'library',kind:'movie',title:'Movie',duration:60,progressSeconds:0};
 const data={item,actions:[],metadata:{genres:[],ratings:[],credits:[]},related:{rows:[{id:'genre:tmdb:1',heading:'Related',entries:three('other')}]}};
 const app=await componentModule(new URL('../src/screens/detail/Detail.tsx',import.meta.url),{
  react:h.react,'@tanstack/react-router':{useNavigate:()=>()=>{},useParams:()=>({itemId:'item'}),useSearch:()=>({library:'library'})},
  '@core/recommendations.ts':recommendationsModule,'@core/home.ts':homeModule,'@core/presentation/index.ts':{...presentation,formatDuration:()=>'1m'},'../../app/i18n':{currentI18n:()=>defaultI18n},'../../app/downloads':{useDownloads:()=>({state:{unavailable:true},ask:()=>{}})},'../../app/errors':{ErrorNotice:'ErrorNotice',ErrorState:'ErrorState'},'../../app/detail':{useDetail:()=>({snapshot:{phase:'ready',data,pending:[]},mutate:()=>{}}),useCreditPages:(_i,_g,initial)=>({credits:initial,more:()=>{},done:true}),useItemRecommendations:()=>recommendations?.phase ? recommendations : {phase:recommendations ? 'ready' : 'loading',data:recommendations,retry:()=>{}}},
  '../../app/not-interested':{announceNotInterested:()=>{},markNotInterested:async()=>({undo:async()=>{}}),withoutHidden:(r:any)=>r},'../../app/not-interested-notice':{useNotInterested:()=>({hidden:new Set(),notice:null})},'@core/recommendation-feedback.ts':{isRecommendationRow:()=>false},'../../app/continue-watching':{announceRemoval:()=>{},removeFromContinueWatching:async()=>({undo:async()=>{}})},'../../app/metadata-editor':{useMetadataEditor:()=>undefined},'../../app/viewer-scope':{useViewerScope:()=>({})},'../../app/session':{useSession:()=>({owner:false})},
  '../../player/PlayerContext':{usePlayerActions:()=>({play:()=>{},more:()=>{}})},
  '../../ui':{...ui,anchorOf:()=>undefined,cx:()=>''},'../shared/EntryActions':{PlaylistPicker:'PlaylistPicker',CollectionPicker:'CollectionPicker'},'../shared/Sections':{SectionView:'SectionView'},'./Detail.module.css':{default:{}},
 });
 return h.render(()=>app.DetailScreen());
}
// P4: movies render every row the recommendations endpoint returned, in its order, each under its
// titleText; detail.related only stands in until the endpoint answers.
test('the endpoint rows replace detail.related on movies, all of them in order; related only before they load',async()=>{
 const text=(code:string,params:Record<string,string>|undefined,fallback:string)=>({code,...(params?{params}:{}),fallback});
 const more={id:'more_like:local:item',title:'More like Movie',titleText:text('home.row.moreLike',{title:'Movie'},'More like Movie'),relation:'more_like',entries:three('m3')};
 const starring={id:'starring:tmdb:7',title:'Starring Ada',titleText:text('home.row.starring',{name:'Ada'},'Starring Ada'),relation:'starring',entries:three('m4')};
 const viewers={id:'viewers_also_watched:local:item',title:'Viewers also watched',titleText:text('home.row.viewersAlsoWatched',undefined,'Viewers also watched'),relation:'viewers_also_watched',entries:three('m5')};
 const genre={id:'genre:tmdb:16',title:'More Animation',relation:'genre',entries:three('m2')};
 const headed=nodes(await detailApp({rows:[more,starring,genre,viewers]})).filter(n=>n.type==='SectionView');
 assert.deepEqual(headed.map(n=>n.props.section.id),[more.id,starring.id,genre.id,viewers.id]);
 assert.deepEqual(headed[0].props.section.heading,{key:'home.row.moreLike',fallback:'More like Movie',params:{title:'Movie'}});
 assert.equal(headed[0].props.origin,'recommendation','title rows offer Not interested');
 const withRow=nodes(await detailApp({rows:[genre]})).filter(n=>n.type==='SectionView');
 assert.equal(withRow.length,1);
 assert.equal(withRow[0].props.section.id,'genre:tmdb:16');
 assert.equal(withRow[0].props.section.heading.fallback,'More Animation');
 assert.deepEqual(withRow[0].props.section.entries,genre.entries);
 const thin=nodes(await detailApp({rows:[{...genre,entries:genre.entries.slice(0,2)}]})).filter(n=>n.type==='SectionView');
 assert.equal(thin.length,0,'a row with fewer than three titles is not shown');
 const none=nodes(await detailApp({rows:[]})).filter(n=>n.type==='SectionView');
 assert.equal(none.length,0,'an answered, empty endpoint shows no rows (not detail.related)');
 const fallback=nodes(await detailApp(null)).filter(n=>n.type==='SectionView');
 assert.equal(fallback.length,1);
 assert.equal(fallback[0].props.section.heading.fallback,'Related');
 const failure={code:'unavailable'};
 const failed=nodes(await detailApp({phase:'error',data:null,error:failure,retry:()=>{}}));
 assert.equal(failed.filter(n=>n.type==='SectionView').length,1,'related remains visible on endpoint failure');
 const error=failed.find(n=>n.type==='ErrorNotice');
 assert.equal(error?.props.error,failure);
 assert.equal(typeof error?.props.retry,'function');
});
