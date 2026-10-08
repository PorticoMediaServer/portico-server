import test from 'node:test';import assert from 'node:assert/strict';import {readFileSync} from 'node:fs';
import {declaredProfiles} from './declared-profile-fixtures.ts';
import {clientProfileProblem} from '../src/client-profile.ts';
test('Go planner capability fixtures exactly match the declared client tables',()=>{
 for(const [name,profile] of Object.entries(declaredProfiles)) assert.equal(clientProfileProblem(profile),null,name);
 assert.deepEqual(JSON.parse(JSON.stringify(declaredProfiles)),JSON.parse(readFileSync(new URL('../../../server/internal/playback/testdata/client-profiles.json',import.meta.url),'utf8')));
});
