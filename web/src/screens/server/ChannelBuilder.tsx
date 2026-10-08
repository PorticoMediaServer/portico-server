import React, {useEffect, useMemo, useRef, useState} from 'react';
import {librarySampleLabel, previewLibraryChannel, saveLibraryChannel, type LibraryChannelConfig, type LibraryChannelOrder, type LibraryChannelPreview, type LibraryChannelRule} from '@core/library-channels';
import {ApiError} from '@core/index.ts';
import {parseBrowseResult, type BrowseFieldCapability, type BrowseNode} from '@core/browse.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {useAction} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {useLibrariesContext} from '../../app/libraries';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {fieldLabel, useBrowseCapabilities, type Predicate} from '../../app/browse';
import {FieldControl} from '../library/BrowseFilters';
import {Button, Checkbox, Dialog, Input, Menu, Notice, Select, Spinner, Switch, Text, TextArea, type MenuItem} from '../../ui';

/**
 * The Library Channels builder (Spec — Custom Channels Builder §7). A rule is "what plays":
 * libraries, movies and/or shows, conditions in the library browse language (the same field
 * controls as library filters), words the title or summary mentions, and pinned titles. The
 * preview on the right updates as you edit: how many titles match, hours of programming, a
 * sample, and today's schedule exactly as saving would make it.
 */

/** Personal fields never select a shared lineup (Justin, 23 Sep); the server refuses them too. */
const PERSONAL = new Set(['playState', 'favorite', 'watchlisted', 'personalRating', 'lastPlayedAt', 'entityKind']);

type Cond = {key: string; field: string; predicates: Predicate[]; not: boolean};
type Group = {key: string; mode: 'all' | 'any'; conds: Cond[]; not: boolean};
type Item = Cond | Group;
type Conditions = {mode: 'all' | 'any'; items: Item[]; opaque?: BrowseNode};

let seq = 0;
const nextKey = () => `k${++seq}`;
const isGroup = (i: Item): i is Group => 'conds' in i;

function condNode(c: Cond): BrowseNode | undefined {
  if (!c.predicates.length) return undefined;
  const node: BrowseNode = c.predicates.length === 1 ? c.predicates[0]! : {all: c.predicates};
  return c.not ? {not: node} : node;
}
function toNode(c: Conditions): BrowseNode | undefined {
  if (c.opaque) return c.opaque;
  const nodes = c.items.map(i => {
    if (!isGroup(i)) return condNode(i);
    const inner = i.conds.map(condNode).filter((n): n is BrowseNode => !!n);
    if (!inner.length) return undefined;
    const g: BrowseNode = i.mode === 'all' ? {all: inner} : {any: inner};
    return i.not ? {not: g} : g;
  }).filter((n): n is BrowseNode => !!n);
  if (!nodes.length) return undefined;
  if (nodes.length === 1 && c.mode === 'all') return nodes[0];
  return c.mode === 'all' ? {all: nodes} : {any: nodes};
}
const isPredicate = (n: BrowseNode): n is Predicate => 'field' in n;
function toCond(n: BrowseNode): Cond | undefined {
  let not = false;
  if ('not' in n) { not = true; n = n.not; }
  if (isPredicate(n)) return {key: nextKey(), field: n.field, predicates: [n], not};
  if ('all' in n && n.all.length && n.all.every(isPredicate) && new Set(n.all.map(p => (p as Predicate).field)).size === 1) return {key: nextKey(), field: (n.all[0] as Predicate).field, predicates: n.all as Predicate[], not};
  return undefined;
}
/**
 * Templates (and older channels) state genre and year in the query's own fields rather than
 * the filter. The builder shows and edits only conditions, and folds them into the filter when
 * it saves (clearing those fields), so they become conditions here; otherwise "Family movies"
 * or "Eighties movies" would open showing no conditions and save as every movie.
 */
export function withLegacyCriteria(c: Conditions, query: {genres?: readonly string[]; yearFrom?: number; yearThrough?: number} | undefined): Conditions {
  if (!query || c.opaque) return c;
  const extra: Cond[] = [];
  if (query.genres?.length) extra.push({key: nextKey(), field: 'genre', predicates: [{field: 'genre', operator: 'contains-any', value: [...query.genres]} as Predicate], not: false});
  const from = query.yearFrom || 0, through = query.yearThrough || 0;
  const year: Predicate | undefined = from && through ? {field: 'year', operator: 'between', value: [from, through]} as Predicate
    : from ? {field: 'year', operator: 'at-least', value: from} as Predicate
    : through ? {field: 'year', operator: 'at-most', value: through} as Predicate : undefined;
  if (year) extra.push({key: nextKey(), field: 'year', predicates: [year], not: false});
  if (!extra.length) return c;
  // Legacy criteria all had to hold, so they join an "all" list (wrapping an "any" list in a group).
  if (c.mode === 'all' || !c.items.length) return {mode: 'all', items: [...c.items, ...extra]};
  if (c.items.some(isGroup)) return {mode: 'all', items: [], opaque: {all: [toNode(c)!, ...extra.map(condNode).filter((n): n is BrowseNode => !!n)]}};
  return {mode: 'all', items: [{key: nextKey(), mode: 'any', conds: c.items.filter((i): i is Cond => !isGroup(i)), not: false}, ...extra]};
}

function fromNode(node: BrowseNode | undefined): Conditions {
  if (!node) return {mode: 'all', items: []};
  const single = toCond(node);
  if (single) return {mode: 'all', items: [single]};
  const top = 'all' in node ? {mode: 'all' as const, list: node.all} : 'any' in node ? {mode: 'any' as const, list: node.any} : undefined;
  if (!top) return {mode: 'all', items: [], opaque: node};
  const items: Item[] = [];
  for (const child of top.list) {
    const cond = toCond(child);
    if (cond) { items.push(cond); continue; }
    let not = false, inner: BrowseNode = child;
    if ('not' in inner) { not = true; inner = inner.not; }
    const list = 'all' in inner ? inner.all : 'any' in inner ? inner.any : undefined;
    const conds = list?.map(toCond);
    if (!list || !conds || conds.some(c => !c)) return {mode: 'all', items: [], opaque: node};
    items.push({key: nextKey(), mode: 'all' in inner ? 'all' : 'any', conds: conds as Cond[], not});
  }
  return {mode: top.mode, items};
}

type Ordering = 'shuffle' | 'random' | 'inOrder' | 'thenShuffle';
const orderingOf = (r: LibraryChannelRule): Ordering => (r.mode === 'weighted-random' ? 'random' : r.mode === 'sequential' ? 'inOrder' : r.mode === 'sequential-then-shuffle' ? 'thenShuffle' : 'shuffle');
const modeOf: Record<Ordering, LibraryChannelRule['mode']> = {shuffle: 'shuffle-bag', random: 'weighted-random', inOrder: 'sequential', thenShuffle: 'sequential-then-shuffle'};

/** The builder field a server validation `path` names (`rules[0].query.kinds` → kinds). */
function fieldOfPath(path: string | undefined): string | undefined {
  if (!path) return undefined;
  if (/\.filter(?:[.[]|$)/.test(path)) return 'conditions';
  const last = path.split('.').pop()?.replace(/\[\d+\]$/, '') ?? '';
  return ({name: 'name', libraryIds: 'libraries', kinds: 'kinds', text: 'mentions', includeItemIds: 'pins', excludeItemIds: 'pins', timezone: 'timezone'} as Record<string, string>)[last];
}
const serverFieldMessage: Record<string, string> = {name: 'builder.error.name', libraries: 'builder.error.libraries', kinds: 'builder.error.kinds', conditions: 'builder.error.conditions', mentions: 'builder.error.mentions', pins: 'builder.error.pins', timezone: 'builder.error.timeZone'};

/** What the builder checks before asking the server (the server checks again). */
function problems(d: LibraryChannelConfig, t: (id: never) => string) {
  const out: Record<string, string> = {};
  const rule = d.rules[0];
  if (!d.name.trim()) out.name = t('builder.error.name' as never);
  if (rule && !rule.query.libraryIds.length) out.libraries = t('builder.error.libraries' as never);
  if (rule && !rule.query.kinds.length && !(rule.query.includeItemIds ?? []).length) out.kinds = t('builder.error.kinds' as never);
  try { new Intl.DateTimeFormat('en-US', {timeZone: d.timezone}); } catch { out.timezone = t('builder.error.timeZone' as never); }
  return out;
}

export function ChannelEditor({value, onClose, onSaved}: {value: {config: LibraryChannelConfig; revision: number; templateName?: string} | null; onClose: () => void; onSaved: () => void}) {
  const {api, session} = useSession();
  const i18n = useI18n();
  const t = i18n.t;
  const serverId = session?.viewer.serverId ?? '';
  const libraries = useLibrariesContext();
  const action = useAction();
  const [draft, setDraft] = useState<LibraryChannelConfig | null>(null);
  const [conditions, setConditions] = useState<Conditions>({mode: 'all', items: []});
  const [advanced, setAdvanced] = useState(false);
  const [tried, setTried] = useState(false);
  // The server refused the channel itself (400 invalid_library_channel), as opposed to a network or permission failure.
  const [rejected, setRejected] = useState(false);
  // The field the server named when it refused the channel; cleared as soon as the channel changes.
  const [serverField, setServerField] = useState<string>();
  // BE-API-10: one request ID per logical save; retries reuse it, edits mint a new one.
  const [opIds] = useState(createOperationIds);
  useEffect(() => {
    const config = value ? structuredClone(value.config) : null;
    setDraft(config);
    setConditions(withLegacyCriteria(fromNode(config?.rules[0]?.query.filter), config?.rules[0]?.query));
    setAdvanced(false);
    setTried(false);
    setRejected(false);
    setServerField(undefined);
    action.clear();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);

  const rule = draft?.rules.find(r => r.id === draft.defaultRuleId) ?? draft?.rules[0];
  const filter = useMemo(() => toNode(conditions), [conditions]);
  // The config the server sees: the draft with the conditions folded into the rule's filter.
  const config = useMemo<LibraryChannelConfig | null>(() => {
    // CH1: Library Channels are always shared; the server refuses any other audience.
    if (!draft || !rule) return draft ? {...draft, viewerAccess: 'server-members'} : draft;
    const query = {...rule.query, filter, genres: [], yearFrom: 0, yearThrough: 0};
    if (!filter) delete (query as {filter?: BrowseNode}).filter;
    return {...draft, viewerAccess: 'server-members', rules: draft.rules.map(r => (r.id === rule.id ? {...r, query} : r))};
  }, [draft, rule, filter]);
  const local = config ? problems(config, t as never) : {};
  const valid = !Object.keys(local).length;
  useEffect(() => setServerField(undefined), [config]);
  // Shown: the form's own checks once a save was tried, and the field the server named.
  const errors: Record<string, string | undefined> = {...(serverField ? {[serverField]: t(serverFieldMessage[serverField] as never)} : {}), ...(tried ? local : {})};

  const videoLibraries = libraries.items.filter(l => ['movie', 'tv', 'anime'].includes(l.kind));
  const chosen = videoLibraries.filter(l => rule?.query.libraryIds.includes(l.id));
  const movieLibrary = chosen.find(l => l.kind === 'movie');
  const showLibrary = chosen.find(l => l.kind !== 'movie');
  const movieCaps = useBrowseCapabilities(movieLibrary?.id, movieLibrary ? 'movies' : undefined);
  const showCaps = useBrowseCapabilities(showLibrary?.id, showLibrary ? 'shows' : undefined);
  const fields = useMemo(() => {
    const byId = new Map<string, {field: BrowseFieldCapability; libraryId: string}>();
    for (const [caps, lib] of [[movieCaps.capabilities, movieLibrary], [showCaps.capabilities, showLibrary]] as const) {
      for (const f of caps?.fields ?? []) if (!PERSONAL.has(f.id) && !byId.has(f.id) && lib) byId.set(f.id, {field: f, libraryId: lib.id});
    }
    return [...byId.values()].sort((a, b) => fieldLabel(a.field.id, a.field.labelKey).localeCompare(fieldLabel(b.field.id, b.field.labelKey)));
  }, [movieCaps.capabilities, showCaps.capabilities, movieLibrary, showLibrary]);

  const preview = useLivePreview(config, valid && !!rule?.query.libraryIds.length);

  if (!value || !draft || !rule || !config) return null;
  const setRule = (patch: Partial<LibraryChannelRule>) => setDraft({...draft, rules: draft.rules.map(r => (r.id === rule.id ? {...r, ...patch} : r))});
  const setQuery = (patch: Partial<LibraryChannelRule['query']>) => setRule({query: {...rule.query, ...patch}});
  const toggle = (list: readonly string[], id: string, on: boolean) => (on ? [...list.filter(x => x !== id), id] : list.filter(x => x !== id));
  const save = () => {
    setTried(true);
    setRejected(false);
    setServerField(undefined);
    if (local.timezone) setAdvanced(true);
    if (!valid) return;
    void action.run(async () => {
      const key = JSON.stringify({config, revision: value.revision});
      try { const out = await saveLibraryChannel(api, serverId, config, value.revision, opIds.forPayload(key)); opIds.release(); return out; } catch (e) {
        if (e instanceof ApiError && (e.code === 'invalid_library_channel' || e.code === 'library_channel_timezone_invalid')) {
          const field = e.code === 'library_channel_timezone_invalid' ? 'timezone' : fieldOfPath(e.path);
          setRejected(true);
          setServerField(field);
          if (field === 'timezone') setAdvanced(true);
        }
        throw e;
      }
    }).then(ok => ok && onSaved());
  };
  const fieldMenu: MenuItem[] = fields.map(f => ({id: f.field.id, label: fieldLabel(f.field.id, f.field.labelKey)}));
  const addCondition = (id: string, group?: Group) => {
    const cond: Cond = {key: nextKey(), field: id, predicates: [], not: false};
    setConditions(c => ({...c, items: group ? c.items.map(i => (i === group ? {...group, conds: [...group.conds, cond]} : i)) : [...c.items, cond]}));
  };
  const replace = (from: Item, to: Item | null, group?: Group) => setConditions(c => ({...c, items: group
    ? c.items.map(i => (i === group ? {...group, conds: group.conds.map(x => (x === from ? to : x)).filter((x): x is Cond => !!x)} : i))
    : c.items.map(i => (i === from ? to : i)).filter((x): x is Item => !!x)}));
  const condRow = (c: Cond, group?: Group) => {
    const f = fields.find(x => x.field.id === c.field);
    return (
      <div key={c.key} style={{display: 'grid', gridTemplateColumns: '160px 1fr auto auto', gap: 8, alignItems: 'start', padding: '8px 0', borderTop: '1px solid var(--color-line-soft)'}}>
        <Text variant="bodyStrong">{fieldLabel(c.field, f?.field.labelKey)}</Text>
        <div>{f ? <FieldControl libraryId={f.libraryId} field={f.field} predicates={c.predicates} onChange={next => replace(c, {...c, predicates: [...next]}, group)} /> : <Text variant="caption" tone="tertiary">{c.predicates.map(p => `${p.operator} ${JSON.stringify(p.value)}`).join(', ')}</Text>}</div>
        <Checkbox checked={c.not} onCheckedChange={v => replace(c, {...c, not: v}, group)} label={t('builder.exclude')} />
        <Button variant="ghost" size="sm" icon="close" label={t('builder.remove')} onClick={() => replace(c, null, group)} />
      </div>
    );
  };
  const matchSelect = (mode: 'all' | 'any', set: (m: 'all' | 'any') => void) => (
    <Select label={t('builder.conditions')} hideLabel value={mode} options={[{value: 'all', label: t('builder.matchAll')}, {value: 'any', label: t('builder.matchAny')}]} onChange={e => set(e.target.value as 'all' | 'any')} />
  );
  const ordering = orderingOf(rule);
  const title = value.templateName ? t('builder.reviewTemplate', {name: value.templateName}) : value.revision ? t('builder.editTitle') : t('builder.newTitle');
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={title} width={960} actions={<><Button variant="ghost" label={t('builder.cancel')} onClick={onClose} /><Button variant="primary" label={value.revision ? t('builder.save') : t('builder.create')} loading={action.busy} onClick={save} /></>}>
      <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1.4fr) minmax(260px, 1fr)', gap: 24, alignItems: 'start'}}>
        <div style={{display: 'flex', flexDirection: 'column', gap: 16, minWidth: 0}}>
          <Input label={t('builder.name')} value={draft.name} onChange={e => setDraft({...draft, name: e.target.value})} maxLength={120} autoFocus error={errors.name} />
          <TextArea label={t('builder.description')} optional value={draft.description} onChange={e => setDraft({...draft, description: e.target.value})} rows={2} maxLength={500} />

          <section style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            <Text as="h3" variant="heading">{t('builder.whatPlays')}</Text>
            <div>
              <Text as="div" variant="label" tone="tertiary" style={{marginBottom: 8}}>{t('builder.libraries')}</Text>
              <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 8}}>
                {videoLibraries.map(l => <Checkbox key={l.id} checked={rule.query.libraryIds.includes(l.id)} onCheckedChange={v => setQuery({libraryIds: toggle(rule.query.libraryIds, l.id, v)})} label={l.name} />)}
              </div>
              {!videoLibraries.length ? <Text variant="caption" tone="tertiary">{t('builder.noVideoLibraries')}</Text> : null}
              {errors.libraries ? <Text variant="caption" tone="danger">{errors.libraries}</Text> : null}
            </div>
            <div>
              <Text as="div" variant="label" tone="tertiary" style={{marginBottom: 8}}>{t('builder.kinds')}</Text>
              <div style={{display: 'flex', gap: 16}}>
                <Checkbox checked={rule.query.kinds.includes('movie')} onCheckedChange={v => setQuery({kinds: toggle(rule.query.kinds, 'movie', v) as LibraryChannelRule['query']['kinds']})} label={t('builder.movies')} />
                <Checkbox checked={rule.query.kinds.includes('episode')} onCheckedChange={v => setQuery({kinds: toggle(rule.query.kinds, 'episode', v) as LibraryChannelRule['query']['kinds']})} label={t('builder.shows')} />
              </div>
              {errors.kinds ? <Text variant="caption" tone="danger">{errors.kinds}</Text> : null}
            </div>

            <div>
              <div style={{display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap'}}>
                <Text variant="label" tone="tertiary">{t('builder.conditions')}</Text>
                {matchSelect(conditions.mode, mode => setConditions(c => ({...c, mode})))}
              </div>
              {conditions.opaque ? <Notice tone="info" compact>{t('builder.opaqueConditions')}</Notice> : null}
              {!conditions.opaque && !conditions.items.length ? <Text as="p" variant="caption" tone="tertiary">{t('builder.noConditions')}</Text> : null}
              {conditions.items.map(item => isGroup(item) ? (
                <div key={item.key} style={{margin: '8px 0', padding: '4px 12px', borderLeft: '3px solid var(--color-line-control)'}}>
                  <div style={{display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap'}}>
                    {matchSelect(item.mode, mode => replace(item, {...item, mode}))}
                    <Checkbox checked={item.not} onCheckedChange={v => replace(item, {...item, not: v})} label={t('builder.exclude')} />
                    <Menu label={t('builder.addCondition')} trigger={<Button variant="ghost" size="sm" icon="plus" label={t('builder.addCondition')} />} items={fieldMenu} onSelect={id => addCondition(id, item)} />
                    <Button variant="ghost" size="sm" icon="close" label={t('builder.remove')} onClick={() => replace(item, null)} />
                  </div>
                  {item.conds.map(c => condRow(c, item))}
                </div>
              ) : condRow(item))}
              {!conditions.opaque ? (
                <div style={{display: 'flex', gap: 8, marginTop: 8}}>
                  <Menu label={t('builder.addCondition')} trigger={<Button variant="secondary" size="sm" icon="plus" label={t('builder.addCondition')} disabled={!fieldMenu.length} />} items={fieldMenu} onSelect={id => addCondition(id)} />
                  <Button variant="ghost" size="sm" icon="plus" label={t('builder.addGroup')} disabled={!fieldMenu.length} onClick={() => setConditions(c => ({...c, items: [...c.items, {key: nextKey(), mode: 'any', conds: [], not: false}]}))} />
                </div>
              ) : null}
              {rule.query.kinds.includes('episode') ? <Text as="p" variant="caption" tone="tertiary">{t('builder.showsNote')}</Text> : null}
              {errors.conditions ? <Text as="p" variant="caption" tone="danger">{errors.conditions}</Text> : null}
            </div>

            <Input label={t('builder.mentions')} optional value={rule.query.text ?? ''} onChange={e => setQuery({text: e.target.value})} maxLength={200} help={t('builder.mentionsHint')} error={errors.mentions} />
            <PinList label={t('builder.alwaysInclude')} ids={rule.query.includeItemIds ?? []} libraryIds={rule.query.libraryIds} onChange={ids => setQuery({includeItemIds: ids})} />
            <PinList label={t('builder.neverInclude')} ids={rule.query.excludeItemIds ?? []} libraryIds={rule.query.libraryIds} onChange={ids => setQuery({excludeItemIds: ids})} />
            {errors.pins ? <Text as="p" variant="caption" tone="danger">{errors.pins}</Text> : null}
          </section>

          <section style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12}}>
            <Select label={t('builder.order')} value={ordering} options={(['shuffle', 'random', 'inOrder', 'thenShuffle'] as const).map(o => ({value: o, label: t(`builder.order.${o}` as never)}))} onChange={e => setRule({mode: modeOf[e.target.value as Ordering]})} />
            {ordering === 'inOrder' || ordering === 'thenShuffle' ? (
              <Select label={t('builder.sortBy')} value={rule.query.order} options={(['title', 'release', 'year', 'recent', 'oldest', 'rating', 'episode'] as const).map(o => ({value: o, label: t(`builder.sort.${o}` as never)}))} onChange={e => setQuery({order: e.target.value as LibraryChannelOrder})} />
            ) : null}
            {rule.query.kinds.includes('episode') ? (
              <Select label={t('builder.episodes')} value={rule.episodeMode} options={([['none', 'none'], ['in-order', 'inOrder'], ['marathon', 'marathon'], ['rotate', 'rotate'], ['randomized', 'randomized']] as const).map(([v, k]) => ({value: v, label: t(`builder.episodes.${k}` as never)}))} onChange={e => setRule({episodeMode: e.target.value as LibraryChannelRule['episodeMode']})} />
            ) : null}
            <Select label={t('builder.recent')} value={String(rule.query.recentDays)} options={[{value: '0', label: t('builder.recent.any')}, ...[30, 90, 365].map(n => ({value: String(n), label: t('builder.recent.days', {count: n})}))]} onChange={e => setQuery({recentDays: Number(e.target.value)})} />
            <Text variant="caption" tone="tertiary">{t('builder.audience')}</Text>
          </section>

          <div style={{display: 'flex', alignItems: 'center', gap: 12}}>
            <Switch checked={draft.enabled} onCheckedChange={v => setDraft({...draft, enabled: v})} label={t('builder.on')} /><Text variant="body">{t('builder.on')}</Text>
          </div>
          <Button variant="link" size="sm" label={advanced ? t('builder.hideAdvanced') : t('builder.advanced')} iconAfter={advanced ? 'chevronUp' : 'chevronDown'} onClick={() => setAdvanced(v => !v)} />
          {advanced ? (
            <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 12}}>
              <Select label={t('builder.whenDone')} value={rule.exhaustion} options={[{value: 'loop', label: t('builder.whenDone.loop')}, {value: 'slate', label: t('builder.whenDone.slate')}]} onChange={e => setRule({exhaustion: e.target.value as LibraryChannelRule['exhaustion']})} />
              <Input label={t('builder.avoidRepeats')} type="number" min={0} inputMode="numeric" value={rule.deduplicationWindow} onChange={e => setRule({deduplicationWindow: Math.max(0, Number(e.target.value) || 0)})} help={t('builder.avoidRepeatsHelp')} />
              <Select label={t('builder.quality')} value={draft.quality.mode} options={(['automatic', 'original', 'limited'] as const).map(q => ({value: q, label: t(`builder.quality.${q}` as never)}))} onChange={e => setDraft({...draft, quality: {...draft.quality, mode: e.target.value as LibraryChannelConfig['quality']['mode']}})} />
              <Input label={t('builder.timeZone')} value={draft.timezone} onChange={e => setDraft({...draft, timezone: e.target.value})} error={errors.timezone} />
            </div>
          ) : null}
          {action.error ? <Notice tone="error" compact>{rejected ? t(serverField ? 'builder.error.save' : 'builder.error.rejected') : action.error}</Notice> : null}
        </div>
        <PreviewPanel state={preview} ready={!!rule.query.libraryIds.length} />
      </div>
    </Dialog>
  );
}

type PreviewState = {data?: LibraryChannelPreview; loading: boolean; failed: boolean};

/** Re-runs the server preview 600 ms after the last edit; a newer edit cancels an older request. */
function useLivePreview(config: LibraryChannelConfig | null, enabled: boolean): PreviewState {
  const {api, session} = useSession();
  const serverId = session?.viewer.serverId ?? '';
  const [state, setState] = useState<PreviewState>({loading: false, failed: false});
  const key = enabled && config ? JSON.stringify(config) : '';
  const latest = useRef(config);
  latest.current = config;
  useEffect(() => {
    if (!key || !latest.current) { setState({loading: false, failed: false}); return; }
    const abort = new AbortController();
    const snapshot = latest.current;
    setState(s => ({...s, loading: true}));
    const timer = setTimeout(() => {
      previewLibraryChannel(api, serverId, snapshot, abort.signal).then(
        data => { if (!abort.signal.aborted) setState({data, loading: false, failed: false}); },
        () => { if (!abort.signal.aborted) setState(s => ({...s, loading: false, failed: true})); },
      );
    }, 600);
    return () => { clearTimeout(timer); abort.abort(); };
  }, [api, serverId, key]);
  return state;
}

function PreviewPanel({state, ready}: {state: PreviewState; ready: boolean}) {
  const i18n = useI18n();
  const t = i18n.t;
  const p = state.data;
  const eligible = p?.rules.reduce((n, r) => n + r.eligible, 0) ?? 0;
  const unresolved = p?.rules.reduce((n, r) => n + r.unresolved, 0) ?? 0;
  const hours = Math.round((p?.durationMs ?? 0) / 3_600_000);
  return (
    <aside aria-live="polite" style={{position: 'sticky', top: 0, display: 'flex', flexDirection: 'column', gap: 12, padding: 16, borderRadius: 12, background: 'var(--color-raised)', minWidth: 0}}>
      <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between'}}>
        <Text variant="label" tone="tertiary">{t('builder.preview')}</Text>
        {state.loading ? <span style={{display: 'inline-flex', alignItems: 'center', gap: 8}}><Spinner /><Text variant="caption" tone="tertiary">{t('builder.previewing')}</Text></span> : null}
      </div>
      {!ready ? <Text variant="caption" tone="tertiary">{t('builder.previewEmpty')}</Text> : null}
      {state.failed ? <Notice tone="warning" compact>{t('builder.error.preview')}</Notice> : null}
      {ready && p ? (
        <>
          <div>
            <Text as="div" variant="title">{p.complete ? t('builder.matches', {count: eligible}) : t('builder.matchesAtLeast', {count: i18n.number(eligible)})}</Text>
            {eligible ? <Text as="div" variant="body" tone="secondary">{t('builder.hours', {hours: Math.max(1, hours)})}</Text> : <Text as="div" variant="caption" tone="tertiary">{t('builder.noMatchesHelp')}</Text>}
            {unresolved ? <Text as="div" variant="caption" tone="tertiary">{t('builder.unresolved', {count: unresolved})}</Text> : null}
          </div>
          {eligible ? (
            <div>
              <Text as="div" variant="label" tone="tertiary" style={{marginBottom: 4}}>{t('builder.samples')}</Text>
              <ul style={{margin: 0, paddingLeft: 20}}>{p.rules.flatMap(r => r.sample).filter((s, i, all) => all.findIndex(o => librarySampleLabel(o) === librarySampleLabel(s)) === i).slice(0, 8).map(s => <li key={s.itemId}><Text variant="body">{librarySampleLabel(s)}</Text></li>)}</ul>
            </div>
          ) : null}
          {p.firstDay.length ? (
            <div>
              <Text as="div" variant="label" tone="tertiary" style={{marginBottom: 4}}>{t('builder.firstDay')}</Text>
              <ol style={{margin: 0, padding: 0, listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 4, maxHeight: 280, overflow: 'auto'}}>
                {p.firstDay.slice(0, 40).map(e => (
                  <li key={e.id} style={{display: 'grid', gridTemplateColumns: '72px 1fr', gap: 8}}>
                    <Text variant="caption" tone="tertiary">{i18n.time(e.startMs)}</Text>
                    <Text variant="caption" tone={e.itemId ? 'primary' : 'tertiary'}>{e.itemId ? e.title : t('builder.nothingScheduled')}</Text>
                  </li>
                ))}
              </ol>
            </div>
          ) : null}
        </>
      ) : null}
    </aside>
  );
}

/** Pinned titles: search the chosen libraries by title, pin a movie or a whole show. */
function PinList({label, ids, libraryIds, onChange}: {label: string; ids: readonly string[]; libraryIds: readonly string[]; onChange: (ids: string[]) => void}) {
  const {api} = useSession();
  const i18n = useI18n();
  const t = i18n.t;
  const libraries = useLibrariesContext();
  const [q, setQ] = useState('');
  const [found, setFound] = useState<ContentEntry[]>();
  const [names, setNames] = useState<Record<string, string>>({});
  useEffect(() => {
    const text = q.trim();
    if (text.length < 2) { setFound(undefined); return; }
    const abort = new AbortController();
    const timer = setTimeout(() => {
      const targets = libraries.items.filter(l => libraryIds.includes(l.id));
      Promise.all(targets.map(l => api.request<unknown>(`/v1/libraries/${encodeURIComponent(l.id)}/browse`, 'POST', {pivot: l.kind === 'movie' ? 'movies' : 'shows', query: {field: 'title', operator: 'contains', value: text}, limit: 6}, abort.signal).then(parseBrowseResult).then(r => r.entries, () => [] as ContentEntry[])))
        .then(lists => { if (!abort.signal.aborted) setFound(lists.flat().slice(0, 10)); });
    }, 250);
    return () => { clearTimeout(timer); abort.abort(); };
  }, [api, q, libraryIds, libraries.items]);
  return (
    <div>
      <Text as="div" variant="label" tone="tertiary" style={{marginBottom: 8}}>{label}</Text>
      <div style={{display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: ids.length ? 6 : 0}}>
        {ids.map(id => <Button key={id} variant="secondary" size="sm" iconAfter="close" label={names[id] ?? id} onClick={() => onChange(ids.filter(x => x !== id))} />)}
      </div>
      <Input label={t('builder.findTitle')} hideLabel placeholder={t('builder.findTitle')} value={q} onChange={e => setQ(e.target.value)} />
      {found ? (
        <div role="listbox" style={{display: 'flex', flexDirection: 'column', marginTop: 4}}>
          {found.length ? found.map(e => (
            <Button key={e.id} variant="ghost" size="sm" label={e.kind === 'show' ? `${e.title} · ${t('builder.pinnedShow')}` : e.title} onClick={() => { setNames(n => ({...n, [e.id]: e.title})); onChange([...ids.filter(x => x !== e.id), e.id]); setQ(''); }} />
          )) : <Text variant="caption" tone="tertiary">{t('builder.noTitles')}</Text>}
        </div>
      ) : null}
    </div>
  );
}
