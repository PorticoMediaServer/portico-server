/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn browse toolbar does what it says.
 *
 * The same toolbar is drawn on Library, Saved, a person's titles and a Home row's See all. Its
 * model is the shared client-core `toolbarModel`; these tests drive the real controls against it:
 * the Sort menu and its direction flip, the quick toggles, the applied-filter chips and Clear all,
 * Save view's availability, and the compact bar's count. No server: facets are stubbed.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {sortDirectionKind} from '@core/presentation/index.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, textOf, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

const field = (id: string, patch: any = {}) => ({id, labelKey: `browse.field.${id}`, type: 'enum', operators: ['in', 'equals'], controlHint: 'facet-multi-select', complexity: 'simple', cost: 'cheap', applicableKinds: ['movie'], ...patch});
const sort = (id: string, labelKey: string, direction: 'asc' | 'desc', patch: any = {}) => ({id, labelKey, directions: ['asc', 'desc'], defaultDirection: direction, expensive: false, applicableKinds: ['movie'], ...patch});
const capabilities = {
  library: {id: 'library', name: 'Movies', kind: 'movie', defaultView: 'browse', pinned: false},
  pivots: [],
  resolvedPivot: {id: 'movies', labelKey: 'browse.pivot.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['browse'], browsable: true},
  fields: [field('genre'), field('year', {type: 'number'}), field('resolution'), field('contentRating'), field('audioLanguage')],
  sorts: [sort('title', 'sort.title', 'asc'), sort('added', 'sort.added', 'desc'), sort('random', 'sort.random', 'asc', {directions: ['asc'], seeded: true})],
  quickFilters: [
    {id: 'unwatched', labelKey: 'browse.quick.unwatched', query: {field: 'watched', operator: 'equals', value: false}},
    {id: 'favorites', labelKey: 'browse.quick.favorites', query: {field: 'favorite', operator: 'equals', value: true}},
  ],
  queryLimits: {maximumDepth: 3, maximumClauses: 20, maximumBytes: 8192, maximumSorts: 2, defaultLimit: 60, maximumLimit: 120, cursorTtlMs: 60000},
};

async function mount(options: {predicates?: any[]; sort?: any; compact?: boolean; total?: number; save?: boolean} = {}) {
  const h = createRenderer();
  const view = {predicates: options.predicates ?? [], sort: options.sort ?? {field: 'title', direction: 'asc'}};
  const calls: {name: string; value?: any}[] = [];
  let reshuffles = 0;
  const app = await componentModule(new URL('../src/screens/library/BrowseFilters.tsx', import.meta.url), {
    react: h.react,
    '../../app/browse': {
      fieldLabel: (id: string, labelKey: string) => labelKey || id,
      predicateLabel: (p: any) => `${p.field}:${Array.isArray(p.value) ? p.value.join('+') : p.value}`,
      reshuffle: () => { reshuffles++; },
      useFacets: () => () => Promise.resolve([]),
    },
    '../../app/session': {useSession: () => ({owner: false})},
    '../../app/content': {browseCountLabel: (total: number) => `${total} titles`, quickFilterLabelForKind: (label: string) => label},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../ui': {
      Button: 'Button', Checkbox: 'Checkbox', Chip: 'Chip', Chips: 'Chips', Icon: 'Icon', Input: 'Input', Menu: 'Menu',
      Popover: ({trigger}: any) => trigger, Spinner: 'Spinner', Text: 'Text', cx: (...names: any[]) => names.filter(Boolean).join(' '),
      useCompact: () => !!options.compact,
    },
    './BrowseFilters.module.css': {default: {}},
  });
  const draw = () => app.BrowseBar({
    libraryId: 'library', capabilities, predicates: view.predicates, sort: view.sort,
    onChange: (next: any[]) => { calls.push({name: 'change', value: next}); view.predicates = next; },
    onSort: (next: any) => { calls.push({name: 'sort', value: next}); view.sort = next; },
    total: options.total, onSave: options.save ? () => calls.push({name: 'save'}) : undefined,
    libraryKind: 'movie', pivot: 'movies',
  });
  h.render(draw);
  const flat = () => tree(h.render(draw));
  return {h, flat, calls, view, reshuffles: () => reshuffles};
}

const menu = (rendered: any[]) => one(rendered, 'Menu');
const sortChipLabel = (active: any) => t('lib.sortChip', {field: `sort.${active.field === 'title' ? 'title' : active.field === 'added' ? 'added' : 'random'}`, direction: t('web.filters.direction', {kind: sortDirectionKind(active.field), dir: active.direction})});

test('the Sort control names the current order and offers the server’s sorts', async () => {
  const screen = await mount();
  const rendered = screen.flat();
  const trigger = tree(menu(rendered).props.trigger).find(node => node.type === 'Chip')!;
  assert.equal(trigger.props.label, sortChipLabel({field: 'title', direction: 'asc'}));
  assert.equal(trigger.props['aria-label'], t('web.filters.sortBy'));
  assert.deepEqual(menu(rendered).props.items.map((item: any) => item.id), ['title', 'added', 'random']);
  assert.equal(menu(rendered).props.items.find((item: any) => item.selected).id, 'title');
});

test('choosing a sort starts in its own direction; choosing the current one turns it round', async () => {
  const screen = await mount();
  menu(screen.flat()).props.onSelect('added');
  assert.deepEqual(screen.calls.at(-1), {name: 'sort', value: {field: 'added', direction: 'desc'}});
  assert.equal(tree(menu(screen.flat()).props.trigger).find(node => node.type === 'Chip').props.label, sortChipLabel({field: 'added', direction: 'desc'}));
  // The current field again flips; Random has no direction to flip, so it draws another shuffle.
  menu(screen.flat()).props.onSelect('added');
  assert.deepEqual(screen.calls.at(-1), {name: 'sort', value: {field: 'added', direction: 'asc'}});
  const random = await mount({sort: {field: 'random', direction: 'asc'}});
  menu(random.flat()).props.onSelect('random');
  assert.equal(random.reshuffles(), 1);
  assert.ok(!random.calls.some(call => call.name === 'sort'));
});

test('the quick toggles add and remove their condition, and replace the same field', async () => {
  const screen = await mount();
  const unwatched = () => all(screen.flat(), 'Chip').find(chip => chip.props.label === 'browse.quick.unwatched')!;
  assert.equal(unwatched().props.pressed, false);
  unwatched().props.onClick();
  assert.deepEqual(screen.calls.at(-1), {name: 'change', value: [{field: 'watched', operator: 'equals', value: false}]});
  // Lit while it filters, and the same click turns it off.
  assert.equal(unwatched().props.pressed, true);
  unwatched().props.onClick();
  assert.deepEqual(screen.calls.at(-1), {name: 'change', value: []});
  // Turning on the other field's quick toggle replaces this one rather than stacking two.
  const favorites = () => all(screen.flat(), 'Chip').find(chip => chip.props.label === 'browse.quick.favorites')!;
  unwatched().props.onClick();
  favorites().props.onClick();
  assert.deepEqual(screen.calls.at(-1).value, [{field: 'watched', operator: 'equals', value: false}, {field: 'favorite', operator: 'equals', value: true}]);
});

test('applied filters are removable chips with Clear all beside them', async () => {
  const screen = await mount({predicates: [{field: 'genre', operator: 'in', value: ['Drama']}, {field: 'year', operator: 'between', value: [1990, 1999]}]});
  const rendered = screen.flat();
  const applied = all(rendered, 'Chip').filter(chip => chip.props.pressed && chip.props.label.includes(':'));
  assert.deepEqual(applied.map(chip => chip.props.label).sort(), ['genre:Drama', 'year:1990+1999']);
  applied.find(chip => chip.props.label === 'genre:Drama').props.onClick();
  assert.deepEqual(screen.calls.at(-1), {name: 'change', value: [{field: 'year', operator: 'between', value: [1990, 1999]}]});
  all(screen.flat(), 'Button').find(button => button.props.label === t('web.filters.clearAll'))!.props.onClick();
  assert.deepEqual(screen.calls.at(-1), {name: 'change', value: []});
});

test('Save view is offered only once the view differs from the one it opens with', async () => {
  let screen = await mount({save: true});
  const save = () => all(screen.flat(), 'Button').find(button => button.props['aria-label'] === t('web.filters.saveView'))!;
  assert.equal(save().props.disabled, true, 'nothing to save at the opening order');
  const filtered = await mount({save: true, predicates: [{field: 'genre', operator: 'in', value: ['Drama']}]});
  const enabled = all(filtered.flat(), 'Button').find(button => button.props['aria-label'] === t('web.filters.saveView'))!;
  assert.equal(enabled.props.disabled, false);
  enabled.props.onClick();
  assert.deepEqual(filtered.calls.at(-1), {name: 'save'});
});

test('the phone bar keeps the count and every filter inside one control', async () => {
  const screen = await mount({compact: true, total: 42, predicates: [{field: 'genre', operator: 'in', value: ['Drama']}]});
  const rendered = screen.flat();
  assert.equal(all(rendered, 'Chip').filter(chip => chip.props.label.startsWith('browse.quick.')).length, 0);
  assert.ok(!textOf(rendered).includes('42 titles'));
  const add = all(rendered, 'Chip').find(chip => chip.props.label === t('web.filters.buttonCount', {count: 1}))!;
  assert.ok(add, 'the filter control counts what is applied');
});
