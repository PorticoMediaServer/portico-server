import test from 'node:test';
import assert from 'node:assert/strict';
import {parseConnectionDocument,parseConnectivityStatus} from '../src/server-administration.ts';
import {settingsBinding,readSettingsBinding,settingsBindingPatch} from '../src/presentation/settings-bindings.ts';

const connection={secure:false,remote:false,plainHttpOutsideHome:false,clientAddress:'127.0.0.1',accessUrls:['https://advertised.example']};
const status={connection,observedAt:'2026-10-23T00:00:00Z',policy:{revision:1,remoteSignInPolicy:'allow',remoteBitrateLimitKbps:0,connectionLimit:0,advertiseUrl:'',lanNetworks:[],accessUrls:[],lanDiscoveryEnabled:false},tls:{},discovery:{},addresses:[],interfaces:[],warnings:[]};
test('managed HTTPS observation is optional, strict and independent of advertised URLs',()=>{
 assert.equal(parseConnectionDocument(connection).managedHttpsOrigin,undefined);
 for(const origin of ['https://route.example','https://route.example:443','https://route.example:41234'])assert.equal(parseConnectionDocument({...connection,managedHttpsOrigin:origin}).managedHttpsOrigin,origin);
 for(const origin of ['http://route.example','https://user:password@route.example','https://route.example/path','https://route.example?ticket=x','https://route.example#x',true])assert.equal(parseConnectionDocument({...connection,managedHttpsOrigin:origin}).managedHttpsOrigin,undefined);
});
test('identity code comes only from the snapshot and remains read-only with own address off',()=>{
 const parsed=parseConnectivityStatus({...status,identityCode:'JJWY-C8J8-J26F',localUrl:'http://localhost:41234'}),binding=settingsBinding('remote.identityCode')!;
 assert.equal(readSettingsBinding(binding,{connectivityStatus:parsed})?.current,'JJWY-C8J8-J26F');
 assert.equal(readSettingsBinding(binding,{forms:{connectivity:{identityCode:'JJWY-C8J8-J26F'}}}),undefined);
 assert.throws(()=>settingsBindingPatch(binding,'changed'));
 assert.equal(parsed.localUrl,'http://localhost:41234');
 assert.equal(parseConnectivityStatus({...status,localUrl:'http://localhost'}).localUrl,'http://localhost');
 for(const localUrl of ['http://other:41234','https://localhost:41234','http://localhost:0','http://localhost:41234/path'])assert.equal(parseConnectivityStatus({...status,localUrl}).localUrl,undefined);
 assert.equal(parseConnectivityStatus({...status,identityCode:'IIII-OOOO-0000'}).identityCode,undefined);
});
