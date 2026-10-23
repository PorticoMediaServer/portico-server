import test from 'node:test';
import assert from 'node:assert/strict';
import {canonicalMusicQualityLadder, canonicalQualityLadder} from '../src/types.ts';
import {canonicalMusicQualityLadder as playerMusic} from '../../player-core/src/quality-ladder.ts';
import {savedMusicQualityRequest} from '../../player-core/src/quality.ts';

test('generated music vocabulary has exact separate bitrates and data amounts',()=>{
 assert.deepEqual(canonicalMusicQualityLadder.map(q=>q.id),['original','320','256','192','128','96']);
 assert.deepEqual(canonicalMusicQualityLadder.map(q=>q.audioKbps),[0,320,256,192,128,96]);
 assert.deepEqual(canonicalMusicQualityLadder.map(q=>q.dataMBPerHour),[0,144,115.2,86.4,57.6,43.2]);
 assert.deepEqual(playerMusic,canonicalMusicQualityLadder);
 assert.equal(canonicalQualityLadder.length,9);
 for (const q of playerMusic) assert.deepEqual(savedMusicQualityRequest(q.id),q.audioKbps?{mode:'limit',maxAudioBitrateKbps:q.audioKbps}:{mode:'original'});
});
