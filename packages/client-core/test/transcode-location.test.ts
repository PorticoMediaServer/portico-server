import test from 'node:test';
import assert from 'node:assert/strict';
import {transcodeLocationChoice, transcodeLocationTarget} from '../src/presentation/transcode-location.ts';

const directory = {path: '/srv/portico/transcode', fallback: false, memoryPath: '/dev/shm'};

test('the stored directory chooses separate, memory or folder; capacity facts answer until it loads', () => {
  assert.equal(transcodeLocationChoice(directory, '').value, 'separate');
  assert.equal(transcodeLocationChoice(directory, '/dev/shm').value, 'memory');
  assert.equal(transcodeLocationChoice(directory, '/dev/shm/portico').value, 'memory');
  assert.equal(transcodeLocationChoice(directory, '/dev/shmem').value, 'folder', 'a sibling path is not the RAM file system');
  assert.equal(transcodeLocationChoice(directory, '/mnt/fast').value, 'folder');
  assert.equal(transcodeLocationChoice(directory).value, 'folder');
  assert.equal(transcodeLocationChoice({...directory, fallback: true}).value, 'separate');
  assert.equal(transcodeLocationChoice({...directory, memoryPath: ''}, '/dev/shm').value, 'folder');
  assert.equal(transcodeLocationChoice({...directory, memoryPath: ''}).memoryAvailable, false);
});

test('a choice writes the directory it names; folder writes nothing; memory needs a RAM file system', () => {
  assert.equal(transcodeLocationTarget('separate', directory), '');
  assert.equal(transcodeLocationTarget('memory', directory), '/dev/shm');
  assert.equal(transcodeLocationTarget('folder', directory), undefined);
  assert.equal(transcodeLocationTarget('memory', {memoryPath: ''}), undefined);
  assert.equal(transcodeLocationTarget('memory', undefined), undefined);
});
