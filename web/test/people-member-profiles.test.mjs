import assert from 'node:assert/strict';
import test from 'node:test';
import {componentModule} from './helpers/component-harness.mjs';
import {createRenderer, tree, all, textOf} from './helpers/interaction-harness.mjs';
import {MemberProfilesClient} from '../../packages/client-core/src/member-profiles.ts';
import {defaultI18n} from '../../packages/i18n/src/index.ts';

// OWN-001: a person's Profiles tab lists their real profiles, and Manage edits
// one through the owner-scoped routes, revision-fenced, never the viewer's own.

const restrictions = (profileId, patch = {}) => ({profileId, ratingSystem: '', maximumAgeRating: '', maximumAge: -1, allowUnrated: true, blockedLabels: [],
  allowDownloads: true, allowLiveTv: true, allowDvr: true, allowWatchTogether: true, revision: 1, ...patch});
const profile = (id, patch = {}) => ({id, name: id === 'main' ? 'Nana' : 'Movie night', art: 'coral', primary: id === 'main', position: id === 'main' ? 0 : 1, revision: 3,
  pinRequired: id !== 'main', pinRevision: 1, allowedLibraries: [], restrictions: restrictions(id), ...patch});
const systems = [{id: 'mpaa', name: 'MPA', values: [{code: 'G', label: 'G', minimumAge: 0}, {code: 'PG', label: 'PG', minimumAge: 8}]}];

async function mount() {
  const calls = [];
  const request = async (path, method = 'GET', body) => {
    calls.push({path, method, body});
    if (method === 'GET') return {accountId: 'nana', items: [profile('main'), profile('movies')]};
    if (method === 'PATCH') return profile('movies', {name: body.name ?? 'Movie night', revision: 4});
    if (method === 'PUT') return profile('movies', {name: 'Film club', revision: 4, restrictions: restrictions('movies', {ratingSystem: body.ratingSystem, maximumAgeRating: body.maximumAgeRating, maximumAge: 8, revision: 2})});
    return profile('movies', {pinRequired: false, revision: 4});
  };
  const api = {forPerson: accountId => new MemberProfilesClient({request}, accountId), ratingSystems: async () => systems};
  const h = createRenderer();
  const passthrough = name => props => ({type: name, props});
  const m = await componentModule(new URL('../src/screens/settings/PeopleMemberProfiles.tsx', import.meta.url), {
    react: h.react,
    '@i18n': {defaultI18n},
    '@design': {profileArtColors: new Proxy({}, {get: () => '#000'})},
    '@core/presentation/errors.ts': {presentError: error => ({body: String(error?.message ?? error)})},
    '../../kit/controls': {Button: 'Button', RichSelect: 'RichSelect', Switch: 'Switch', TextField: 'TextField'},
    '../../kit/Icon': {Icon: 'Icon'},
    '../../kit/overlays/Dialog': {Dialog: props => ({type: 'Dialog', props: {title: props.title, open: props.open, children: props.open ? [props.children, props.footer] : null}})},
    '../../kit/settings': {SettingsAction: 'SettingsAction', SettingsStringList: passthrough('StringList')},
    './account-profile/profile.module.css': {default: new Proxy({}, {get: (_, key) => String(key)})},
    './people-member-profiles.module.css': {default: new Proxy({}, {get: (_, key) => String(key)})},
  });
  const render = () => tree(h.render(() => m.PeopleMemberProfiles({api, accountId: 'nana', canEdit: true})));
  const settle = async (turns = 8) => {let value; for (let i = 0; i < turns; i++) {value = render(); await new Promise(resolve => setTimeout(resolve, 0));} return value;};
  return {calls, settle};
}
const button = (rendered, label) => all(rendered, 'Button').filter(node => textOf(node) === label);

test('the Profiles tab lists the person\'s real profiles with what sets each apart, and Manage edits through the owner routes', async () => {
  const app = await mount();
  let value = await app.settle();
  assert.equal(app.calls[0].path, '/v1/admin/access/members/nana/profiles');
  const text = textOf(value);
  for (const expected of ['Nana', 'Movie night', 'Main profile', 'Protected by a PIN']) assert.ok(text.includes(expected), expected);
  const manage = button(value, 'Manage');
  assert.equal(manage.length, 2);
  manage[1].props.onClick();
  value = await app.settle();
  const dialog = all(value, 'Dialog').find(node => node.props.open && node.props.title === 'Movie night');
  assert.ok(dialog, 'Manage opens the editor for that profile');
  all(tree(dialog), 'TextField')[0].props.onChange({currentTarget: {value: 'Film club'}});
  value = await app.settle(2);
  all(value, 'RichSelect')[0].props.onValueChange('PG');
  value = await app.settle(2);
  button(value, 'Save')[0].props.onClick();
  await app.settle();
  const writes = app.calls.filter(call => call.method !== 'GET');
  assert.deepEqual(writes.map(call => [call.method, call.path]), [
    ['PATCH', '/v1/admin/access/members/nana/profiles/movies'],
    ['PUT', '/v1/admin/access/members/nana/profiles/movies/restrictions'],
  ]);
  assert.deepEqual(writes[0].body, {expectedRevision: 3, name: 'Film club'});
  assert.equal(writes[1].body.expectedRevision, 1);
  assert.equal(writes[1].body.ratingSystem, 'mpaa');
  assert.equal(writes[1].body.maximumAgeRating, 'PG');
});

test('removing a PIN asks first and then clears it through the owner route', async () => {
  const app = await mount();
  let value = await app.settle();
  button(value, 'Manage')[1].props.onClick();
  value = await app.settle();
  button(value, 'Remove PIN')[0].props.onClick();
  value = await app.settle(2);
  assert.equal(app.calls.some(call => call.method === 'POST'), false, 'nothing is cleared before the confirmation');
  const confirm = all(value, 'SettingsAction').find(node => textOf(node) === 'Remove PIN');
  confirm.props.onClick();
  await app.settle();
  const reset = app.calls.find(call => call.method === 'POST');
  assert.deepEqual(reset, {path: '/v1/admin/access/members/nana/profiles/movies/pin-reset', method: 'POST', body: {expectedRevision: 3}});
});
