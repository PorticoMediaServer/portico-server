import assert from 'node:assert/strict';
import test from 'node:test';
import {loadReadOnlyServerSettingsFacts,parseReadOnlyServerSettingsFacts,settingsSectionsWithMajorIssues} from '../src/presentation/settings-server-facts.ts';

test('TV reads only public identity and never projects owner facts or actions',async()=>{
 const raw={id:'server',name:'Home',version:'1.0',signIn:'local',password:'private',paths:['/data'],actions:['restart']};
 assert.deepEqual(parseReadOnlyServerSettingsFacts(raw),{serverId:'server',name:'Home',version:'1.0',signIn:'local',readOnly:true});
 const calls:unknown[][]=[];
 const api={request:async<T>(...args:unknown[]):Promise<T>=>{calls.push(args);return raw as T;}};
 assert.equal((await loadReadOnlyServerSettingsFacts(api,'server')).readOnly,true);
 assert.equal(calls.length,1);assert.equal(calls[0]?.[0],'/v1/system');assert.equal(calls[0]?.[1],'GET');
 await assert.rejects(loadReadOnlyServerSettingsFacts(api,'different'));
 assert.throws(()=>parseReadOnlyServerSettingsFacts({...raw,signIn:'unknown'}));
 assert.throws(()=>parseReadOnlyServerSettingsFacts({...raw,name:'bad\nname'}));
});

test('navigation dots are a set of qualified major-issue sections with no counts',()=>{
 type Issue=Parameters<typeof settingsSectionsWithMajorIssues>[0][number];
 const issue=(code:string,severity:string,kind:string)=>({code,severity,kind} as Issue);
 const sections=settingsSectionsWithMajorIssues([
  issue('library_unreachable','playback_stop','library'),issue('library_unreachable','playback_stop','library'),
  issue('remote_unreachable','playback_stop','remote-source'),issue('storage_nearly_full','data_risk','storage'),
  issue('backup_failed','data_risk','backups'),issue('secure_renewal_failed','data_risk','networking'),
  issue('library_unreachable','warning','library'),issue('diagnostic','data_risk','storage'),
 ]);
 assert.deepEqual([...sections],['libraries','remote-sources','storage','remote']);
});
