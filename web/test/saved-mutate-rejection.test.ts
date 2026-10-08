import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const ui = Object.fromEntries(['AnchoredMenu','Button','Input','ListRow','Loading','MenuHeading','MenuRow','MenuSeparator','Notice','Text'].map(x => [x, x]));
function nodes(tree: any): any[] {
  if (!tree || typeof tree !== 'object') return [];
  if (Array.isArray(tree)) return tree.flatMap(nodes);
  return [tree, ...nodes(tree.props?.children), ...nodes(tree.props?.action)];
}
const tick = () => new Promise(r => setTimeout(r, 0));

// Follow-up 2 (Q7): SavedService.mutate now rejects on retry-required/conflict.
// The playlist picker's create path must surface that rejection in its existing
// error Notice (never a silent catch), and must not close or report success.
test('playlist picker create failure surfaces the rejection instead of closing', async () => {
  const h = hooks();
  const snap: any = {phase: 'ready', projection: {sections: []}, pagination: {cursor: null, next: []}};
  let selected = 0;
  let mutated = 0;
  const service = {
    select: async () => { selected++; },
    next: async () => {},
    mutate: async () => { mutated++; throw Object.assign(new Error('denied'), {code: 'denied'}); },
  };
  const done: string[] = [];
  const app = await componentModule(new URL('../src/screens/shared/EntryActions.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {},
    '../../app/together': {},
    '../../app/downloads': {},
    '../../app/i18n': {useI18n: () => defaultI18n, currentI18n: () => defaultI18n},
    '../../app/delete-media': {},
    '@core/library-content.ts': {},
    '@core/saved.ts': {},
    '@core/personal-saved.ts': {},
    '@core/queue-controller.ts': {},
    '../../app/viewer-scope': {useViewerScope: () => ({})},
    '../../app/session': {useSession: () => ({api: {}, session: {}})},
    '../../app/not-interested':{announceNotInterested:()=>{},markNotInterested:async()=>({undo:async()=>{}}),withoutHidden:(r:any)=>r},'../../app/not-interested-notice':{useNotInterested:()=>({hidden:new Set(),notice:null})},'@core/recommendation-feedback.ts':{isRecommendationRow:()=>false},'../../app/continue-watching': {announceRemoval: () => {}, removeFromContinueWatching: async () => ({undo: async () => {}})},
    '../../app/metadata-editor': {},
    '../../app/detail': {},
    '@core/presentation/index.ts': {viewerScope: () => ({})},
    '../../app/content': {useService: () => ({service, snapshot: snap})},
    '../../app/errors': {errorText: (e: any) => e?.message ?? 'failed'},
    '../../app/open': {},
    '../../player/engine': {},
    '../../ui': ui,
    './Sections': {useAppendedPages: (_scope: string, _cursor: string | null, page: readonly unknown[]) => page},
    './playlist-add': {addItemsToPlaylist: async () => ({ok: 0, failed: [] as string[]}), addItemsToPlaylistViaJob: async () => ({ok: 0, failed: [] as string[]})},
    './bulk-job': {runBulkJobs: async () => ({ok: 0, failed: [], jobs: 0})},
    './container-watched': {containerKindFor: () => undefined, useContainerWatched: () => ({state: null, loading: false, saving: false, error: null, refresh: () => {}, setWatched: async () => false})},
  });
  const render = () => h.render(() => app.PlaylistPicker({itemId: 'm1', itemTitles: ['Movie'], onDone: (v: string) => done.push(v)}));
  let all = nodes(render());
  const input = all.find(n => n.type === 'Input');
  assert.ok(input, 'picker names the new playlist through an Input');
  input.props.onChange({target: {value: 'Party'}});
  all = nodes(render());
  const create = all.find(n => n.type === 'Button' && n.props?.variant === 'primary');
  assert.ok(create, 'picker offers a primary create action');
  create.props.onClick();
  await tick();
  await tick();
  all = nodes(render());
  assert.equal(mutated, 1);
  assert.equal(selected, 1);
  assert.deepEqual(done, [], 'a rejected create never reports success');
  const notice = all.find(n => n.type === 'Notice');
  assert.ok(notice, 'the rejection surfaces in the picker error Notice');
  const text = Array.isArray(notice.props.children) ? notice.props.children.join(' ') : String(notice.props.children ?? '');
  assert.match(text, /denied/, 'the Notice carries the failure reason');
});
