import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from './transform-tsx.mjs';

// The page fences a selection on the app router's rendered location; the test
// supplies that boundary rather than compiling the whole router tree.
const routerModule = `export const router = {state: {location: {href: '/', pathname: '/', search: {}, searchStr: ''}}, subscribe: () => () => {}, navigate: () => {}};`;
registerHooks({
  resolve(specifier, context, next) {
    if ((specifier === './router' || specifier === '../router' || specifier.endsWith('/router')) && context.parentURL?.includes('/web-next/src/')) {
      return {url: 'remote-sources:router', shortCircuit: true};
    }
    return next(specifier, context);
  },
  load(url, context, next) {
    if (url === 'remote-sources:router') return {format: 'module', source: routerModule, shortCircuit: true};
    if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
    if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
    return next(url, context);
  },
});
const {defaultI18n} = await import('../../packages/i18n/src/index.ts');
const {sourceReadPreset, sourceReadPresetName} = await import('../../packages/client-core/src/remote-sources.ts');
const {RemoteSourcesPage, remoteErrorText} = await import('../src/screens/settings/remote-sources/RemoteSourcesPage.tsx');
// The page's reader admits the server's original empty list, with its viewer fence.
const pageProps: Parameters<typeof RemoteSourcesPage>[0] = {
  api: {request: async () => ({scope: {serverId: 'server', viewerFence: 'fence'}, sources: [], mountSupported: false})} as never,
  serverId: 'server', viewerId: 'owner', isCurrent: () => true, reportError: () => {},
};

test('the four reading choices are the server SourceReadPreset values', () => {
  assert.deepEqual(sourceReadPreset('list'), {scanDepth: 'file_list_only', wholeFileReads: false, connections: 2, requestsPerSecond: 2, bytesPerSecond: 0});
  assert.deepEqual(sourceReadPreset('once'), {scanDepth: 'basic', wholeFileReads: false, connections: 4, requestsPerSecond: 4, bytesPerSecond: 0});
  assert.deepEqual(sourceReadPreset('everything'), {scanDepth: 'complete', wholeFileReads: false, connections: 8, requestsPerSecond: 10, bytesPerSecond: 0});
  assert.deepEqual(sourceReadPreset('ask'), {scanDepth: 'basic', wholeFileReads: false, connections: 2, requestsPerSecond: 2, bytesPerSecond: 0});
  assert.equal(sourceReadPresetName(sourceReadPreset('everything')), 'everything');
  assert.equal(sourceReadPresetName({...sourceReadPreset('once'), connections: 6}), undefined);
});

test('the Remote sources page draws the add action and its empty state', async () => {
  // The page owns its reader in a layout pass, so a static render of the page only
  // draws its loading status. Drive the page's own pass, then draw the body it commits.
  const React = await import('react');
  const internals = (React as any).__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;
  const previous = internals.H;
  const slots: any[] = []; let cursor = 0, effects: (() => void)[] = [], dirty = false;
  const changed = (deps: any[], old: any[]) => !old || deps.some((value, index) => value !== old[index]);
  const dispatcher = {
    useState(initial: any) {const i = cursor++;if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial;
      return [slots[i], (value: any) => {slots[i] = typeof value === 'function' ? value(slots[i]) : value;dirty = true;}];},
    useRef(initial: any) {const i = cursor++;return slots[i] ??= {current: initial};},
    useEffect(effect: () => void, deps: any[]) {const i = cursor++, old = slots[i];if (changed(deps, old)) {slots[i] = deps;effects.push(effect);}},
    useLayoutEffect(effect: () => void, deps: any[]) {const i = cursor++, old = slots[i];if (changed(deps, old)) {slots[i] = deps;effects.push(effect);}},
    useMemo(factory: () => any, deps: any[]) {const i = cursor++, old = slots[i];
      if (changed(deps, old?.deps)) slots[i] = {deps, value: factory()};
      return slots[i].value;},
    useCallback(fn: any, deps: any[]) {const i = cursor++, old = slots[i];
      if (changed(deps, old?.deps)) slots[i] = {deps, value: fn};
      return slots[i].value;},
  };
  let tree: any;
  try {
    internals.H = dispatcher;
    do {dirty = false;cursor = 0;effects = [];tree = RemoteSourcesPage({...pageProps});
      for (const effect of effects) effect();
    } while (dirty);
  } finally {internals.H = previous;}
  // The reader's own mount read is the server's empty source list.
  await new Promise(resolve => setImmediate(resolve));
  const markup = renderToStaticMarkup(tree);
  assert.match(markup, /Add remote source/);
  assert.match(markup, /No remote sources yet/);
  assert.match(markup, /give Portico a folder on another computer/);
});

test('the source states and the blocked sentence have their drawn words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.state.unreachable'), 'Can’t be reached');
  assert.equal(defaultI18n.t('next.remoteSources.state.credentials'), 'Needs its sign-in again');
  assert.equal(defaultI18n.t('next.remoteSources.state.waiting'), 'Waiting');
  assert.equal(defaultI18n.t('next.remoteSources.state.limited'), 'Limited');
  assert.match(defaultI18n.t('next.remoteSources.problem.range_unsupported'), /read parts of a file/);
});

test('a hostile transport message is never shown; only a bounded source error is', () => {
  const hostile = Object.assign(new Error('https://private-user:private-password@private.example/media/'), {code: 'remote_offline'});
  const text = remoteErrorText(hostile);
  assert.equal(text, defaultI18n.t('next.remoteSources.error.generic'));
  assert.doesNotMatch(text, /private-user|private-password|private\.example/);
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'remote_credentials_required'})), defaultI18n.t('next.remoteSources.problem.credentials_required'));
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'refresh_failed'})), defaultI18n.t('next.remoteSources.error.refreshFailed'));
});

test('saved Add handlers cannot adopt a reopened form lifetime', async t => {
  const React = await import('react');
  const {AddSourceDialog} = await import('../src/screens/settings/remote-sources/RemoteSourcesPage.tsx');
  // The drawn dialog asks the real phone media query; this is its narrowest boundary.
  const previousWindow = (globalThis as any).window;
  (globalThis as any).window = {matchMedia: () => ({matches: false, addEventListener() {}, removeEventListener() {}})};
  t.after(() => {if (previousWindow === undefined) delete (globalThis as any).window; else (globalThis as any).window = previousWindow;});
  // Execute the real dialog hooks and effects, retaining callbacks from open A.
  const internals = (React as any).__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;
  const previous = internals.H;
  // The drawn dialog mounts its loaded body as a second component, so each keeps
  // its own hook slots across the passes of its own lifetime.
  const passes = new Map<any, any[]>();
  const changed = (deps: any[] | undefined, old: any[] | undefined) => !deps || !old || deps.some((value, index) => value !== old[index]);
  const mount = (component: any, props: any) => {
    const slots = passes.get(component) ?? passes.set(component, []).get(component)!;
    let cursor = 0, effects: (() => void)[] = [], dirty = false, tree: any;
    const dispatcher = {
      useState(initial: any) {const i = cursor++;if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial;
        return [slots[i], (value: any) => {slots[i] = typeof value === 'function' ? value(slots[i]) : value;dirty = true;}];},
      useRef(initial: any) {const i = cursor++;return slots[i] ??= {current: initial};},
      useLayoutEffect(effect: () => void, deps: any[]) {const i = cursor++, old = slots[i];
        if (changed(deps, old)) {slots[i] = deps;effects.push(effect);}},
      useCallback(fn: any, deps: any[]) {const i = cursor++, old = slots[i];
        if (changed(deps, old?.deps)) slots[i] = {deps, value: fn};
        return slots[i].value;},
      useEffect(effect: () => void, deps: any[]) {const i = cursor++, old = slots[i];
        if (changed(deps, old)) {slots[i] = deps;effects.push(effect);}},
      useMemo(factory: () => any, deps: any[]) {const i = cursor++, old = slots[i];
        if (changed(deps, old?.deps)) slots[i] = {deps, value: factory()};
        return slots[i].value;},
      useSyncExternalStore(subscribe: (onChange: () => void) => () => void, getSnapshot: () => any) {
        const i = cursor++;
        if (!slots[i]) {slots[i] = {};effects.push(() => {subscribe(() => {dirty = true;});});}
        return getSnapshot();},
    };
    do {dirty = false;cursor = 0;effects = [];internals.H = dispatcher;
      try {tree = component(props);}
      finally {internals.H = previous;}
      for (const effect of effects) effect();
    } while (dirty);
    return tree;
  };
  let io = 0, completions = 0, open = true;
  const service = {testConnection: async () => {io++;return {origin: 'https://a.example', rangeSupport: 'unknown'};},
    configure: async () => {io++;}, getSnapshot: () => ({sources: [], busy: false, ambiguous: false})} as any;
  const api = {request: async () => ({})} as never;
  // One identity per callback: the body's owned resources compare dependencies by identity.
  const isCurrent = () => true, reportError = () => completions++, onClose = () => completions++, onCreated = () => completions++;
  const render = () => {
    const dialog = mount(AddSourceDialog, {service, api, serverId: 'server', viewerId: 'owner', open, isCurrent, reportError, onClose, onCreated});
    return mount(dialog.type, {...dialog.props, open});
  };
  const elements = (node: any): any[] => Array.isArray(node) ? node.flatMap(elements) : node && typeof node === 'object' && node.props ? [node, ...elements(node.props.children), ...elements(node.props.footer)] : [];
  const button = (tree: any, label: string) => elements(tree).find(node => node.props.children === defaultI18n.t(label as any));
  let tree = render();
  // The drawn wizard opens on what to connect; WebDAV is its default family.
  button(tree, 'next.remoteSources.continue').props.onClick(); tree = render();
  for (const [label, value] of [['name', 'A'], ['address', 'https://a.example'], ['password', 'secret-A']]) {
    elements(tree).find(node => node.props.label === defaultI18n.t(`next.remoteSources.${label}` as any)).props.onChange({target: {value}});tree = render();
  }
  const oldTest = button(tree, 'next.remoteSources.test').props.onClick;
  oldTest();await new Promise(resolve => setImmediate(resolve));tree = render();
  const oldCreate = button(tree, 'next.remoteSources.saveAndContinue').props.onClick;
  assert.equal(button(tree, 'next.remoteSources.saveAndContinue').props.disabled, false);
  button(tree, 'next.remoteSources.back').props.onClick(); tree = render();
  button(tree, 'next.remoteSources.cancel').props.onClick(); open = false; render(); open = true; tree = render();
  const before = {io, completions, state: JSON.stringify([...passes.values()])};
  oldTest();await oldCreate();await new Promise(resolve => setImmediate(resolve));
  assert.equal(io, before.io, 'old form sends no provider test or mutation');
  assert.equal(completions, before.completions, 'old form cannot close or complete successor');
  assert.equal(JSON.stringify([...passes.values()]), before.state, 'successor hook state stays unchanged');
});

test('the imported rclone kind has its drawn words and its bound program error', () => {
  assert.equal(defaultI18n.t('next.remoteSources.kindRclone'), 'Imported rclone source');
  assert.match(defaultI18n.t('next.remoteSources.connectRclone'), /Paste a configuration/);
  assert.match(defaultI18n.t('next.remoteSources.configurationHelp'), /never shows it again/);
  assert.match(defaultI18n.t('next.remoteSources.bundledProgram'), /comes with Portico Server/);
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'administration_unavailable'})), defaultI18n.t('next.remoteSources.problem.administration_unavailable'));
  assert.equal(remoteErrorText(Object.assign(new Error('x'), {code: 'remote_administration_unavailable'})), defaultI18n.t('next.remoteSources.problem.administration_unavailable'));
});

test('the account-configuration kind has its drawn words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.connectFromConfig'), 'From my config');
  assert.match(defaultI18n.t('next.remoteSources.configPath'), /name and kind/);
  assert.match(defaultI18n.t('next.remoteSources.configEncrypted'), /encrypted/);
  assert.match(defaultI18n.t('next.remoteSources.configChecksOnSave'), /tests the remote when you save/);
});

test('the S3 kind has its drawn fields and its own refusal words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.connectS3'), 'S3 storage');
  assert.match(defaultI18n.t('next.remoteSources.s3EndpointHelp'), /https/);
  assert.match(defaultI18n.t('next.remoteSources.s3Checks'), /nothing is typed in rclone/);
  assert.match(defaultI18n.t('next.remoteSources.s3Invalid'), /Check the endpoint/);
});

test('the Google Drive service-account kind has its drawn words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.connectDrive'), 'Google Drive');
  assert.equal(defaultI18n.t('next.remoteSources.driveServiceAccount'), 'Service account key');
  assert.match(defaultI18n.t('next.remoteSources.driveKeyHelp'), /never shows it again/);
  assert.match(defaultI18n.t('next.remoteSources.driveKeyOAuth'), /OAuth client, not a service account key/);
  assert.match(defaultI18n.t('next.remoteSources.driveCannotSee'), /shared with the address above/);
  assert.match(defaultI18n.t('next.remoteSources.driveOAuthUnavailable'), /not available on this server yet/);
});

test('the OneDrive/Dropbox token kind has its drawn words', () => {
  assert.equal(defaultI18n.t('next.remoteSources.connectCloud'), 'OneDrive or Dropbox');
  assert.equal(defaultI18n.t('next.remoteSources.cloudOneDrive'), 'OneDrive');
  assert.equal(defaultI18n.t('next.remoteSources.cloudDropbox'), 'Dropbox');
  assert.match(defaultI18n.t('next.remoteSources.cloudTokenHelp'), /never shows it back/);
  assert.match(defaultI18n.t('next.remoteSources.cloudTokenClient'), /not a signed-in token/);
  assert.match(defaultI18n.t('next.remoteSources.cloudTokenGoogle'), /Google credential file/);
});
