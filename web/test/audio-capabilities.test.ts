import test from 'node:test';
import assert from 'node:assert/strict';
import {probeAudioDecode} from '../src/bridge/audio/capabilities.ts';
import {clientProfileProblem} from '../../packages/client-core/src/client-profile.ts';

test('audioDecode declares only what passed the fixtures on this engine and the browser confirms', async () => {
  const all = async () => true;
  const blink = await probeAudioDecode(all, 'blink');
  assert.deepEqual(blink.map(a => [a.codec, a.via]), [['pcm', 'pcm'], ['flac', 'webcodecs'], ['mp3', 'webcodecs'], ['aac', 'webcodecs'], ['opus', 'webcodecs']]);
  const webkit = await probeAudioDecode(all, 'webkit');
  assert.equal(webkit.find(a => a.codec === 'flac')!.via, 'js-flac', 'Safari’s WebCodecs FLAC failed the fixtures: the JavaScript decoder plays FLAC');
  assert.equal(webkit.find(a => a.codec === 'alac'), undefined, 'ALAC decodes nowhere here: the server converts it to FLAC');
  const gecko = await probeAudioDecode(all, 'gecko');
  assert.deepEqual(gecko.map(a => a.codec), ['pcm', 'flac'], 'an engine never measured declares only what is decoded here');
  const none = await probeAudioDecode(undefined, 'blink');
  assert.deepEqual(none.map(a => [a.codec, a.via]), [['pcm', 'pcm'], ['flac', 'js-flac']], 'no WebCodecs: FLAC and PCM still play');
  const limited = await probeAudioDecode(async c => c.sampleRate <= 48000 && c.numberOfChannels <= 2, 'blink');
  assert.equal(limited.find(a => a.codec === 'aac')!.maxSampleRate, 48000);
  assert.equal(limited.find(a => a.codec === 'aac')!.maxChannels, 2);
});

test('the client profile carries audioDecode and passes the admission rules', async () => {
  const audioDecode = (await probeAudioDecode(async () => true, 'blink')).map(a => ({...a, containers: [...a.containers]}));
  assert.equal(clientProfileProblem({version: 1, client: {family: 'browser'}, evidence: 'probed', audioDecode}), null);
});
