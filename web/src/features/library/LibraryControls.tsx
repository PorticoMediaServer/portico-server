import { NavigationMoveDownIcon, NavigationMoveUpIcon, ActionConfirmIcon, NavigationExpandIcon, ActionAddIcon, ActionCustomizeIcon, ActionDeleteIcon, ActionCloseIcon } from '#portico-icons';
import { type FormEvent, type ReactNode, useEffect, useId, useMemo, useRef, useState } from 'react';
import {
  availableFields,
  availableSorts,
  compileFilter,
  countConditions,
  expressionToFilter,
  formatCapabilityLabel,
  serializeSavedViewDraft,
  productMessage,
  type BrowseFacetOption,
  type FilterConditionNode,
  type FilterGroupNode,
  type FilterNode,
  type LibraryBrowseCapabilities,
  type SavedView,
} from '@porticomediaserver/client-core';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { AnchoredOverlay, ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productLanguageProblem } from '../../components/states/ProductLanguageState';
import { secureRandomUUID } from '../../runtime/secureRandomUUID';
import type {
  BrowseExpression,
  BrowseSort,
  LibraryFieldCapability,
  LibraryPivotCapability,
  LibrarySortCapability,
  LibraryWorkspaceLibrary,
  LibraryWorkspaceSource,
} from './libraryTypes';

type ChoiceOption = {
  id: string;
  label: string;
  detail?: string;
  disabled?: boolean;
  trailing?: ReactNode;
};

export function CapabilityMenu({
  label,
  value,
  options,
  onChange,
  compact = false,
  selectedIndicator,
}: {
  label: string;
  value: string;
  options: ChoiceOption[];
  onChange: (id: string) => void;
  compact?: boolean;
  selectedIndicator?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const identifier = useId();
  const triggerId = `${identifier}-trigger`;
  const listboxId = `${identifier}-listbox`;
  const selected = options.find((option) => option.id === value);
  return <div className={`library-choice ${compact ? 'compact' : ''}`}>
    <button
      id={triggerId}
      ref={triggerRef}
      type="button"
      className="library-choice-trigger"
      aria-haspopup="listbox"
      aria-expanded={open}
      aria-controls={open ? listboxId : undefined}
      onClick={() => setOpen((current) => !current)}
    >
      <span><span className="library-choice-label">{label}</span><strong>{selected?.label ?? formatCapabilityLabel(value)}</strong></span>
      {selectedIndicator && <span className="library-choice-selected-indicator">{selectedIndicator}</span>}
      <NavigationExpandIcon />
    </button>
    {open && <AnchoredOverlay
      id={listboxId}
      anchorRef={triggerRef}
      returnFocusRef={triggerRef}
      className="library-choice-popover"
      minAnchorWidth
      role="listbox"
      labelledBy={triggerId}
      onDismiss={() => setOpen(false)}
    >
      {options.map((option) => <button
        key={option.id}
        type="button"
        role="option"
        aria-selected={value === option.id}
        disabled={option.disabled}
        className={value === option.id ? 'chosen' : ''}
        onClick={() => {
          onChange(option.id);
          setOpen(false);
          triggerRef.current?.focus();
        }}
      >
        <span><strong>{option.label}</strong>{option.detail && <span>{option.detail}</span>}</span>
        {option.trailing ?? (value === option.id && <ActionConfirmIcon />)}
      </button>)}
    </AnchoredOverlay>}
  </div>;
}

function directionIcon(direction: BrowseSort['direction']) {
  return direction === 'asc' ? <NavigationMoveUpIcon /> : <NavigationMoveDownIcon />;
}

export function SortCapabilityMenu({
  value,
  options,
  onChange,
}: {
  value: BrowseSort;
  options: LibrarySortCapability[];
  onChange: (sort: BrowseSort) => void;
}) {
  const selected = options.find((option) => option.id === value.field);
  const toggleDirection = () => {
    if (!selected) return value.direction;
    const alternate = selected.directions.find((direction) => direction !== value.direction);
    return alternate ?? value.direction;
  };
  return <CapabilityMenu
    label={productMessage('library.control-sort').text ?? ''}
    value={value.field}
    selectedIndicator={directionIcon(value.direction)}
    options={options.map((option) => {
      const chosen = option.id === value.field;
      return {
        id: option.id,
        label: option.label,
        detail: option.expensive ? productMessage('library.sort-expensive').text : undefined,
        trailing: chosen
          ? <span className="library-sort-menu-state">{directionIcon(value.direction)}<ActionConfirmIcon /></span>
          : undefined,
      };
    })}
    onChange={(field) => {
      const capability = options.find((option) => option.id === field);
      if (!capability) return;
      onChange(field === value.field
        ? { ...value, direction: toggleDirection() }
        : { field, direction: capability.defaultDirection });
    }}
  />;
}

function replaceNode(root: FilterGroupNode, id: string, update: (node: FilterNode) => FilterNode): FilterGroupNode {
  const visit = (node: FilterNode): FilterNode => {
    const next = node.id === id ? update(node) : node;
    return next.kind === 'group' ? { ...next, children: next.children.map(visit) } : next;
  };
  return visit(root) as FilterGroupNode;
}

function removeNode(root: FilterGroupNode, id: string): FilterGroupNode {
  const visit = (node: FilterNode): FilterNode => node.kind === 'group'
    ? { ...node, children: node.children.filter((child) => child.id !== id).map(visit) }
    : node;
  return visit(root) as FilterGroupNode;
}

function isListOperator(operator: string) {
  return ['in', 'not-in', 'contains-any', 'contains-all', 'between'].includes(operator);
}

function hasIncompleteChoiceCondition(node: FilterNode, fields: LibraryFieldCapability[]): boolean {
  if (node.kind === 'group') return node.children.some((child) => hasIncompleteChoiceCondition(child, fields));
  if (['is-present', 'is-missing'].includes(node.operator)) return false;
  const field = fields.find((candidate) => candidate.id === node.field);
  if (!field || !(field.controlHint === 'select' || field.controlHint === 'facet-multi-select' || field.allowedValues?.length || field.facetSource)) return false;
  return isListOperator(node.operator) ? !(node.rawValues?.length || node.rawValue.trim()) : !node.rawValue.trim();
}

function firstCondition(fields: LibraryFieldCapability[]): FilterConditionNode | undefined {
  const field = fields.find((candidate) => candidate.operators.length > 0);
  return field ? {
    id: `condition-${secureRandomUUID()}`,
    kind: 'condition',
    field: field.id,
    operator: field.operators[0],
    rawValue: field.valueType === 'boolean' ? 'true' : '',
    negated: false,
  } : undefined;
}

function FilterChoiceEditor({ node, field, libraryId, source, onChange }: {
  node: FilterConditionNode;
  field: LibraryFieldCapability;
  libraryId: string;
  source: LibraryWorkspaceSource;
  onChange: (value: { rawValue: string; rawValues?: string[] }) => void;
}) {
  const [facetOptions, setFacetOptions] = useState<BrowseFacetOption[]>([]);
  const [facetState, setFacetState] = useState<'idle' | 'loading' | 'success' | 'error'>('idle');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    if (!field.facetSource) {
      setFacetOptions([]);
      setFacetState('idle');
      return;
    }
    const controller = new AbortController();
    setFacetState('loading');
    source.libraryFacetOptions(libraryId, field.facetSource, controller.signal).then(
      (options) => { if (!controller.signal.aborted) { setFacetOptions(options); setFacetState('success'); } },
      () => { if (!controller.signal.aborted) setFacetState('error'); },
    );
    return () => controller.abort();
  }, [field.facetSource, field.id, libraryId, revision, source]);
  const options: BrowseFacetOption[] = field.facetSource
    ? facetOptions
    : (field.allowedValues ?? []).map((value) => ({ value, label: formatCapabilityLabel(value) }));
  const multiple = field.controlHint === 'facet-multi-select' || ['in', 'not-in', 'contains-any', 'contains-all'].includes(node.operator);
  const selected = node.rawValues ?? (node.rawValue ? [node.rawValue] : []);
  if (facetState === 'loading') return <div className="library-filter-choice-state" role="status">{productMessage('library.facet-loading').title}</div>;
  if (facetState === 'error') return <div className="library-filter-choice-state error" role="alert">{productMessage('library.facet-unavailable').title}<button type="button" onClick={() => setRevision((value) => value + 1)}>{productMessage('library.facet-unavailable').actions[0]?.label}</button></div>;
  if (!options.length) return <div className="library-filter-choice-state" role="status">{productMessage('library.facet-empty').title}</div>;
  if (!multiple) return <CapabilityMenu label={productMessage('library.control-value').text ?? ''} value={node.rawValue} options={[
    { id: '', label: productMessage('library.select-value').text ?? '', disabled: true },
    ...options.map((option) => ({ id: option.value, label: option.label, detail: option.count == null ? undefined : productMessage(option.count === 1 ? 'media.item-count-single' : 'media.item-count', { count: option.count }).text })),
  ]} onChange={(rawValue) => onChange({ rawValue, rawValues: undefined })} />;
  return <fieldset className="library-filter-value-options">
    <legend>{productMessage('library.control-values').text}</legend>
    {options.map((option) => <label key={option.value}><input type="checkbox" checked={selected.includes(option.value)} onChange={(event) => {
      const next = event.target.checked ? [...selected, option.value] : selected.filter((value) => value !== option.value);
      onChange({ rawValue: '', rawValues: next });
    }} /><span>{option.label}{option.count == null ? null : <small>{option.count}</small>}</span></label>)}
  </fieldset>;
}

function FilterConditionEditor({
  node,
  fields,
  libraryId,
  source,
  onChange,
  onRemove,
}: {
  node: FilterConditionNode;
  fields: LibraryFieldCapability[];
  libraryId: string;
  source: LibraryWorkspaceSource;
  onChange: (node: FilterConditionNode) => void;
  onRemove: () => void;
}) {
  const field = fields.find((candidate) => candidate.id === node.field) ?? fields[0];
  if (!field) return null;
  const operators = field.operators.map((operator) => ({ id: operator, label: formatCapabilityLabel(operator) }));
  const valueNotRequired = ['is-present', 'is-missing'].includes(node.operator);
  const valueLabel = ['in', 'not-in', 'contains-any', 'contains-all', 'between'].includes(node.operator)
    ? productMessage('library.filter-values-separated').text
    : field.valueType === 'date' ? productMessage('library.filter-date-value').text : productMessage('library.control-value').text;
  return <div className="library-filter-condition">
    <CapabilityMenu
      label={productMessage('library.control-field').text ?? ''}
      value={field.id}
      options={fields.map((candidate) => ({ id: candidate.id, label: candidate.label, detail: formatCapabilityLabel(candidate.valueType) }))}
      onChange={(fieldId) => {
        const next = fields.find((candidate) => candidate.id === fieldId);
        if (next) onChange({ ...node, field: next.id, operator: next.operators[0] ?? '', rawValue: next.valueType === 'boolean' ? 'true' : '', rawValues: undefined });
      }}
    />
    <CapabilityMenu label={productMessage('library.control-condition').text ?? ''} value={node.operator} options={operators} onChange={(operator) => {
      const nextValues = isListOperator(operator) ? (node.rawValues ?? (node.rawValue ? [node.rawValue] : [])) : undefined;
      onChange({ ...node, operator, rawValue: nextValues ? '' : (node.rawValues?.[0] ?? node.rawValue), rawValues: nextValues });
    }} />
    {!valueNotRequired && (field.valueType === 'boolean'
      ? <CapabilityMenu
          label={productMessage('library.control-value').text ?? ''}
          value={node.rawValue || 'true'}
          options={[{ id: 'true', label: productMessage('value.yes').text ?? '' }, { id: 'false', label: productMessage('value.no').text ?? '' }]}
          onChange={(rawValue) => onChange({ ...node, rawValue, rawValues: undefined })}
        />
      : field.controlHint === 'select' || field.controlHint === 'facet-multi-select' || Boolean(field.allowedValues?.length) || Boolean(field.facetSource)
        ? <FilterChoiceEditor node={node} field={field} libraryId={libraryId} source={source} onChange={(value) => onChange({ ...node, ...value })} />
      : <label className="library-filter-value">
          <span>{valueLabel}</span>
          <input
            value={node.rawValue}
            inputMode={['number', 'duration', 'date-number'].includes(field.valueType) ? 'decimal' : undefined}
            placeholder={field.valueType === 'date' ? productMessage('library.filter-date-example').text : undefined}
            onChange={(event) => onChange({ ...node, rawValue: event.target.value, rawValues: undefined })}
          />
        </label>)}
    <label className="library-negate-choice"><input type="checkbox" checked={node.negated} onChange={(event) => onChange({ ...node, negated: event.target.checked })} /><span>{productMessage('library.filter-exclude').text}</span></label>
    <IconButton label={productMessage('library.filter-remove-condition').text ?? ''} onClick={onRemove}><ActionDeleteIcon /></IconButton>
  </div>;
}

function FilterGroupEditor({
  node,
  fields,
  libraryId,
  source,
  depth,
  maximumDepth,
  maximumClauses,
  totalClauses,
  onChange,
  onRemove,
  root = false,
}: {
  node: FilterGroupNode;
  fields: LibraryFieldCapability[];
  libraryId: string;
  source: LibraryWorkspaceSource;
  depth: number;
  maximumDepth: number;
  maximumClauses: number;
  totalClauses: number;
  onChange: (node: FilterGroupNode) => void;
  onRemove?: () => void;
  root?: boolean;
}) {
  const canAddCondition = totalClauses < maximumClauses && fields.length > 0;
  const canAddGroup = canAddCondition && depth < maximumDepth;
  const addCondition = () => {
    const condition = firstCondition(fields);
    if (condition) onChange({ ...node, children: [...node.children, condition] });
  };
  const addGroup = () => {
    const condition = firstCondition(fields);
    if (!condition) return;
    onChange({
      ...node,
      children: [...node.children, {
        id: `group-${secureRandomUUID()}`,
        kind: 'group',
        mode: 'all',
        negated: false,
        children: [condition],
      }],
    });
  };
  return <section className={`library-filter-group ${root ? 'root' : ''}`}>
    <div className="library-filter-group-head">
      <span>{productMessage('library.filter-match').text}</span>
      <div className="library-logic-switch" aria-label={productMessage('library.filter-group-logic').text}>
        <button type="button" className={node.mode === 'all' ? 'selected' : ''} onClick={() => onChange({ ...node, mode: 'all' })}>{productMessage('library.filter-match-all').text}</button>
        <button type="button" className={node.mode === 'any' ? 'selected' : ''} onClick={() => onChange({ ...node, mode: 'any' })}>{productMessage('library.filter-match-any').text}</button>
      </div>
      <span>{productMessage('library.filter-conditions-suffix').text}</span>
      <label className="library-negate-choice"><input type="checkbox" checked={node.negated} onChange={(event) => onChange({ ...node, negated: event.target.checked })} /><span>{productMessage('library.filter-exclude-group').text}</span></label>
      {!root && onRemove && <IconButton label={productMessage('library.filter-remove-group').text ?? ''} onClick={onRemove}><ActionDeleteIcon /></IconButton>}
    </div>
    <div className="library-filter-children">
      {node.children.map((child) => child.kind === 'condition'
        ? <FilterConditionEditor
            key={child.id}
            node={child}
            fields={fields}
            libraryId={libraryId}
            source={source}
            onChange={(next) => onChange(replaceNode(node, child.id, () => next))}
            onRemove={() => onChange(removeNode(node, child.id))}
          />
        : <FilterGroupEditor
            key={child.id}
            node={child}
            fields={fields}
            libraryId={libraryId}
            source={source}
            depth={depth + 1}
            maximumDepth={maximumDepth}
            maximumClauses={maximumClauses}
            totalClauses={totalClauses}
            onChange={(next) => onChange(replaceNode(node, child.id, () => next))}
            onRemove={() => onChange(removeNode(node, child.id))}
          />)}
      {!node.children.length && <p className="library-filter-empty">{productMessage('library.conditions-empty').title}</p>}
    </div>
    <div className="library-filter-add">
      <button type="button" disabled={!canAddCondition} onClick={addCondition}><ActionAddIcon /> {productMessage('action.add-condition').text}</button>
      <button type="button" disabled={!canAddGroup} onClick={addGroup}><ActionAddIcon /> {productMessage('action.add-group').text}</button>
    </div>
  </section>;
}

function SortEditor({
  sorts,
  capabilities,
  onChange,
}: {
  sorts: BrowseSort[];
  capabilities: LibrarySortCapability[];
  onChange: (sorts: BrowseSort[]) => void;
}) {
  const unused = capabilities.filter((capability) => !sorts.some((sort) => sort.field === capability.id));
  return <section className="library-sort-editor">
    <div className="library-builder-section-title"><h3>{productMessage('library.sort-order-title').text}</h3><p>{productMessage('library.sort-order-body').text}</p></div>
    <div className="library-sort-rows">
      {sorts.map((sort, index) => {
        const capability = capabilities.find((candidate) => candidate.id === sort.field);
        if (!capability) return null;
        return <div className="library-sort-row" key={`${sort.field}-${index}`}>
          <span className="library-sort-priority">{index + 1}</span>
          <SortCapabilityMenu
            value={sort}
            options={capabilities}
            onChange={(nextSort) => onChange(sorts.map((candidate, candidateIndex) => candidateIndex === index
              ? nextSort
              : candidate).filter((candidate, candidateIndex, all) => all.findIndex((match) => match.field === candidate.field) === candidateIndex))}
          />
          <IconButton label={productMessage('library.sort-remove', { sort: capability.label }).text ?? ''} disabled={sorts.length === 1} onClick={() => onChange(sorts.filter((_, candidateIndex) => candidateIndex !== index))}><ActionDeleteIcon /></IconButton>
        </div>;
      })}
    </div>
    <button
      type="button"
      className="library-add-sort"
      disabled={!unused.length}
      onClick={() => {
        const capability = unused[0];
        if (capability) onChange([...sorts, { field: capability.id, direction: capability.defaultDirection }]);
      }}
    ><ActionAddIcon /> {productMessage('action.add-sort').text}</button>
  </section>;
}

export function AdvancedLibraryDialog({
  capabilities,
  library,
  source,
  pivot,
  expression,
  sorts,
  onApply,
  onDismiss,
}: {
  capabilities: LibraryBrowseCapabilities;
  library: LibraryWorkspaceLibrary;
  source: LibraryWorkspaceSource;
  pivot: LibraryPivotCapability;
  expression?: BrowseExpression;
  sorts: BrowseSort[];
  onApply: (expression: BrowseExpression | undefined, sorts: BrowseSort[]) => void;
  onDismiss: () => void;
}) {
  const fields = useMemo(() => availableFields(capabilities, pivot), [capabilities, pivot]);
  const sortCapabilities = useMemo(() => availableSorts(capabilities, pivot), [capabilities, pivot]);
  const [root, setRoot] = useState(() => expressionToFilter(expression, fields));
  const [draftSorts, setDraftSorts] = useState(sorts);
  const conditionCount = countConditions(root);
  const hasIncompleteChoice = hasIncompleteChoiceCondition(root, fields);
  const apply = () => {
    onApply(compileFilter(root, fields), draftSorts);
    onDismiss();
  };
  return <ModalOverlay className="library-filter-sheet" labelledBy="library-filter-title" onDismiss={onDismiss}>
    <header>
      <div><h2 id="library-filter-title">{productMessage('library.refine-title', { pivot: pivot.label }).text}</h2><p>{productMessage('library.condition-budget', { count: conditionCount, maximum: capabilities.queryLimits.maximumClauses }).text}</p></div>
      <IconButton label={productMessage('action.dismiss').text ?? ''} onClick={onDismiss}><ActionCloseIcon /></IconButton>
    </header>
    <div className="library-filter-sheet-body">
      <div className="library-builder-section-title"><h3>{productMessage('library.filters-title').text}</h3><p>{productMessage('library.filters-body').text}</p></div>
      <FilterGroupEditor
        root
        node={root}
        fields={fields}
        libraryId={library.id}
        source={source}
        depth={1}
        maximumDepth={capabilities.queryLimits.maximumDepth}
        maximumClauses={capabilities.queryLimits.maximumClauses}
        totalClauses={conditionCount}
        onChange={setRoot}
      />
      <SortEditor sorts={draftSorts} capabilities={sortCapabilities} onChange={setDraftSorts} />
    </div>
    <footer>
      <button type="button" className="library-clear-builder" onClick={() => setRoot(expressionToFilter(undefined, fields))}>{productMessage('action.clear-filters').text}</button>
      <SecondaryButton onClick={onDismiss}>{productMessage('action.cancel').text}</SecondaryButton>
      <PrimaryButton onClick={apply} disabled={hasIncompleteChoice}>{productMessage('action.apply').text}</PrimaryButton>
    </footer>
  </ModalOverlay>;
}

export function SaveLibraryViewDialog({
  library,
  source,
  pivot,
  expression,
  sorts,
  presentationFields,
  onSaved,
  onDismiss,
}: {
  library: LibraryWorkspaceLibrary;
  source: LibraryWorkspaceSource;
  pivot: LibraryPivotCapability;
  expression?: BrowseExpression;
  sorts: BrowseSort[];
  presentationFields: string[];
  onSaved: (view: SavedView) => void;
  onDismiss: () => void;
}) {
  const [title, setTitle] = useState('');
  const [pinned, setPinned] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!title.trim() || busy) return;
    setBusy(true);
    setError('');
    const controller = new AbortController();
    try {
      const view = await source.createSavedView(serializeSavedViewDraft({
        title: title.trim(), libraryId: library.id, pivot: pivot.id, query: expression,
        sort: sorts, presentationFields, isPinned: pinned,
      }), controller.signal);
      onSaved(view);
      onDismiss();
    } catch (reason) {
      const failure = productLanguageProblem(reason);
      setError(failure.body ?? failure.title ?? productMessage('library.save-failed').body ?? productMessage('library.save-failed').title ?? '');
    } finally {
      setBusy(false);
    }
  };
  return <ModalOverlay className="library-save-dialog" labelledBy="library-save-title" onDismiss={onDismiss}>
    <form onSubmit={(event) => void submit(event)}>
      <header><div><h2 id="library-save-title">{productMessage('library.save-title').text}</h2><p>{library.name} · {pivot.label}</p></div><IconButton label={productMessage('library.save-close').text ?? ''} onClick={onDismiss}><ActionCloseIcon /></IconButton></header>
      <div className="library-save-fields">
        <label><span>{productMessage('library.save-name').text}</span><input autoFocus value={title} onChange={(event) => setTitle(event.target.value)} placeholder={productMessage('library.save-name-example').text} /></label>
        <label className="library-save-pin"><input type="checkbox" checked={pinned} onChange={(event) => setPinned(event.target.checked)} /><span>{productMessage('library.save-pin').text}</span></label>
        {error && <p className="library-dialog-error" role="alert">{error}</p>}
      </div>
      <footer><SecondaryButton onClick={onDismiss}>{productMessage('action.cancel').text}</SecondaryButton><PrimaryButton type="submit" disabled={!title.trim() || busy}><ActionConfirmIcon /> {busy ? productMessage('state.saving').text : productMessage('action.save-view').text}</PrimaryButton></footer>
    </form>
  </ModalOverlay>;
}

export function AdvancedButton({ count, onClick }: { count: number; onClick: () => void }) {
  return <SecondaryButton onClick={onClick}><ActionCustomizeIcon /> {productMessage(count ? 'library.more-filters-count' : 'library.more-filters', { count }).text}</SecondaryButton>;
}

export function LibraryControlGroup({ children }: { children: ReactNode }) {
  return <div className="library-control-group">{children}</div>;
}
