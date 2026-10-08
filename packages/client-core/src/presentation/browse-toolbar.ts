import type {BrowseCapabilities, BrowseFieldCapability, BrowseQuickFilter, BrowseSortCapability, BrowseSortSelection} from '../browse.ts';
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';

/**
 * One browse toolbar for every grid in the product (library pivots, categories, collections,
 * search "See all", Saved). The server publishes the vocabulary (fields, operators, sorts, quick
 * filters); this decides what the toolbar makes of it, the same way on web, phone and TV:
 *
 * - **Sort** is one control showing the field and the direction.
 * - **Promoted fields**: three or four filters per library kind sit on the bar as their own
 *   menus (Genre, Year, Resolution for movies…).
 * - **Quick toggles**: the play-state and favorite quick filters, as switches.
 * - **"+ Filter"** opens the rest, in three groups: your activity, about the title, about the file.
 * - **Applied filters** are removable chips on a second line, with a summary per field.
 *
 * Nothing here renders; each client draws it with its own controls.
 */

export type ToolbarPredicate = Readonly<{field: string; operator: string; value: unknown}>;
export type ToolbarGroupId = 'activity' | 'title' | 'file';

/** Fields the bar promotes, by library kind, in order. Only fields the server offers appear. */
const PROMOTED: Readonly<Record<string, readonly string[]>> = {
  movie: ['genre', 'year', 'resolution', 'contentRating'],
  tv: ['genre', 'year', 'network', 'contentRating'],
  anime: ['genre', 'year', 'studio'],
  music: ['genre', 'decade', 'artist'],
  audiobook: ['author', 'narrator', 'genre', 'series'],
  recordings: ['genre', 'year'],
};

/** Quick filters shown as toggles on the bar (the rest stay inside "+ Filter" as fields). */
const QUICK_ON_BAR = ['unwatched', 'in-progress', 'favorites'];

const ACTIVITY = new Set(['playState', 'favorite', 'watchlisted', 'personalRating', 'lastPlayedAt']);
const FILE = new Set(['resolution', 'dynamicRange', 'audioLanguage', 'durationSeconds', 'dateAdded', 'availability', 'entityKind']);
/** Fields that are a worse version of another field on the bar, or internal. */
const HIDDEN = new Set(['title', 'entityKind']);
/** Decade is Year with a coarser step; the Year control offers decades as presets. */
const FOLDED_INTO_YEAR = new Set(['decade']);

export function toolbarGroupOf(fieldId: string): ToolbarGroupId {
  if (ACTIVITY.has(fieldId)) return 'activity';
  if (FILE.has(fieldId)) return 'file';
  return 'title';
}

/** The order fields take inside a group: how often people reach for them, not registry order. */
const RANK: readonly string[] = ['genre', 'year', 'decade', 'contentRating', 'studio', 'network', 'collection', 'series', 'author', 'narrator', 'artist', 'album', 'actor', 'director', 'writer', 'tag', 'label', 'communityRating', 'criticRating', 'releaseDate',
  'playState', 'favorite', 'watchlisted', 'personalRating', 'lastPlayedAt',
  'resolution', 'dynamicRange', 'audioLanguage', 'durationSeconds', 'dateAdded', 'availability'];
const rankOf = (id: string) => { const i = RANK.indexOf(id); return i < 0 ? RANK.length : i; };

export type ToolbarGroup = Readonly<{id: ToolbarGroupId; fields: readonly BrowseFieldCapability[]}>;

export type ToolbarModel = Readonly<{
  /** Fields with their own control on the bar. */
  promoted: readonly BrowseFieldCapability[];
  /** Quick filters shown as toggles. */
  quick: readonly BrowseQuickFilter[];
  /** Everything else, grouped, for "+ Filter". Promoted fields appear here too, so one list is complete. */
  groups: readonly ToolbarGroup[];
  /** Sort choices in the order the menu lists them. */
  sorts: readonly BrowseSortCapability[];
  /** The current sort and whether it can be flipped. */
  sort: Readonly<{field: string; direction: 'asc' | 'desc'; flippable: boolean}>;
  /** Applied predicates that are not a quick toggle, one chip each. */
  applied: readonly ToolbarPredicate[];
}>;

export function toolbarModel(capabilities: BrowseCapabilities, libraryKind: string, predicates: readonly ToolbarPredicate[], sort: BrowseSortSelection, options: {owner?: boolean; /** False on a television: a filter that can only be typed is left out. */ textEntry?: boolean} = {}): ToolbarModel {
  const pivot = capabilities.resolvedPivot?.id ?? '';
  const audio = /artist|album|song/.test(pivot);
  const fields = capabilities.fields.filter(f => !HIDDEN.has(f.id) && !FOLDED_INTO_YEAR.has(f.id) && (options.owner || (f.id !== 'availability' && f.id !== 'match')) && (options.textEntry !== false || filterControlOf(f) !== 'text'));
  const byId = new Map(fields.map(f => [f.id, f]));
  const promoted = (PROMOTED[libraryKind] ?? PROMOTED.movie!).map(id => byId.get(id)).filter((f): f is BrowseFieldCapability => !!f).slice(0, 4);
  // Music has no "watched": its quick toggles are favorites only (played/unplayed stay inside Filter).
  const quick = capabilities.quickFilters.filter(q => QUICK_ON_BAR.includes(q.id) && !(audio && q.id !== 'favorites'));
  const quickNodes = quick.map(q => q.query as ToolbarPredicate).filter(n => n && 'field' in n);
  const same = (a: ToolbarPredicate, b: ToolbarPredicate) => a.field === b.field && a.operator === b.operator && JSON.stringify(a.value) === JSON.stringify(b.value);
  const applied = predicates.filter(p => !quickNodes.some(n => same(n, p)));
  const groups: ToolbarGroup[] = (['title', 'activity', 'file'] as const)
    .map(id => ({id, fields: fields.filter(f => toolbarGroupOf(f.id) === id).sort((a, b) => rankOf(a.id) - rankOf(b.id))}))
    .filter(g => g.fields.length > 0);
  const directions = capabilities.sorts.find(s => s.id === sort.field)?.directions ?? ['asc', 'desc'];
  return Object.freeze({promoted, quick, groups, sorts: capabilities.sorts, sort: Object.freeze({field: sort.field, direction: sort.direction, flippable: directions.length > 1}), applied});
}

/** The next sort when a menu item is chosen: the current field flips, another starts at its default. */
export function nextToolbarSort(capabilities: BrowseCapabilities, sort: BrowseSortSelection, fieldId: string): BrowseSortSelection {
  const cap = capabilities.sorts.find(s => s.id === fieldId);
  if (!cap) return sort;
  if (fieldId === sort.field) {
    const flipped = sort.direction === 'asc' ? 'desc' : 'asc';
    return cap.directions.includes(flipped) ? {field: fieldId, direction: flipped} : sort;
  }
  return {field: fieldId, direction: cap.defaultDirection};
}

/** The values a field currently selects, as the chip summarises them ("Drama, Comedy", "1990–1999", "Yes"). */
export function fieldSelectionSummary(field: BrowseFieldCapability, predicates: readonly ToolbarPredicate[], formatValue: (v: unknown) => string): string | undefined {
  const mine = predicates.filter(p => p.field === field.id);
  if (!mine.length) return undefined;
  const parts: string[] = [];
  let low: unknown, high: unknown;
  for (const p of mine) {
    switch (p.operator) {
      case 'between': if (Array.isArray(p.value)) { low = p.value[0]; high = p.value[1]; } break;
      case 'at-least': case 'greater-than': low = p.value; break;
      case 'at-most': case 'less-than': high = p.value; break;
      default: parts.push(Array.isArray(p.value) ? p.value.map(formatValue).join(', ') : formatValue(p.value));
    }
  }
  if (low !== undefined || high !== undefined) parts.push(low !== undefined && high !== undefined ? `${formatValue(low)}–${formatValue(high)}` : low !== undefined ? `≥ ${formatValue(low)}` : `≤ ${formatValue(high)}`);
  return parts.join(', ');
}

/** Decade presets for a Year control, from the library's oldest to newest year. */
export function decadePresets(minYear: number, maxYear: number): readonly {label: string; from: number; to: number}[] {
  if (!Number.isFinite(minYear) || !Number.isFinite(maxYear) || maxYear < minYear) return [];
  const out: {label: string; from: number; to: number}[] = [];
  for (let d = Math.floor(maxYear / 10) * 10; d >= Math.floor(minYear / 10) * 10; d -= 10) out.push({label: `${d}s`, from: d, to: d + 9});
  return out;
}

/**
 * How a field is chosen, the same on every client: a list of the library's values with counts,
 * a yes/no, one of a few choices, a range offered as presets, or (last resort) typed text.
 * Year is chosen by decade: the server counts decades, and a decade is a real field.
 */
export type FilterControl = 'values' | 'toggle' | 'choice' | 'range' | 'text';

export function filterControlOf(field: BrowseFieldCapability): FilterControl {
  if (field.id === 'year') return 'values';
  if (field.controlHint === 'facet-multi-select' || (field.type === 'enum' && (field.allowedValues?.length ?? 0) > 4)) return 'values';
  if (field.type === 'boolean' || field.controlHint === 'toggle') return 'toggle';
  if (field.type === 'enum' && field.allowedValues) return 'choice';
  if (field.type === 'number' || field.type === 'date' || field.controlHint === 'number-range' || field.controlHint === 'date-range') return 'range';
  return 'text';
}

/** The field a values list reads and writes: Year is offered and stored by decade. */
export function filterValuesField(field: BrowseFieldCapability): Readonly<{facet: string; field: string; operator: string}> {
  if (field.id === 'year') return {facet: 'decade', field: 'decade', operator: 'in'};
  return {facet: field.facetSource?.field ?? field.id, field: field.id, operator: field.operators.includes('contains-any') ? 'contains-any' : 'in'};
}

export type RangePreset = Readonly<{id: string; label: Readonly<{id: MessageId; values?: MessageValues}>; predicates: readonly ToolbarPredicate[]}>;

/**
 * A range as choices a person makes ("Under 90 min", "8 or higher", "In the last 30 days"), never
 * two number boxes. `now` dates the recency windows.
 */
export function rangePresets(field: BrowseFieldCapability, now: Date): readonly RangePreset[] {
  const id = field.id;
  const atLeast = (value: number | string): ToolbarPredicate => ({field: id, operator: 'at-least', value});
  const atMost = (value: number | string): ToolbarPredicate => ({field: id, operator: 'at-most', value});
  const between = (low: number, high: number): readonly ToolbarPredicate[] => ((field.operators as readonly string[]).includes('between') ? [{field: id, operator: 'between', value: [low, high]}] : [atLeast(low), atMost(high)]);
  if (id === 'durationSeconds') {
    const minutes = (n: number) => n * 60;
    return [
      {id: 'under-30', label: {id: 'filter.preset.durationUnder', values: {minutes: 30}}, predicates: [atMost(minutes(30))]},
      {id: '30-60', label: {id: 'filter.preset.durationBetween', values: {from: 30, to: 60}}, predicates: between(minutes(30), minutes(60))},
      {id: '60-90', label: {id: 'filter.preset.durationBetween', values: {from: 60, to: 90}}, predicates: between(minutes(60), minutes(90))},
      {id: '90-120', label: {id: 'filter.preset.durationBetween', values: {from: 90, to: 120}}, predicates: between(minutes(90), minutes(120))},
      {id: 'over-120', label: {id: 'filter.preset.durationOver', values: {minutes: 120}}, predicates: [atLeast(minutes(120))]},
    ];
  }
  if (id === 'personalRating') return [5, 4, 3, 2].map(stars => ({id: `stars-${stars}`, label: {id: 'filter.preset.stars' as MessageId, values: {count: stars}}, predicates: [atLeast(stars)]}));
  if (id === 'criticRating') return [90, 80, 70, 60].map(value => ({id: `critic-${value}`, label: {id: 'filter.preset.percent' as MessageId, values: {value}}, predicates: [atLeast(value)]}));
  if (field.type === 'number') return [9, 8, 7, 6, 5].map(value => ({id: `rating-${value}`, label: {id: 'filter.preset.atLeast' as MessageId, values: {value}}, predicates: [atLeast(value)]}));
  if (field.type === 'date') {
    const since = (days: number) => new Date(now.getTime() - days * 86_400_000).toISOString().slice(0, 10);
    return [
      ...[7, 30, 90].map(days => ({id: `last-${days}`, label: {id: 'filter.preset.lastDays' as MessageId, values: {count: days}}, predicates: [atLeast(since(days))]})),
      {id: 'last-365', label: {id: 'filter.preset.lastYear'}, predicates: [atLeast(since(365))]},
    ];
  }
  return [];
}

/** The preset a field's current predicates match, if any. */
export function activeRangePreset(presets: readonly RangePreset[], predicates: readonly ToolbarPredicate[]): string | undefined {
  const key = (list: readonly ToolbarPredicate[]) => JSON.stringify(list.map(p => [p.operator, p.value]));
  const mine = key(predicates);
  return presets.find(p => key(p.predicates) === mine)?.id;
}

/**
 * The words on an applied filter's chip, the same on every client: "Year: 1990s, 2000s",
 * "Critic rating ≥ 8", "No collection". A decade is Year's value, so it reads as Year.
 * `fieldName` is the client's label for a field id.
 */
export function appliedFilterLabel(p: ToolbarPredicate, fieldName: (fieldId: string) => string, t: (id: MessageId, values?: MessageValues) => string): string {
  const decade = p.field === 'decade';
  const field = fieldName(decade ? 'year' : p.field);
  const one = (v: unknown) => (decade ? `${String(v)}s` : v === true ? t('lib.yes') : v === false ? t('lib.no') : browseValueLabel(String(v)));
  const value = Array.isArray(p.value) ? p.value.map(one).join(', ') : p.value === null || p.value === undefined ? '' : one(p.value);
  switch (p.operator) {
    case 'is-present': return t('filter.applied.has', {field: field.toLowerCase()});
    case 'is-missing': return t('filter.applied.missing', {field: field.toLowerCase()});
    case 'not-equals': case 'not-in': return t('filter.applied.not', {field, value});
    case 'at-least': case 'greater-than': return `${field} ≥ ${value}`;
    case 'at-most': case 'less-than': return `${field} ≤ ${value}`;
    case 'between': return t('filter.applied.is', {field, value: Array.isArray(p.value) ? `${one(p.value[0])}–${one(p.value[1])}` : value});
    case 'starts-with': return t('filter.applied.startsWith', {field, value});
    default: return t('filter.applied.is', {field, value});
  }
}

/** Enum values as people say them ("HDR10", "4K"), not as wire tokens (CON-04). */
const VALUE_LABELS: Readonly<Record<string, string>> = {sdr: 'SDR', hdr: 'HDR', hdr10: 'HDR10', 'hdr10+': 'HDR10+', hdr10plus: 'HDR10+', hlg: 'HLG', dolby_vision: 'Dolby Vision', 'dolby-vision': 'Dolby Vision', '4k': '4K', '8k': '8K', uhd: '4K', fhd: '1080p', hd: '720p', sd: 'SD'};
export function browseValueLabel(value: string): string {
  const known = VALUE_LABELS[value.toLowerCase()];
  if (known) return known;
  if (/^\d+p$/i.test(value)) return value.toLowerCase();
  if (/^\d{4}$/.test(value)) return value;
  const words = value.replace(/[-_]+/g, ' ').trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** The filter a genre's name opens on a title page: that genre, in the title's own library (Justin, 2 Oct 2026). */
export function genreFilter(name: string): ToolbarPredicate {
  return {field: 'genre', operator: 'contains-any', value: [name]};
}

/** The order a field's counted values are listed in: decades newest first (a timeline), everything else as the server counts them (most titles first). */
export function orderFilterValues<T extends Readonly<{value: string}>>(field: BrowseFieldCapability, values: readonly T[]): readonly T[] {
  return field.id === 'year' ? [...values].sort((a, b) => Number(b.value) - Number(a.value)) : values;
}
