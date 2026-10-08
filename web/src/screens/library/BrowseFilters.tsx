import React, {useEffect, useMemo, useState} from 'react';
import type {BrowseCapabilities, BrowseFacetValue, BrowseFieldCapability, BrowsePositionAnchor, BrowseSortSelection} from '@core/browse.ts';
import {formatBrowseAnchor} from '@core/browse.ts';
import {fieldLabel, predicateLabel, useFacets, type Predicate} from '../../app/browse';
import {useSession} from '../../app/session';
import {activeRangePreset, browseValueLabel, fieldSelectionSummary, filterControlOf, filterValuesField, nextToolbarSort, orderFilterValues, rangePresets, toolbarModel, type ToolbarGroupId, type ToolbarModel} from '@core/presentation/index.ts';
import {browseCountLabel, quickFilterLabelForKind} from '../../app/content';
import {Button, Checkbox, Chip, Chips, Icon, Input, Menu, Popover, Spinner, Text, cx, useCompact, type MenuItem} from '../../ui';
import {currentI18n} from '../../app/i18n';
import s from './BrowseFilters.module.css';

/**
 * The browse toolbar, drawn from the shared model (`toolbarModel` in client-core): one Sort
 * control, the library kind's promoted filters as their own menus, the play-state and favorite
 * quick toggles, "+ Filter" with everything else in three groups, and the applied filters as
 * removable chips on a second line. The same shape on every grid; Apple draws the same model.
 */
export function BrowseBar({libraryId, capabilities, predicates, onChange, sort, onSort, fixedSort, total, onSave, libraryKind, pivot}: {/** The tab orders itself (Recently aired): no Sort control. */ fixedSort?: boolean; libraryId: string; capabilities: BrowseCapabilities; predicates: readonly Predicate[]; onChange: (next: readonly Predicate[]) => void; sort: BrowseSortSelection; onSort: (next: BrowseSortSelection) => void; total?: number; onSave?: () => void; libraryKind: string; pivot?: string}) {
  const t = currentI18n().t;
  const compact = useCompact();
  const {owner} = useSession();
  const model = useMemo(() => toolbarModel(capabilities, libraryKind, predicates, sort, {owner}), [capabilities, libraryKind, predicates, sort, owner]);
  const same = (a: Predicate, b: Predicate) => a.field === b.field && a.operator === b.operator && JSON.stringify(a.value) === JSON.stringify(b.value);
  const quickNode = (id: string) => { const q = model.quick.find(f => f.id === id); return q && 'field' in q.query ? (q.query as Predicate) : undefined; };
  const isQuickOn = (id: string) => { const n = quickNode(id); return !!n && predicates.some(p => same(p, n)); };
  const toggleQuick = (id: string) => { const n = quickNode(id); if (!n) return; onChange(isQuickOn(id) ? predicates.filter(p => !same(p, n)) : [...predicates.filter(p => p.field !== n.field), n]); };
  // Year is chosen and stored by decade (the shared rule), so a field's predicates are those of either id.
  const owns = (field: BrowseFieldCapability, p: Predicate) => p.field === field.id || p.field === filterValuesField(field).field;
  const mineOf = (field: BrowseFieldCapability) => predicates.filter(p => owns(field, p));
  const setField = (field: BrowseFieldCapability, mine: readonly Predicate[]) => onChange([...predicates.filter(p => !owns(field, p)), ...mine]);
  const sortCap = capabilities.sorts.find(x => x.id === sort.field);
  const sortItems: MenuItem[] = [
    ...model.sorts.map(x => ({id: x.id, label: fieldLabel(x.id, x.labelKey), selected: x.id === sort.field, icon: x.id === sort.field ? (sort.direction === 'desc' ? 'chevronDown' as const : 'chevronUp' as const) : undefined, meta: x.expensive ? t('web.filters.slower') : undefined})),
  ];
  const sortLabel = `${fieldLabel(sort.field, sortCap?.labelKey)}${model.sort.flippable ? ` · ${directionLabel(sort.field, sort.direction)}` : ''}`;
  const [adding, setAdding] = useState(false);
  const [openField, setOpenField] = useState<string | null>(null);
  const defaultSort = capabilities.resolvedPivot?.defaultSort[0];
  const canSave = !!onSave && (predicates.length > 0 || sort.field !== (defaultSort?.field ?? 'title') || sort.direction !== (defaultSort?.direction ?? 'asc'));
  return (
    <div className={s.toolbar}>
      <div className={s.bar}>
        {fixedSort ? null : <Menu label={t('web.filters.sort')} align="start" trigger={<Chip label={sortLabel} icon="sort" aria-label={t('web.filters.sortBy')} />} items={sortItems} onSelect={id => onSort(nextToolbarSort(capabilities, sort, id))} />}
        {compact ? null : model.quick.map(f => <Chip key={f.id} label={quickFilterLabelForKind(f.labelKey, libraryKind)} pressed={isQuickOn(f.id)} onClick={() => toggleQuick(f.id)} />)}
        {compact ? null : model.promoted.map(f => (
          <FieldMenu key={f.id} libraryId={libraryId} field={f} predicates={mineOf(f)} open={openField === f.id} onOpenChange={o => setOpenField(o ? f.id : null)} onChange={mine => setField(f, mine)} />
        ))}
        <Popover open={adding} onOpenChange={setAdding} align="start" trigger={<Chip label={compact && predicates.length ? t('web.filters.buttonCount', {count: predicates.length}) : t('web.filters.add')} icon="plus" count={compact ? undefined : undefined} />}>
          <AddFilterPanel libraryId={libraryId} model={model} predicates={predicates} mineOf={mineOf} quickOn={isQuickOn} toggleQuick={toggleQuick} onChange={setField} onClear={() => { onChange([]); setAdding(false); }} onDone={() => setAdding(false)} compact={compact} libraryKind={libraryKind} />
        </Popover>
        <span className={s.spacer} />
        {typeof total === 'number' && !compact ? <span className={s.summary}>{browseCountLabel(total, pivot, libraryKind)}</span> : null}
        {onSave ? <Button variant="ghost" size="sm" icon="bookmark" label={compact ? undefined : t('web.filters.saveView')} aria-label={t('web.filters.saveView')} onClick={onSave} disabled={!canSave} /> : null}
      </div>
      {model.applied.length ? (
        <div className={s.applied}>
          {model.applied.map((p, i) => <Chip key={`${p.field}:${i}`} label={predicateLabel(p as Predicate, capabilities)} pressed icon="close" onClick={() => onChange(predicates.filter(x => x !== p))} />)}
          <Button variant="ghost" size="sm" label={t('web.filters.clearAll')} onClick={() => onChange([])} />
        </div>
      ) : null}
    </div>
  );
}

function directionLabel(field: string, direction: 'asc' | 'desc'): string {
  const t = currentI18n().t;
  const kind = /title|name|artist|album|author/.test(field) ? 'alpha' : /added|date|year|release|played|last/.test(field) ? 'date' : /rating/.test(field) ? 'rating' : /duration|length/.test(field) ? 'duration' : 'other';
  return t('web.filters.direction', {kind, dir: direction});
}

/** A promoted field on the bar: a chip that names the field and its current selection, opening its control. */
function FieldMenu({libraryId, field, predicates, open, onOpenChange, onChange}: {libraryId: string; field: BrowseFieldCapability; predicates: readonly Predicate[]; open: boolean; onOpenChange: (o: boolean) => void; onChange: (mine: readonly Predicate[]) => void}) {
  const t = currentI18n().t;
  const label = fieldLabel(field.id, field.labelKey);
  // The chip names the field and is lit while it filters; what it selects is said once, on the applied line below.
  return (
    <Popover open={open} onOpenChange={onOpenChange} align="start" trigger={<Chip label={label} pressed={predicates.length > 0} icon="chevronDown" />}>
      <div className={s.panel}>
        <div className={s.panelHead}><Text variant="bodyStrong">{label}</Text>{predicates.length ? <Button variant="ghost" size="sm" label={t('web.filters.clear')} onClick={() => onChange([])} /> : null}</div>
        <FieldControl libraryId={libraryId} field={field} predicates={predicates} onChange={onChange} />
        <div className={s.footer}><span /><Button variant="primary" size="sm" label={t('web.filters.done')} onClick={() => onOpenChange(false)} /></div>
      </div>
    </Popover>
  );
}

/**
 * "+ Filter": every field the server offers, in three groups (about the title, your activity,
 * about the file). A field opens its control in place; the group list stays, so adding a second
 * filter is one more click. On phones the quick toggles live here too.
 */
function AddFilterPanel({libraryId, model, predicates, mineOf, quickOn, toggleQuick, onChange, onClear, onDone, compact, libraryKind}: {libraryId: string; model: ToolbarModel; predicates: readonly Predicate[]; mineOf: (field: BrowseFieldCapability) => readonly Predicate[]; quickOn: (id: string) => boolean; toggleQuick: (id: string) => void; onChange: (field: BrowseFieldCapability, mine: readonly Predicate[]) => void; onClear: () => void; onDone: () => void; compact: boolean; libraryKind: string}) {
  const t = currentI18n().t;
  const [expanded, setExpanded] = useState<string | null>(null);
  const titles: Record<ToolbarGroupId, string> = {title: t('web.filters.group.title'), activity: t('web.filters.group.activity'), file: t('web.filters.group.file')};
  return (
    <div className={s.panel}>
      <div className={s.panelHead}><Text variant="bodyStrong">{t('web.filters.button')}</Text></div>
      {compact && model.quick.length ? <div className={s.group}><Text variant="label" tone="tertiary">{t('web.filters.quick')}</Text><Chips>{model.quick.map(f => <Chip key={f.id} label={quickFilterLabelForKind(f.labelKey, libraryKind)} pressed={quickOn(f.id)} onClick={() => toggleQuick(f.id)} />)}</Chips></div> : null}
      <div className={s.fields}>
        {model.groups.map(g => (
          <div key={g.id} className={s.group}>
            <Text variant="label" tone="tertiary">{titles[g.id]}</Text>
            {g.fields.map(f => {
              const mine = mineOf(f);
              const summary = fieldSelectionSummary(f, mine.map(p => ({...p, field: f.id})), v => (f.id === 'year' && mine.some(p => p.field === 'decade') ? `${String(v)}s` : valueLabel(String(v))));
              const open = expanded === f.id;
              return (
                <div key={f.id}>
                  <button type="button" className={s.groupHead} aria-expanded={open} onClick={() => setExpanded(open ? null : f.id)}>
                    <span className={s.fieldName}>{fieldLabel(f.id, f.labelKey)}</span>
                    <span className={s.fieldSummary}>{summary ?? ''}<Icon name={open ? 'chevronUp' : 'chevronDown'} size={14} /></span>
                  </button>
                  {open ? <FieldControl libraryId={libraryId} field={f} predicates={mine} onChange={next => onChange(f, next)} /> : null}
                </div>
              );
            })}
          </div>
        ))}
      </div>
      <div className={s.footer}>
        <Button variant="ghost" size="sm" label={t('web.filters.clearAll')} disabled={!predicates.length} onClick={onClear} />
        <Button variant="primary" size="sm" label={t('web.filters.done')} onClick={onDone} />
      </div>
    </div>
  );
}

/**
 * One field's control, as the shared rule classifies it (`filterControlOf`, client-core): the
 * library's own values with counts, Any / Yes / No, a short list of choices, or a range offered as
 * presets ("Under 90 min", "8 or higher", "In the last 30 days"); Year is chosen by decade. Never
 * two number boxes. Reused by the Library Channels builder (one condition's value editor).
 */
export function FieldControl({libraryId, field, predicates, onChange}: {libraryId: string; field: BrowseFieldCapability; predicates: readonly Predicate[]; onChange: (mine: readonly Predicate[]) => void}) {
  const t = currentI18n().t;
  const facets = useFacets(libraryId);
  const control = filterControlOf(field);
  const target = filterValuesField(field);
  const [values, setValues] = useState<readonly BrowseFacetValue[]>();
  const [q, setQ] = useState('');
  const [loading, setLoading] = useState(false);
  const counted = control === 'values' && (field.id === 'year' || !!field.facetSource || field.controlHint === 'facet-multi-select');
  useEffect(() => {
    if (!counted) return;
    let cancelled = false;
    setLoading(true);
    const timer = setTimeout(() => { facets(target.facet, q).then(v => { if (!cancelled) setValues(v); }, () => { if (!cancelled) setValues([]); }).finally(() => { if (!cancelled) setLoading(false); }); }, q ? 200 : 0);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [facets, counted, target.facet, q]);
  const name = fieldLabel(field.id, field.labelKey);
  /** One choice of several: a row that is the current one or not. */
  const choices = (items: readonly {id: string; label: string; selected: boolean; choose: () => void}[]) => (
    <div className={s.options} role="radiogroup" aria-label={name}>
      {items.map(item => <button key={item.id} type="button" role="radio" aria-checked={item.selected} className={s.choice} onClick={item.choose}><span>{item.label}</span>{item.selected ? <Icon name="check" size={14} /> : null}</button>)}
    </div>
  );
  if (control === 'values') {
    const chosen = new Set<string>(predicates.flatMap(p => (Array.isArray(p.value) ? p.value.map(String) : p.value === null ? [] : [String(p.value)])));
    const options: readonly BrowseFacetValue[] = orderFilterValues(field, values?.length ? values : (field.allowedValues ?? []).map(v => ({value: v, label: v, count: 0})));
    const numeric = field.id === 'year' || field.type === 'number';
    const set = (next: Set<string>) => onChange(next.size ? [{field: target.field, operator: target.operator, value: [...next].map(x => (numeric ? Number(x) : x))} as Predicate] : []);
    return (
      <>
        {(values?.length ?? 0) > 10 || q ? <Input label={t('web.filters.find', {field: name.toLowerCase()})} hideLabel placeholder={t('web.filters.findPlaceholder')} value={q} onChange={e => setQ(e.target.value)} /> : null}
        <div className={s.options}>
          {loading && !values ? <div className={s.loading}><Spinner /></div> : null}
          {options.map(o => (
            <div key={o.value} className={s.option}>
              <Checkbox checked={chosen.has(o.value)} onCheckedChange={on => { const next = new Set(chosen); if (on) next.add(o.value); else next.delete(o.value); set(next); }} label={<span className={s.optionLabel}><span>{o.label && o.label !== o.value ? o.label : valueLabel(o.value)}</span>{counted && o.count ? <span className={s.optionCount}>{o.count.toLocaleString()}</span> : null}</span>} />
            </div>
          ))}
          {values && !options.length && !loading ? <Text variant="caption" tone="tertiary">{t('web.filters.nothing')}</Text> : null}
        </div>
      </>
    );
  }
  if (control === 'toggle') {
    const current = predicates[0]?.value;
    return choices([
      {id: 'any', label: t('web.filters.any'), selected: current !== true && current !== false, choose: () => onChange([])},
      {id: 'yes', label: t('web.filters.yes'), selected: current === true, choose: () => onChange([{field: field.id, operator: 'equals', value: true}])},
      {id: 'no', label: t('web.filters.no'), selected: current === false, choose: () => onChange([{field: field.id, operator: 'equals', value: false}])},
    ]);
  }
  if (control === 'choice') {
    const current = String(predicates[0]?.value ?? '');
    return choices([
      {id: 'any', label: t('web.filters.any'), selected: !current, choose: () => onChange([])},
      ...(field.allowedValues ?? []).map(v => ({id: v, label: valueLabel(v), selected: current === v, choose: () => onChange([{field: field.id, operator: 'equals', value: v}])})),
    ]);
  }
  if (control === 'range') {
    const presets = rangePresets(field, new Date());
    const active = activeRangePreset(presets, predicates);
    return choices([
      {id: 'any', label: t('web.filters.any'), selected: !predicates.length, choose: () => onChange([])},
      ...presets.map(p => ({id: p.id, label: t(p.label.id, p.label.values), selected: active === p.id, choose: () => onChange(p.predicates as readonly Predicate[])})),
    ]);
  }
  const op = field.operators.includes('contains') ? 'contains' : 'equals';
  return <Input label={name} hideLabel placeholder={`${name}…`} defaultValue={String(predicates[0]?.value ?? '')} onBlur={e => onChange(e.target.value.trim() ? [{field: field.id, operator: op, value: e.target.value.trim()}] : [])} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }} />;
}

/** Position rail for a sorted grid (M20): each anchor is a server-computed position, so a
 * jump is one request. Labels read from the anchor key under the current sort ("2019",
 * "1990s", "Sep 2026", "8★", "1h 30m"); an empty key reads the catalogue "No value". */
export function AlphabetRail({index, current, onJump, sortField}: {index: readonly BrowsePositionAnchor[]; current?: string; onJump: (key: string) => void; /** Engine sort the anchors were computed under; labels format from it. */ sortField?: string}) {
  if (index.length < 4) return null;
  const noValue = currentI18n().t('library.anchorNoValue');
  const label = (key: string) => formatBrowseAnchor(key, sortField ?? 'title', noValue);
  return (
    <nav className={s.rail} aria-label={currentI18n().t('web.filters.jumpTo')}>
      {index.map(a => { const text = label(a.key); return <button key={a.key} type="button" className={cx(s.railKey)} aria-current={current === a.key ? 'true' : undefined} onClick={() => onJump(a.key)} title={currentI18n().t('web.filters.jumpToKey', {key: text})}>{text}</button>; })}
    </nav>
  );
}

/** CON-04: enum values read as people say them ("HDR10", "4K"); the rule is shared (client-core). */
export const valueLabel = browseValueLabel;
