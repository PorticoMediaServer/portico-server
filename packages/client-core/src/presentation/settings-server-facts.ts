import {routePath} from '../../../contracts/src/routes.ts';
import type {SettingsConfigurationAPI} from '../settings-configuration.ts';
import type {OwnerActivitySummary} from '../admin/activity-summary/types.ts';
import {unreadableServerResponse} from '../server-messages.ts';

/** The public system producer is sufficient for TV's safe read-only identity.
 * This path never fetches owner settings, credentials or troubleshooting. */
export type ReadOnlyServerSettingsFacts=Readonly<{serverId:string;name:string;version:string;signIn:'portico'|'local';readOnly:true}>;
export function parseReadOnlyServerSettingsFacts(raw:unknown):ReadOnlyServerSettingsFacts{
 if(!raw||typeof raw!=='object')throw new Error(unreadableServerResponse);const v=raw as Record<string,unknown>;
 for(const key of ['id','name','version'])if(typeof v[key]!=='string'||(v[key] as string).length>256||/[\u0000-\u001f]/.test(v[key] as string))throw new Error(unreadableServerResponse);
 if(!v.id||v.signIn!=='portico'&&v.signIn!=='local')throw new Error(unreadableServerResponse);
 return Object.freeze({serverId:v.id as string,name:v.name as string,version:v.version as string,signIn:v.signIn,readOnly:true});
}
export async function loadReadOnlyServerSettingsFacts(api:SettingsConfigurationAPI,serverId:string,signal?:AbortSignal):Promise<ReadOnlyServerSettingsFacts>{
 const facts=parseReadOnlyServerSettingsFacts(await api.request<unknown>(routePath('get_system'),'GET',undefined,signal));if(facts.serverId!==serverId)throw new Error(unreadableServerResponse);return facts;
}
export type SettingsMajorIssue=OwnerActivitySummary['majorIssues'][number];
/** A dot is a qualified producer fact. Diagnostics and page totals are not inputs. */
export function settingsSectionsWithMajorIssues(issues:readonly SettingsMajorIssue[]):ReadonlySet<string>{
 const sections=new Set<string>();
 for(const issue of issues){
  if(issue.severity==='playback_stop'&&(issue.code==='library_unreachable'||issue.code==='remote_unreachable'||issue.code==='remote_program_unavailable'))sections.add(issue.kind==='library'?'libraries':'remote-sources');
  if(issue.severity==='data_risk'&&(issue.code==='storage_nearly_full'||issue.code==='backup_failed'))sections.add('storage');
  if(issue.severity==='data_risk'&&issue.code==='secure_renewal_failed')sections.add('remote');
 }
 return sections;
}
