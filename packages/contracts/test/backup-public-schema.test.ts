import test from 'node:test';
import assert from 'node:assert/strict';
import {decode} from '../src/runtime.ts';

test('backup policy responses expose protection facts without secret material', () => {
  const settings = {backupFolder: '/backups', includeArtwork: false, passphraseProtected: true, passphraseSet: true};
  for (const name of ['GetAdminBackupSettingsResponse', 'PutAdminBackupSettingsResponse']) {
    assert.doesNotThrow(() => decode(name, settings));
    for (const field of ['passphrase', 'passphraseSealed', 'masterKey']) {
      assert.throws(() => decode(name, {...settings, [field]: 'private'}));
    }
    assert.throws(() => decode(name, {...settings, passphraseSet: null}));
  }
});

test('backup inspection publishes only an actual non-null encryption fact', () => {
  for (const encrypted of [true, false]) {
    assert.doesNotThrow(() => decode('PostAdminBackupsInspectResponse', {encrypted}));
  }
  for (const input of [{}, {encrypted: null}, {encrypted: 'false'}, {encrypted: false, path: '/private'}, {encrypted: false, passphrase: 'private'}]) {
    assert.throws(() => decode('PostAdminBackupsInspectResponse', input));
  }
});

test('restore accepts optional passphrase input only at the declared top level', () => {
  const request = {operationId: 'restore-operation', source: {backupId: 'backup'}};
  assert.doesNotThrow(() => decode('PostAdminBackupsRestoreRequest', request));
  assert.doesNotThrow(() => decode('PostAdminBackupsRestoreRequest', {...request, passphrase: 'private'}));
  assert.throws(() => decode('PostAdminBackupsRestoreRequest', {...request, passphrase: null}));
  assert.throws(() => decode('PostAdminBackupsRestoreRequest', {...request, source: {...request.source, passphrase: 'private'}}));
});
