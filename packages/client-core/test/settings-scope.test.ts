import test from 'node:test';
import assert from 'node:assert/strict';
import {settingsScopeLabel, settingsStructure, type SettingsPlatform, type SettingsRow} from '../src/presentation/index.ts';
import {createI18n} from '../../i18n/src/index.ts';

function rows(platform: SettingsPlatform): SettingsRow[] {
  const structure = settingsStructure({platform, owner: false, capabilities: {}} as Parameters<typeof settingsStructure>[0]);
  return structure.headings.flatMap(h => h.pages.flatMap(p => p.sections.flatMap(s => s.rows)));
}

test('a setting the server keeps per kind of device names the kind, never "This device"', () => {
  const i18n = createI18n({locale: 'en-US'} as Parameters<typeof createI18n>[0]);
  assert.equal(i18n.t(settingsScopeLabel('deviceClass', 'web')!), 'All web browsers');
  assert.equal(i18n.t(settingsScopeLabel('deviceClass', 'phone')!), 'All phones and tablets');
  assert.equal(i18n.t(settingsScopeLabel('deviceClass', 'tv')!), 'All TVs');
  assert.equal(i18n.t(settingsScopeLabel('device', 'tv')!), 'This device');
  assert.equal(settingsScopeLabel('profile', 'web'), undefined);
  assert.equal(settingsScopeLabel(undefined, 'web'), undefined);
});

test('rows stored in the registry are never marked as this one device, and rows the device keeps are', () => {
  for (const platform of ['web', 'phone', 'tv'] as const) {
    for (const row of rows(platform)) {
      if (row.kind === 'preference') assert.notEqual(row.scope, 'device', `${row.key} is stored on the server`);
      if (row.kind === 'composite' && row.storage === 'registry') assert.notEqual(row.scope, 'device', `${row.id} is stored on the server`);
      if (row.kind === 'composite' && row.storage === 'device') assert.equal(row.scope, 'device', `${row.id} is kept by the device`);
    }
  }
  // The saved quality away from home is the server's `quality.remote`, kept per kind of device.
  const quality = rows('web').find(row => row.kind === 'preference' && row.key === 'quality.remote');
  assert.equal(quality && 'scope' in quality ? quality.scope : undefined, 'deviceClass');
});

test('the Player setting is offered only where there is a second player to choose, never on the web, and belongs to the one device', () => {
  const find = (platform: SettingsPlatform, playerEngines?: boolean) => settingsStructure({platform, owner: false, capabilities: {playerEngines}} as Parameters<typeof settingsStructure>[0])
    .headings.flatMap(h => h.pages.flatMap(p => p.sections.flatMap(s => s.rows))).find(row => row.kind === 'composite' && row.id === 'playerEngine');
  assert.equal(find('phone'), undefined, 'a build without mpv has nothing to choose');
  assert.equal(find('web', true), undefined);
  for (const platform of ['phone', 'tv'] as const) {
    const row = find(platform, true);
    assert.ok(row && row.kind === 'composite');
    assert.equal(row.storage, 'device');
    assert.equal(row.scope, 'device');
    assert.deepEqual(row.control.type === 'choice' ? row.control.choices.map(c => c.value) : [], ['automatic', 'native', 'mpv']);
  }
  const i18n = createI18n({locale: 'en-US'} as Parameters<typeof createI18n>[0]);
  assert.equal(i18n.t('settings.row.player'), 'Player');
  assert.equal(i18n.t('settings.player.automatic'), 'Automatic (recommended)');
  assert.equal(i18n.t('saved.removal.fromPlaylist', {title: 'Dune'}), 'Removed “Dune” from the playlist.');
  assert.equal(i18n.t('player.cannotPlay.gone', {title: 'Dune'}), 'Dune is no longer on this server.');
});


test('secondary local profiles retain their profile controls without account security or devices', () => {
 const panels = (manage: boolean) => settingsStructure({platform:'web', owner:false, capabilities:{localAccount:true, manageSignIn:manage, manageDevices:manage, deviceName:manage}})
  .headings.flatMap(h=>h.pages.flatMap(p=>p.sections.flatMap(s=>s.rows))).filter(r=>r.kind==='custom').map(r=>r.id);
 const secondary=panels(false), primary=panels(true);
 assert(secondary.includes('profiles'));
 for(const id of ['password','twoStep','devices','deviceName']){
  assert(!secondary.includes(id), id+' requires the primary profile');
  assert(primary.includes(id), id+' remains available to the primary profile');
 }
});
