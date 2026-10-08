import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule,hooks} from './helpers/component-harness.mjs';
const nodes=(tree:any):any[]=>!tree||typeof tree!=='object'?[]:Array.isArray(tree)?tree.flatMap(nodes):[tree,...nodes(tree.props?.children),...nodes(tree.props?.action),...nodes(tree.props?.secondary)];
const ui=Object.fromEntries(['Button','Inset','Menu','Page','Section','Select','StateView','TitleHero','WindowedGrid'].map(x=>[x,x]));
async function screen(workspace:any,search:any={library:'library'},recommendations:any={phase:'idle',data:null,retry:()=>{}}){
 const h=hooks();const navigations:any[]=[];
 const app=await componentModule(new URL('../src/screens/detail/Show.tsx',import.meta.url),{react:{...h.react,default:h.react},'@tanstack/react-router':{useNavigate:()=>(to:any)=>navigations.push(to),useParams:()=>({showId:'show'}),useSearch:()=>search},'@core/presentation/index.ts':{viewerScope:()=>({viewerId:'v',serverId:'s'})},'../../app/i18n':{currentI18n:()=>defaultI18n},'../../app/detail':{useShowWorkspace:()=>workspace,useShowRecommendations:()=>recommendations},'@core/home.ts':{homeRowHeading:(r:any)=>({key:r.id,fallback:r.title})},'../../app/not-interested':{withoutHidden:(r:any)=>r},'../../app/not-interested-notice':{useNotInterested:()=>({hidden:new Set(),notice:null})},'../shared/Sections':{SectionView:'SectionView',LoadingGrid:'LoadingGrid'},'../../app/episodes':{useEpisodes:()=>undefined},'../../app/errors':{ErrorNotice:'ErrorNotice',ErrorState:'ErrorState',changedError:{code:'refresh_required'}},'../../app/metadata-editor':{repairTargetFor:()=>null,useMetadataEditor:()=>null},'../../app/viewer-scope':{useViewerScope:()=>({viewerId:'v',serverId:'s'})},'../../app/session':{useSession:()=>({api:{},session:{viewer:{authority:'hosted'}},owner:false})},'../../player/PlayerContext':{usePlayerActions:()=>({playSequence:()=>{}})}, '../../ui':ui,'./EpisodeCard':{EpisodeCard:'EpisodeCard',EpisodePlaceholder:'EpisodePlaceholder',episodeCode:(e:any)=>e.episodeNumber?`E${e.episodeNumber}`:''},'./EpisodePanel':{EpisodePanel:'EpisodePanel'},'../shared/container-watched':{useContainerWatched:()=>({state:null,loading:false,saving:false,error:null,refresh:()=>{},setWatched:async()=>false})},'./Show.module.css':{default:{}}});
 return {tree:()=>h.render(()=>app.ShowScreen()),navigations};
}
const data={show:{id:'show',libraryId:'library',title:'Show',libraryKind:'tv'},seasons:Array.from({length:3},(_,i)=>({id:'s'+i,showId:'show',number:i,title:''})),seasonTotalCount:3,groups:[],selected:{seasonId:'s1'},episodes:{sections:[{entries:[{id:'e1',title:'Pilot',kind:'episode',episodeNumber:1,seasonNumber:1,playback:{itemId:'e1'}}],totalCount:1,nextCursor:''}]}};
test('retained show page failure exposes Retry and keeps the hero',async()=>{
 let retries=0;
 const {tree}=await screen({snapshot:{phase:'error',error:{message:'Connection lost'},data,pagination:{cursor:null,canPrevious:false},seasonPagination:{canPrevious:false,nextCursor:null}},retry:()=>retries++,selectSeason:()=>{},selectGroup:()=>{},nextSeasons:()=>{},previousSeasons:()=>{}});
 const all=nodes(tree());
 const notice=all.find(n=>n.type==='ErrorNotice');assert.equal(notice.props.context,'show');notice.props.retry();assert.equal(retries,1);
 const hero=all.find(n=>n.type==='TitleHero');assert.equal(hero.props.title,'Show');assert.equal(hero.props.primary.label,'Play E1');
});
test('season switcher puts Specials last and writes the season number into the URL',async()=>{
 const selected:string[]=[];
 const {tree,navigations}=await screen({snapshot:{phase:'ready',error:null,data,pagination:{cursor:null,canPrevious:false},seasonPagination:{canPrevious:false,nextCursor:null}},retry:()=>{},selectSeason:(id:string)=>selected.push(id),selectGroup:()=>{},nextSeasons:()=>{},previousSeasons:()=>{}});
 const switcher=nodes(tree()).find(n=>typeof n.type==='function'&&n.type.name==='SeasonSwitcher');
 assert.deepEqual(switcher.props.seasons.map((x:any)=>x.number),[1,2,0]);
 switcher.props.onSeason(switcher.props.seasons[1]);
 assert.deepEqual(selected,['s2']);assert.equal(navigations.at(-1).search.season,'2');
});

test('failed show recommendations have a row-level retry while the show remains visible',async()=>{
 let attempts=0;
 const failure={code:'unavailable'};
 const {tree}=await screen({snapshot:{phase:'ready',error:null,data,pagination:{cursor:null,canPrevious:false},seasonPagination:{canPrevious:false,nextCursor:null}},retry:()=>{},selectSeason:()=>{},selectGroup:()=>{},nextSeasons:()=>{},previousSeasons:()=>{}},{library:'library'},{phase:'error',data:null,error:failure,retry:()=>attempts++});
 const all=nodes(tree());
 const error=all.find(n=>n.type==='ErrorNotice');
 assert.equal(error?.props.error,failure);
 error.props.retry();
 assert.equal(attempts,1);
 assert.equal(all.find(n=>n.type==='TitleHero')?.props.title,'Show');
});
