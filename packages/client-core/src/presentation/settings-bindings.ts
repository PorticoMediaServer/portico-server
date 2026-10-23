import type {SettingsConfiguration, ConfigurationValue} from '../settings-configuration.ts';
import {canonicalQualityLadder} from '../../../contracts/src/types.ts';
import type {PreferenceSnapshot} from '../preferences.ts';

export type SettingsBindingForm='runtime'|'connectivity'|'maintenance'|'alerts'|'preferences'|'device'|'remote'|'managedRoute'|'logs';
export type SettingsBinding=Readonly<{form:SettingsBindingForm;key:string;scale?:number;codec?:'number-choice'|'unlimited'|'domain'|'interface'|'remote-policy'|'away-quality'|'update-checks'|'window-mode'|'days'|'log-retention'|'still-watching'|'missing-rule';fields?:Readonly<Record<string,string>>;window?:'heavy'|'backup'}>;
export type SettingsBindingSources=Readonly<{configuration?:SettingsConfiguration;preferences?:PreferenceSnapshot;forms?:Readonly<Partial<Record<SettingsBindingForm,Readonly<Record<string,unknown>>>>>;device?:Readonly<Record<string,unknown>>}>;
export type SettingsBindingValue=Readonly<{current:unknown;effective:unknown;default:unknown;source:string;configOnly:boolean;applicationMode?:string}>;
export type SettingsBindingPatch=Readonly<{form:SettingsBindingForm;values:Readonly<Record<string,unknown>>}>;
const bindings:Readonly<Record<string,SettingsBinding>>={
 'playback.skipping':{form:'preferences',key:'playback.skipping',fields:{intro:'playback.introSkip',credits:'playback.creditsSkip',recap:'playback.recapSkip'}},
 'playback.stillWatching':{form:'preferences',key:'playback.passoutProtection',fields:{enabled:'playback.passoutProtection',episodes:'playback.passoutAfterEpisodes'},codec:'still-watching'},
 'storage.missingRule':{form:'runtime',key:'library.missingTitles',codec:'missing-rule'},
 'storage.missingDaysExact':{form:'runtime',key:'library.missingTitlesDays'},
 'segments.mode':{form:'runtime',key:'library.segmentDetection'},
 'people.maxStreamsEach':{form:'runtime',key:'perAccountCap',codec:'unlimited'},
 'people.communityActivity':{form:'runtime',key:'home.communityActivityEnabled'},
 'integrations.requestsPerMinute':{form:'runtime',key:'apiRequestsPerMinute'},
 'streaming.whenCantPlay':{form:'runtime',key:'planningPolicy'},
 'general.name':{form:'runtime',key:'name'},
 'general.updateChecks':{form:'runtime',key:'updates.feedUrl',codec:'update-checks'},
 'general.updateFeed':{form:'runtime',key:'updates.feedUrl'},
 'updates.channel':{form:'runtime',key:'updates.releaseChannel'},
 'streaming.uploadSpeed':{form:'connectivity',key:'uploadCapacityKbps',scale:1000},
 'streaming.awayQuality':{form:'connectivity',key:'remoteBitrateLimitKbps',codec:'away-quality'},
 'streaming.awayExact':{form:'connectivity',key:'remoteBitrateLimitKbps',scale:1000},
 'streaming.hardware':{form:'runtime',key:'hardwareBackend'},
 'streaming.hardwareDevice':{form:'runtime',key:'hardwareDevice'},
 'streaming.toneMapping':{form:'runtime',key:'hdrToneMapping'},
 'streaming.toneMappingMethod':{form:'runtime',key:'hdrToneMappingAlgorithm'},
 'streaming.encoderSpeed':{form:'runtime',key:'x264Preset'},
 'streaming.repackage':{form:'runtime',key:'directStreamRemux'},
 'streaming.ffmpegPath':{form:'runtime',key:'ffmpegPath'},
 'streaming.convertAhead':{form:'runtime',key:'throttleBufferMs',scale:1000,codec:'number-choice'},
 'streaming.rewindKeep':{form:'runtime',key:'playedRetentionMs',scale:1000,codec:'number-choice'},
 'streaming.sessionSpace':{form:'runtime',key:'sessionStorageMiB',codec:'number-choice'},
 'streaming.maxConversions':{form:'runtime',key:'maxConcurrentSessions',codec:'unlimited'},
 'streaming.maxHardwareConversions':{form:'runtime',key:'maxHardwareSessions',codec:'unlimited'},
 'streaming.maxSoftwareConversions':{form:'runtime',key:'maxSoftwareSessions',codec:'unlimited'},
 'streaming.maxBackgroundConversions':{form:'runtime',key:'maxBackgroundSessions',codec:'unlimited'},
 'streaming.maxStreams':{form:'runtime',key:'serverCap',codec:'unlimited'},
 'streaming.pausedTimeout':{form:'connectivity',key:'pausedSessionTimeoutMinutes',codec:'number-choice'},
 'remote.ownCertificate':{form:'connectivity',key:'ownAddressEnabled'},
 'remote.externalAddress':{form:'connectivity',key:'customCertificateDomain',codec:'domain'},
 'remote.certFile':{form:'connectivity',key:'customCertificatePath'},
 'remote.certKey':{form:'connectivity',key:'customCertificateKeyPath'},
 'remote.homeNets':{form:'connectivity',key:'lanNetworks'},
 'remote.discovery':{form:'connectivity',key:'lanDiscoveryEnabled'},
 'remote.dlna':{form:'connectivity',key:'dlnaEnabled'},
 'remote.interface':{form:'connectivity',key:'advertisedInterface',codec:'interface'},
 'remote.connectionLimit':{form:'connectivity',key:'connectionLimit',codec:'unlimited'},
 'remote.extraAddresses':{form:'connectivity',key:'accessUrls'},
 'remote.trustedProxies':{form:'connectivity',key:'trustedProxies'},
 'remote.who':{form:'connectivity',key:'remoteSignInPolicy',codec:'remote-policy'},
 'general.heavyWork':{form:'maintenance',key:'enabled',window:'heavy',codec:'window-mode'},
 'general.heavyStart':{form:'maintenance',key:'startMinute',window:'heavy',codec:'number-choice'},
 'general.heavyLength':{form:'maintenance',key:'durationMinutes',window:'heavy',scale:60,codec:'number-choice'},
 'general.heavyDays':{form:'maintenance',key:'days',window:'heavy',codec:'days'},
 'storage.backupSchedule':{form:'maintenance',key:'enabled',window:'backup',codec:'window-mode'},
 'storage.backupTime':{form:'maintenance',key:'startMinute',window:'backup',codec:'number-choice'},
 'storage.backupWindowLength':{form:'maintenance',key:'durationMinutes',window:'backup',scale:60,codec:'number-choice'},
 'storage.backupDays':{form:'maintenance',key:'days',window:'backup',codec:'days'},
 'history.keepHistory':{form:'runtime',key:'playHistoryDays',codec:'number-choice'},
 'general.backgroundSpeed':{form:'maintenance',key:'backgroundSpeed'},
 'general.alertStorageWarn':{form:'alerts',key:'storageWarningPercent',codec:'number-choice'},
 'general.alertStorageCritical':{form:'alerts',key:'storageCriticalPercent',codec:'number-choice'},
 'general.alertCertificateDays':{form:'alerts',key:'certificateWarningDays',codec:'number-choice'},
 'storage.analysisSpace':{form:'runtime',key:'analysisStorageGiB',codec:'number-choice'},
 'storage.analysisStage':{form:'runtime',key:'analysisStageStorageMiB',codec:'number-choice'},
 'storage.subtitleTracks':{form:'runtime',key:'subtitleTrackLimit',codec:'number-choice'},
 'storage.subtitleFile':{form:'runtime',key:'subtitleFileLimitMiB',codec:'number-choice'},
 'storage.keepNotifications':{form:'runtime',key:'notificationDays',codec:'number-choice'},
 'storage.backupKeep':{form:'maintenance',key:'backupKeepCount',codec:'number-choice'},
 'storage.backupKeepExact':{form:'maintenance',key:'backupKeepCount'},
 'storage.keepConversions':{form:'maintenance',key:'retention.conversions',codec:'number-choice'},
 'storage.keepPrepared':{form:'maintenance',key:'retention.prepared',codec:'number-choice'},
 'storage.keepDownloads':{form:'maintenance',key:'retention.downloads',codec:'number-choice'},
 'storage.keepSubtitles':{form:'maintenance',key:'retention.subtitles',codec:'number-choice'},
 'storage.keepTrickplay':{form:'maintenance',key:'retention.trickplay',codec:'number-choice'},
 'troubleshooting.logLevel':{form:'logs',key:'logLevel'},
 'troubleshooting.keepLogs':{form:'runtime',key:'diagnosticDays',codec:'number-choice'},
 'troubleshooting.keepLogsByKind':{form:'runtime',key:'logRetention',codec:'log-retention'},
};
const preferenceKeys:Readonly<Record<string,string>>={
 'audioSubtitles.preferredAudio':'playback.preferredAudioLanguages','audioSubtitles.preferredSubtitles':'playback.preferredSubtitleLanguages','audioSubtitles.subtitleMode':'playback.subtitleMode','audioSubtitles.subtitleSize':'playback.subtitleSize','audioSubtitles.subtitleBackground':'playback.subtitleBackground',
 'quality.home':'quality.home','quality.remote':'quality.remote','quality.mobileData':'quality.mobile',
 'quality.directPlay':'delivery.directPlay','quality.directStream':'delivery.directStream','quality.transcode':'delivery.transcode',
 'privacy.showActivity':'privacy.showActivityToMembers','privacy.includeWatchTogether':'privacy.includeInWatchTogether','privacy.pauseHistory':'privacy.pauseWatchHistory','privacy.rememberSearches':'search.rememberHistory',
 'music.audioNormalization':'music.audioNormalization','music.crossfadeMs':'music.crossfadeMs','music.gapless':'music.gapless','playback.showSyncedLyrics':'playback.showSyncedLyrics','playback.audiobookSpeed':'audiobooks.defaultSpeed','music.idlePause':'playback.sleepTimerMinutes',
 'music.qualityAway':'music.qualityAway','music.qualityMobile':'music.qualityMobile',
 'language.locale':'region.locale','language.hourCycle':'region.hourCycle',
 'playback.autoplayNext':'playback.autoplayNext','playback.upNextCountdownMs':'playback.upNextCountdownMs','playback.startNextDuringCredits':'playback.startNextDuringCredits','playback.skipBackMs':'playback.skipBackMs','playback.skipForwardMs':'playback.skipForwardMs','playback.playedThresholdPercent':'playback.playedThresholdPercent','playback.startedThresholdPercent':'playback.startedThresholdPercent',
 'playback.introSkip':'playback.introSkip','playback.creditsSkip':'playback.creditsSkip','playback.recapSkip':'playback.recapSkip',
 'appearance.backdrops':'appearance.showBackdrops','appearance.cardSizeExact':'appearance.cardSizePercent','appearance.notificationDot':'notifications.badges',
};
export function settingsBinding(rowId:string):SettingsBinding|undefined{return bindings[rowId]??(preferenceKeys[rowId]?{form:'preferences',key:preferenceKeys[rowId]!}:undefined);}
function at(source:Readonly<Record<string,unknown>>|undefined,key:string):unknown{if(source&&Object.hasOwn(source,key))return source[key];return key.split('.').reduce<unknown>((v,key)=>v&&typeof v==='object'?(v as Record<string,unknown>)[key]:undefined,source);}
const days=['monday','tuesday','wednesday','thursday','friday','saturday','sunday'] as const;
const categories=['server','playback','scan','network','client'] as const;
const cadences:Readonly<Record<string,readonly string[]>>={nightly:days,weekdays:days.slice(0,5),weekends:days.slice(5),weekly:['sunday']};
function defaultFeed(sources:SettingsBindingSources):unknown{return sources.configuration?.fields.find(f=>f.id==='general.updateFeed')?.default;}
function windows(sources:SettingsBindingSources):readonly Readonly<Record<string,unknown>>[]{const v=sources.forms?.maintenance?.windows;return Array.isArray(v)?v:[];}
function windowMode(binding:SettingsBinding,w:Readonly<Record<string,unknown>>):unknown{
 if(!w.enabled)return binding.window==='heavy'?'any':'never';
 if(binding.window==='heavy')return w.startMinute===60&&w.durationMinutes===300&&Array.isArray(w.days)&&w.days.length===7&&days.every(d=>(w.days as unknown[]).includes(d))&&!w.timezone?'overnight':'custom';
 return Object.entries(cadences).find(([,wanted])=>Array.isArray(w.days)&&w.days.length===wanted.length&&wanted.every(d=>(w.days as unknown[]).includes(d)))?.[0]??'custom';
}
function presented(binding:SettingsBinding,v:unknown,sources:SettingsBindingSources):unknown{
 if(v===undefined)return undefined;
 if(binding.codec==='missing-rule'){const n=sources.configuration?.fields.find(f=>f.id==='storage.missingDaysExact')?.current??sources.forms?.runtime?.['library.missingTitlesDays'];return v==='never'?'never':v==='immediately'?'now':v==='after_days'?(n===7?'7d':n===30?'30d':n===90?'90d':'custom'):undefined;}
 if(binding.key==='planningPolicy')return v==='minimize_server_work'?'least':v==='maximum_fidelity'?'most':undefined;
 if(binding.codec==='update-checks'){const feed=defaultFeed(sources);return v===''?'never':feed===undefined?undefined:v===feed?'auto':'custom';}
 if(binding.codec==='away-quality')return v===0?'original':canonicalQualityLadder.find(q=>q.height>0&&q.videoKbps===v)?.id.replace('2160p','4k')??'custom';
 if(binding.codec==='domain')return v?`https://${String(v)}`:'';
 if(binding.codec==='interface')return v||'all';
 if(binding.codec==='remote-policy')return v==='allow'?'anyone':v==='owner-only'?'owner':v==='off'?'none':undefined;
 if(binding.codec==='unlimited')return v===0||v===null?'unlimited':String(v);
 if(binding.codec==='days'&&Array.isArray(v)){const chosen=v;return Object.fromEntries(days.map(d=>[d,chosen.includes(d)?'runs':'skips']));}
 if(binding.codec==='log-retention'&&Array.isArray(v)){const entries=v as {category:string;days:number}[];return Object.fromEntries(categories.map(k=>[k,String(entries.find(r=>r.category===k)?.days??'default')]));}
 if(binding.scale){if(v===0&&binding.key==='uploadCapacityKbps')return null;v=typeof v==='number'?v/binding.scale:v;}
 return binding.codec==='number-choice'?String(v):v;
}
export function readSettingsBinding(binding:SettingsBinding,sources:SettingsBindingSources,rowId?:string):SettingsBindingValue|undefined{
 if(binding.fields){const values=Object.entries(binding.fields).map(([name,key])=>[name,readSettingsBinding({form:binding.form,key},sources)] as const);if(values.some(([,value])=>!value))return undefined;const part=(kind:'current'|'effective'|'default')=>{const v=Object.fromEntries(values.map(([name,value])=>[name,value![kind]]));return binding.codec==='still-watching'?(v.enabled?v.episodes:0):v;};return {current:part('current'),effective:part('effective'),default:part('default'),source:[...new Set(values.map(([,v])=>v!.source))].join(','),configOnly:false};}
 if(binding.form==='preferences'){
  const snapshot=sources.preferences,field=snapshot?.registry.fields.find(f=>f.key===binding.key);if(!snapshot||!field)return undefined;
  const source=snapshot.effectiveSource[binding.key];if(source===undefined)return undefined;
  const stored=snapshot.documents.find(d=>d.scope===source)?.values[binding.key];
  return {current:presented(binding,stored??snapshot.effective[binding.key],sources),effective:presented(binding,snapshot.effective[binding.key],sources),default:presented(binding,field.default,sources),source,configOnly:false};
 }
 const alias=rowId==='general.updateChecks'?'general.updateFeed':rowId==='streaming.awayQuality'?'streaming.awayExact':rowId==='storage.backupKeep'?'storage.backupKeepExact':rowId;
 const fact:ConfigurationValue|undefined=alias?sources.configuration?.fields.find(f=>f.id===alias):undefined;
 if(fact){const display={...binding,scale:undefined,...(rowId==='streaming.awayQuality'?{scale:1/1000}:{})};const show=(v:unknown)=>rowId==='streaming.awayQuality'?presented(binding,typeof v==='number'?v*1000:v,sources):presented(display,v,sources);return {current:show(fact.current),effective:show(fact.effective),default:show(fact.default),source:fact.source,configOnly:fact.configOnly,applicationMode:fact.applicationMode};}
 const w=binding.window?windows(sources).find(w=>w.id===binding.window):undefined;
 const raw=binding.window?(binding.codec==='window-mode'?w&&windowMode(binding,w):w?.[binding.key]):at(binding.form==='device'?sources.device:sources.forms?.[binding.form],binding.key);if(raw===undefined)return undefined;
 return {current:presented(binding,raw,sources),effective:presented(binding,raw,sources),default:undefined,source:binding.form,configOnly:false};
}
/** A write always targets an existing producer; Advanced values are not rounded to a preset. */
export function settingsBindingPatch(binding:SettingsBinding,value:unknown,sources:SettingsBindingSources={}):SettingsBindingPatch{
 if(binding.fields){
 if(binding.codec==='still-watching'){const n=Number(value);if(!Number.isSafeInteger(n)||![0,2,3,4,5].includes(n))throw new Error('Invalid episode count.');return {form:binding.form,values:n===0?{'playback.passoutProtection':false}:{'playback.passoutProtection':true,'playback.passoutAfterEpisodes':n}};}
 if(!value||typeof value!=='object'||Array.isArray(value)||Object.keys(value).length!==Object.keys(binding.fields).length)throw new Error('Invalid setting matrix.');return {form:binding.form,values:Object.fromEntries(Object.entries(binding.fields).map(([name,key])=>{const v=(value as Record<string,unknown>)[name];if(!['ask','auto','off'].includes(String(v)))throw new Error('Invalid skip mode.');return [key,v];}))};
 }
 if(binding.codec==='missing-rule'){if(value==='never')return {form:binding.form,values:{'library.missingTitles':'never'}};if(value==='now')return {form:binding.form,values:{'library.missingTitles':'immediately'}};const n=value==='7d'?7:value==='30d'?30:value==='90d'?90:undefined;if(n===undefined)throw new Error('Custom is derived.');return {form:binding.form,values:{'library.missingTitles':'after_days','library.missingTitlesDays':n}};}
 let raw=value;
 if(binding.key==='planningPolicy')raw=value==='least'?'minimize_server_work':value==='most'?'maximum_fidelity':undefined;
 if(value==='custom'&&binding.codec!=='window-mode')throw new Error('Custom is derived.');
 if(binding.codec==='domain'){
  if(value==='')raw='';else{if(typeof value!=='string')throw new Error('An HTTPS address is required.');const url=new URL(value.includes('://')?value:`https://${value}`);if(url.protocol!=='https:'||url.username||url.password||url.pathname!=='/'||url.search||url.hash||url.port)throw new Error('An HTTPS domain is required.');raw=url.hostname;}
 }else if(binding.codec==='update-checks'){raw=value==='never'?'':value==='auto'?defaultFeed(sources):undefined;}
 else if(binding.codec==='away-quality'){raw=value==='original'?0:canonicalQualityLadder.find(q=>q.height>0&&q.id===(value==='4k'?'2160p':value))?.videoKbps;}
 else if(binding.codec==='interface')raw=value==='all'?'':value;
 else if(binding.codec==='remote-policy')raw=value==='anyone'?'allow':value==='owner'?'owner-only':value==='none'?'off':undefined;
 else if(binding.codec==='unlimited')raw=value==='unlimited'||value==='auto'?(binding.key==='serverCap'?null:0):Number(value);
 else if(binding.codec==='number-choice')raw=Number(value);
 else if(binding.codec==='days'){
  if(!value||typeof value!=='object'||Object.keys(value).length!==7)throw new Error('Seven days are required.');
  const m=value as Record<string,unknown>;if(days.some(d=>m[d]!=='runs'&&m[d]!=='skips'))throw new Error('Invalid day.');raw=days.filter(d=>m[d]==='runs');
 }else if(binding.codec==='log-retention'){
  if(!value||typeof value!=='object'||Object.keys(value).some(k=>!categories.includes(k as typeof categories[number])))throw new Error('Invalid log category.');
  raw=Object.entries(value).flatMap(([category,days])=>{if(days==='default')return [];const n=Number(days);if(!Number.isSafeInteger(n)||n<1||n>90)throw new Error('Invalid log retention.');return [{category,days:n}];});
 }
 if(binding.scale)raw=value===null&&binding.key==='uploadCapacityKbps'?0:typeof raw==='number'?Math.round(raw*binding.scale):raw;
 if(typeof raw==='number'&&!Number.isFinite(raw)||raw===undefined)throw new Error('Invalid setting value.');
 if(binding.window){
  const all=windows(sources),existing=all.find(w=>w.id===binding.window);if(!existing)throw new Error('Load maintenance settings before editing a window.');let change:Record<string,unknown>={[binding.key]:raw};
  if(binding.codec==='window-mode'){
   if(binding.window==='heavy')change=value==='any'?{enabled:false}:value==='overnight'?{enabled:true,cadence:'nightly',startMinute:60,durationMinutes:300,days:[...days],timezone:''}:value==='custom'?{enabled:true}:{};
   else change=value==='never'?{enabled:false}:value==='custom'?{enabled:true}:typeof value==='string'&&cadences[value]?{enabled:true,cadence:value,days:[...cadences[value]!]}:{};
   if(!Object.keys(change).length)throw new Error('Invalid maintenance window.');
  }else if(binding.codec==='days')change={days:raw,cadence:'custom'};
  return {form:binding.form,values:{windows:all.map(w=>w.id===binding.window?{...w,...change}:w)}};
 }
 return {form:binding.form,values:{[binding.key]:raw}};
}
/** Apply dotted domain fields as well as literal dotted registry keys. */
export function applySettingsBindingPatch<T extends Readonly<Record<string,unknown>>>(draft:T,patch:SettingsBindingPatch):T{
 const next=structuredClone(draft) as Record<string,unknown>;
 for(const [key,value] of Object.entries(patch.values)){
  if(Object.hasOwn(next,key)||!key.includes('.')){next[key]=structuredClone(value);continue;}
  const parts=key.split('.');let at=next;for(const part of parts.slice(0,-1)){const child=at[part];at[part]=child&&typeof child==='object'&&!Array.isArray(child)?{...child}:{};at=at[part] as Record<string,unknown>;}at[parts.at(-1)!]=structuredClone(value);
 }
 return next as T;
}

export type SettingsPresetOption=Readonly<{value:unknown;sets?:Readonly<Record<string,unknown>>}>;
function equal(a:unknown,b:unknown):boolean{
 if(Object.is(a,b))return true;
 if(Array.isArray(a)&&Array.isArray(b))return a.length===b.length&&a.every((v,i)=>equal(v,b[i]));
 if(a&&b&&typeof a==='object'&&typeof b==='object'){const aa=a as Record<string,unknown>,bb=b as Record<string,unknown>;return Object.keys(aa).length===Object.keys(bb).length&&Object.keys(aa).every(k=>Object.hasOwn(bb,k)&&equal(aa[k],bb[k]));}
 return false;
}
/** Only full exact sets match; no nearest-choice policy loses Advanced values. */
function matches(actual:unknown,expected:unknown):boolean{if(expected&&typeof expected==='object'&&!Array.isArray(expected)){if(!actual||typeof actual!=='object'||Array.isArray(actual))return false;return Object.entries(expected).every(([key,value])=>matches((actual as Record<string,unknown>)[key],value));}return equal(actual,expected);}
export function exactPresetValue(options:readonly SettingsPresetOption[],read:(rowId:string)=>unknown):unknown{return options.find(option=>option.sets&&Object.entries(option.sets).every(([key,value])=>matches(read(key),value)))?.value??'custom';}
export function presetPatch(options:readonly SettingsPresetOption[],choice:unknown,read?:(rowId:string)=>unknown):Readonly<Record<string,unknown>>{const option=options.find(option=>Object.is(option.value,choice));if(!option?.sets)throw new Error('A stored preset is required.');return Object.fromEntries(Object.entries(option.sets).map(([key,value])=>[key,value&&typeof value==='object'&&!Array.isArray(value)?{...(read?.(key) as Record<string,unknown>??{}),...structuredClone(value)}:structuredClone(value)]));}
export const resetPreset=presetPatch;
export function changedFromDefault(current:unknown,defaultValue:unknown):boolean{return !equal(current,defaultValue);}

export {searchConfiguration} from '../settings-configuration.ts';
