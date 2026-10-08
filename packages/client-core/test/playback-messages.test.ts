import test from 'node:test';
import assert from 'node:assert/strict';
import {playbackUserMessage} from '../src/playback-messages.ts';
test('playback messages preserve actionable categories without exposing arbitrary server text',()=>{
 assert.match(playbackUserMessage({code:'owner_server_cap',message:'secret session 123'}),/simultaneous stream limit/);
 assert.match(playbackUserMessage({status:403}),/does not have access/);
 assert.match(playbackUserMessage({code:'receipt_expired'}),/could not be confirmed/);
 assert.equal(playbackUserMessage('http://server/private?token=secret'), 'Playback could not continue. Please try again.');
 assert.equal(playbackUserMessage({message:JSON.stringify({account:'secret'})}), 'Playback could not continue. Please try again.');
 assert.match(playbackUserMessage(new TypeError('Failed to fetch')),/Check your connection/);
});

test('trusted control outage message describes the automatic recovery',()=>{
 assert.equal(playbackUserMessage('Playback control disconnected. Retrying…'),'Reconnecting playback controls…');
});
