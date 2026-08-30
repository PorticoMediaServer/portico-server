import { ApiError, type SettingsDocument, type SettingsGroups, type SettingsGroupsUpdate, type SettingsGroupSummary, type SettingsSummaryResponse } from '@porticomediaserver/client-core';
import { StatusLockedIcon, ActionResetIcon } from '#portico-icons';
import { Fragment, useEffect, useMemo, useRef, useState } from 'react';
import { SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { secureRandomUUID } from '../../runtime/secureRandomUUID';
import {
  ChoiceControl,
  InlineNotice,
  NumberControl,
  ReadOnlyValue,
  SaveBar,
  SecretControl,
  type SecretChange,
  SettingRow,
  SettingsGroup,
  StringListControl,
  TextControl,
  ToggleControl,
} from './SettingsControls';
import { serverSettingFieldGroups, type SettingsFieldDefinition, type SettingsFieldGroup, type WritableSettingsGroup } from './settingsFields';
import { useAbortableMutation } from './settingsHooks';
import type { SettingsDataSource, SettingsViewer } from './settingsTypes';

type DraftValue = boolean | number | string | string[] | SecretChange;
type DraftGroup = Record<string, DraftValue>;
type SettingsDraft = Partial<Record<WritableSettingsGroup, DraftGroup>>;

function settingsIdempotencyKey(): string {
  return `settings-${secureRandomUUID()}`;
}

function groupRecord(groups: SettingsGroups, key: WritableSettingsGroup): Record<string, unknown> {
  const value = groups[key];
  return value && typeof value === 'object' ? value as Record<string, unknown> : {};
}

function hasOwn(record: Record<string, unknown> | undefined, key: string): boolean {
  return Boolean(record && Object.prototype.hasOwnProperty.call(record, key));
}

function capabilityFor(group: SettingsFieldGroup, summary: SettingsSummaryResponse): SettingsGroupSummary | undefined {
  return summary.groups.find((candidate) => candidate.id === group.capabilityId);
}

function fieldValue(document: SettingsDocument, draft: SettingsDraft, group: SettingsFieldGroup, field: SettingsFieldDefinition): unknown {
  const pending = draft[group.settingsKey];
  if (hasOwn(pending, field.field)) return pending?.[field.field];
  return groupRecord(document.groups, group.settingsKey)[field.field] ?? field.defaultValue;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function numberValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function booleanValue(value: unknown): boolean {
  return value === true;
}

function stringListValue(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === 'string') : [];
}

function secretPresent(value: unknown): boolean {
  if (!value || typeof value !== 'object') return false;
  return (value as { present?: unknown }).present === true;
}

type ResolvedFieldState = {
  requestedValue: unknown;
  effectiveValue?: unknown;
  effectiveSource?: string;
  policyLimit?: unknown;
  restrictionReason?: string;
};

function resolvedFieldState(value: unknown): ResolvedFieldState | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const candidate = value as Record<string, unknown>;
  if (!Object.prototype.hasOwnProperty.call(candidate, 'requestedValue')) return undefined;
  return {
    requestedValue: candidate.requestedValue,
    effectiveValue: candidate.effectiveValue,
    effectiveSource: typeof candidate.effectiveSource === 'string' ? candidate.effectiveSource : undefined,
    policyLimit: candidate.policyLimit,
    restrictionReason: typeof candidate.restrictionReason === 'string' ? candidate.restrictionReason : undefined,
  };
}

function displayResolvedValue(value: unknown): string {
  if (value === undefined) return 'Unavailable';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (Array.isArray(value)) return value.map((entry) => String(entry)).join(', ');
  return 'Reported by server';
}

function sanitizeDraft(draft: SettingsDraft): SettingsGroupsUpdate {
  const groups: Record<string, Record<string, unknown>> = {};
  for (const [groupKey, fields] of Object.entries(draft)) {
    const next: Record<string, unknown> = {};
    for (const [field, value] of Object.entries(fields ?? {})) {
      if (value !== undefined) next[field] = value;
    }
    if (Object.keys(next).length > 0) groups[groupKey] = next;
  }
  return groups as SettingsGroupsUpdate;
}

function validateNumericDraft(draft: SettingsDraft, groups: SettingsFieldGroup[]): Map<string, string> {
  const errors = new Map<string, string>();
  for (const group of groups) {
    const pending = draft[group.settingsKey];
    if (!pending) continue;
    for (const field of group.fields) {
      if (field.kind !== 'number' || !hasOwn(pending, field.field)) continue;
      const key = `${group.settingsKey}.${field.field}`;
      const value = pending[field.field];
      if (typeof value !== 'number' || !Number.isFinite(value)) {
        errors.set(key, `${field.label} requires a number.`);
        continue;
      }
      if (field.min !== undefined && value < field.min) {
        errors.set(key, `${field.label} must be at least ${field.min}.`);
        continue;
      }
      if (field.max !== undefined && value > field.max) {
        errors.set(key, `${field.label} must be no more than ${field.max}.`);
        continue;
      }
      const step = field.step ?? 1;
      if (step > 0) {
        const origin = field.min ?? 0;
        const steps = (value - origin) / step;
        if (Math.abs(steps - Math.round(steps)) > 1e-9) errors.set(key, `${field.label} must use increments of ${step}.`);
      }
    }
  }
  return errors;
}

function changedFieldCount(draft: SettingsDraft): number {
  return Object.values(draft).reduce((total, group) => total + Object.keys(group ?? {}).length, 0);
}

function capabilityStatus(capability: SettingsGroupSummary | undefined): string {
  return capability?.status?.trim().toLocaleLowerCase() ?? 'unknown';
}

function capabilityCannotEdit(capability: SettingsGroupSummary | undefined): boolean {
  if (!capability) return false;
  if (capability.readOnly || capability.implemented === false) return true;
  // The server remains the authority for this state. A disabled control is
  // explanatory UI only; it never replaces the server-side permission check.
  return ['unsupported', 'unavailable', 'denied', 'offline', 'recovery-only', 'busy'].includes(capabilityStatus(capability));
}

function capabilityStateLabel(capability: SettingsGroupSummary | undefined): string | undefined {
  const status = capabilityStatus(capability);
  if (status === 'unknown' || status === 'ready' || status === 'available' || status === 'configured' || status === 'healthy') return undefined;
  if (status === 'unsupported') return 'Unsupported by this server';
  if (status === 'unavailable') return 'Unavailable: a required server facility is not available';
  if (status === 'denied') return 'Your account is not permitted to change this group';
  if (status === 'offline') return 'The owning server is offline';
  if (status === 'recovery-only') return 'Recovery mode: normal settings changes are fenced';
  if (status === 'busy') return 'Busy: conflicting work must finish before this can change';
  if (status === 'invalid') return 'Saved value needs attention';
  if (status === 'degraded') return 'Available with a reported impairment';
  return 'The server reported a state that needs attention';
}

function isGroupVisible(group: SettingsFieldGroup, document: SettingsDocument, summary: SettingsSummaryResponse): boolean {
  return Boolean(document.groups[group.settingsKey] || capabilityFor(group, summary));
}

function visibilityMatches(condition: SettingsFieldDefinition['visibleWhen'], document: SettingsDocument, draft: SettingsDraft): boolean {
  if (!condition) return true;
  const pending = draft[condition.settingsKey];
  const value = hasOwn(pending, condition.field)
    ? pending?.[condition.field]
    : groupRecord(document.groups, condition.settingsKey)[condition.field];
  return value === condition.equals;
}

export function ServerSettingsForm({ section, document, summary, viewer, source, onDocumentChange, onReload }: {
  section: string;
  document: SettingsDocument;
  summary: SettingsSummaryResponse;
  viewer: SettingsViewer;
  source: SettingsDataSource;
  onDocumentChange: (document: SettingsDocument) => void;
  onReload: () => void;
}) {
  const [draft, setDraft] = useState<SettingsDraft>({});
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const [fieldErrors, setFieldErrors] = useState<Map<string, string>>(() => new Map());
  const intentKey = useRef<string | undefined>(undefined);
  const { busy, run } = useAbortableMutation();
  // Drafts belong to one authority and section. An authoritative refresh may
  // advance the revision while a user is editing; changed fields are patches
  // and must remain visible so a conflict can be re-applied deliberately.
  const scopeKey = `${viewer.authOrigin ?? 'unknown'}:${viewer.id}:${viewer.serverId ?? viewer.serverName}:${section}`;
  const groups = useMemo(() => (serverSettingFieldGroups[section] ?? []).filter((group) => isGroupVisible(group, document, summary) && visibilityMatches(group.visibleWhen, document, draft)), [document, draft, section, summary]);
  const dirty = changedFieldCount(draft) > 0;

  useEffect(() => {
    intentKey.current = undefined;
    setDraft({});
    setFeedback('');
    setError('');
    setFieldErrors(new Map());
  }, [scopeKey]);

  const update = (group: WritableSettingsGroup, field: string, value: DraftValue) => {
    // Editing after a submitted/unknown-outcome attempt creates a new body.
    // Keep the prior key only while retrying that unchanged body.
    intentKey.current = settingsIdempotencyKey();
    setDraft((current) => ({ ...current, [group]: { ...(current[group] ?? {}), [field]: value } }));
    setFieldErrors((current) => {
      if (!current.has(`${group}.${field}`)) return current;
      const next = new Map(current);
      next.delete(`${group}.${field}`);
      return next;
    });
    setFeedback('');
    setError('');
  };

  const save = async () => {
    const fieldCount = changedFieldCount(draft);
    if (!fieldCount) return;
    setError('');
    setFeedback('');
    const validation = validateNumericDraft(draft, groups);
    if (validation.size > 0) {
      // This body was not submitted. A corrected draft is a new intent.
      intentKey.current = undefined;
      setFieldErrors(validation);
      setError('Fix the highlighted settings before saving.');
      const first = validation.keys().next().value as string | undefined;
      if (first) window.requestAnimationFrame(() => globalThis.document.querySelector<HTMLElement>(`[data-settings-field="${first}"]`)?.focus());
      return;
    }
    setFieldErrors(new Map());
    try {
      const idempotencyKey = intentKey.current ?? settingsIdempotencyKey();
      intentKey.current = idempotencyKey;
      const updated = await run((signal) => source.updateSettings({ expectedRevision: document.revision, idempotencyKey, groups: sanitizeDraft(draft) }, signal));
      onDocumentChange(updated);
      setDraft({});
      intentKey.current = undefined;
      setFeedback(`${fieldCount} ${fieldCount === 1 ? 'setting' : 'settings'} saved.`);
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 409) {
        // A conflict is a known non-commit. Reload/reapply targets a new
        // revision and must receive a new idempotency key.
        intentKey.current = undefined;
        setError('These settings changed in another session. Reload before saving again.');
      } else {
        setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'save these settings' }));
      }
    }
  };

  if (groups.length === 0) return <div className="portico-settings-state"><StatusLockedIcon /><strong>No settings are available here</strong><p>Your account or this server does not expose any configurable settings in this section.</p></div>;

  const changedFields = document.applyImpact?.changedFields ?? [];
  const restartFields = document.applyImpact?.restartRequiredFields ?? document.restartRequiredFields;
  const generation = document.generation;
  return <div className="portico-settings-form">
    <div className="portico-settings-scope-bar" aria-label="Settings authority and revision">
      <span><strong>This Server</strong><small>Server owner settings · authoritative saved revision <code>{document.revision}</code>{generation ? ` · active revision ${generation.activeRevision}` : ''}{generation?.pendingRevision ? ` · pending revision ${generation.pendingRevision}` : ''}</small></span>
      <span className={dirty ? 'draft' : 'saved'}>{dirty ? `${changedFieldCount(draft)} draft ${changedFieldCount(draft) === 1 ? 'change' : 'changes'}` : 'Draft matches saved state'}</span>
    </div>
    {document.restartRequired && <InlineNotice tone="warn">A server restart is required before the most recent change takes effect.{restartFields.length > 0 && ` Affected fields: ${restartFields.join(', ')}.`}</InlineNotice>}
    {changedFields.length > 0 && !dirty && <InlineNotice tone="success">Saved revision <code>{document.revision}</code> committed {changedFields.length} change{changedFields.length === 1 ? '' : 's'}. {generation?.instruction ?? (document.restartRequired ? 'Restart the server to apply the pending change.' : 'The server did not report application timing.')}</InlineNotice>}
    {groups.map((group) => {
      const capability = capabilityFor(group, summary);
      const readOnly = viewer.role === 'user' || capabilityCannotEdit(capability);
      const capabilityState = capabilityStateLabel(capability);
      return <SettingsGroup key={group.id} title={group.title} description={group.description}>
        {capabilityState && <InlineNotice tone={capabilityStatus(capability) === 'invalid' || capabilityStatus(capability) === 'degraded' ? 'warn' : 'info'}>{capabilityState}</InlineNotice>}
        {capability?.requiresPorticoClaim && !capability.configured && <InlineNotice tone="info">Claim this server with a Portico account before changing this group.</InlineNotice>}
        {capability?.requiresRuntimeDependency && !capability.configured && <InlineNotice tone="warn">A required server dependency is not currently available.</InlineNotice>}
        {group.fields.filter((field) => visibilityMatches(field.visibleWhen, document, draft)).map((field) => {
          const rawValue = fieldValue(document, draft, group, field);
          const resolved = resolvedFieldState(rawValue);
          const value = resolved?.requestedValue ?? rawValue;
          const restartRequired = document.restartRequiredFields.includes(`${group.settingsKey}.${field.field}`);
          const fieldKey = `${group.settingsKey}.${field.field}`;
          const fieldError = fieldErrors.get(fieldKey);
          const warning = field.warningByValue?.[String(value)];
          const indicator = restartRequired ? <span className="portico-setting-restart">Restart</span> : undefined;
          let control;
          if (field.kind === 'toggle') {
            control = <ToggleControl label={field.label} value={booleanValue(value)} disabled={readOnly} onChange={(next) => update(group.settingsKey, field.field, next)} />;
          } else if (field.kind === 'number') {
            control = <NumberControl label={field.label} value={numberValue(value)} disabled={readOnly} min={field.min} max={field.max} step={field.step} unit={field.unit} fieldKey={fieldKey} error={fieldError} onChange={(next) => update(group.settingsKey, field.field, next)} />;
          } else if (field.kind === 'choice') {
            control = <ChoiceControl label={field.label} value={stringValue(value)} disabled={readOnly} options={field.options ?? []} onChange={(next) => update(group.settingsKey, field.field, next)} />;
          } else if (field.kind === 'string-list') {
            control = <StringListControl label={field.label} value={stringListValue(value)} disabled={readOnly} placeholder={field.placeholder} onChange={(next) => update(group.settingsKey, field.field, next)} />;
          } else if (field.kind === 'secret') {
            const secretDraft = draft[group.settingsKey]?.[field.field] as SecretChange;
            control = <SecretControl label={field.label} present={secretPresent(groupRecord(document.groups, group.settingsKey)[field.field])} value={secretDraft} disabled={readOnly} onChange={(next) => {
              if (next !== undefined) intentKey.current = settingsIdempotencyKey();
              setDraft((current) => {
                const fields = { ...(current[group.settingsKey] ?? {}) };
                if (next === undefined) delete fields[field.field]; else fields[field.field] = next;
                const updated = { ...current, [group.settingsKey]: fields };
                if (Object.keys(fields).length === 0) delete updated[group.settingsKey];
                return updated;
              });
              setFeedback(''); setError('');
            }} />;
          } else {
            control = <TextControl label={field.label} value={stringValue(value)} disabled={readOnly} placeholder={field.placeholder} multiline={field.kind === 'textarea'} onChange={(next) => update(group.settingsKey, field.field, next)} />;
          }
          return <Fragment key={`${group.id}-${field.field}`}><SettingRow label={field.label} description={field.description} indicator={indicator}>{readOnly && field.kind === 'secret' ? <ReadOnlyValue>Restricted</ReadOnlyValue> : control}</SettingRow>{resolved && <div className="portico-setting-resolution" role="status"><span><strong>Requested</strong> {displayResolvedValue(resolved.requestedValue)}</span><span><strong>Effective</strong> {displayResolvedValue(resolved.effectiveValue)}{resolved.effectiveSource ? ` · ${resolved.effectiveSource}` : ''}</span>{(resolved.policyLimit !== undefined || resolved.restrictionReason) && <span><strong>Policy</strong> {displayResolvedValue(resolved.policyLimit)}{resolved.restrictionReason ? ` · ${resolved.restrictionReason}` : ''}</span>}</div>}{warning && <InlineNotice tone="warn"><strong>Storage and I/O warning.</strong> {warning}</InlineNotice>}</Fragment>;
        })}
        {readOnly && <div className="portico-settings-readonly-note"><StatusLockedIcon />{capability?.implemented === false ? 'This server does not currently provide this settings group.' : capabilityState ?? 'Your account can view this group but cannot change it.'}</div>}
      </SettingsGroup>;
    })}
    <SaveBar dirty={dirty} busy={busy} feedback={feedback} error={error} onSave={save} onReset={() => { intentKey.current = undefined; setDraft({}); setFeedback(''); setError(''); setFieldErrors(new Map()); }} />
    {error.includes('another session') && <div className="portico-settings-conflict-action"><SecondaryButton onClick={onReload}><ActionResetIcon /> Reload current settings</SecondaryButton></div>}
  </div>;
}
