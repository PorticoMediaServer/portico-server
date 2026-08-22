import { ApiError, type SettingsDocument, type SettingsGroups, type SettingsGroupsUpdate, type SettingsGroupSummary, type SettingsSummaryResponse } from '@porticomediaserver/client-core';
import { LockKeyhole, RotateCcw } from '#portico-icons';
import { Fragment, useEffect, useMemo, useState } from 'react';
import { SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
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
  const { busy, run } = useAbortableMutation();
  const groups = useMemo(() => (serverSettingFieldGroups[section] ?? []).filter((group) => isGroupVisible(group, document, summary) && visibilityMatches(group.visibleWhen, document, draft)), [document, draft, section, summary]);
  const dirty = changedFieldCount(draft) > 0;

  useEffect(() => {
    setDraft({});
    setFeedback('');
    setError('');
    setFieldErrors(new Map());
  }, [document.revision, section]);

  const update = (group: WritableSettingsGroup, field: string, value: DraftValue) => {
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
      setFieldErrors(validation);
      setError('Fix the highlighted settings before saving.');
      const first = validation.keys().next().value as string | undefined;
      if (first) window.requestAnimationFrame(() => globalThis.document.querySelector<HTMLElement>(`[data-settings-field="${first}"]`)?.focus());
      return;
    }
    setFieldErrors(new Map());
    try {
      const updated = await run((signal) => source.updateSettings({ expectedRevision: document.revision, groups: sanitizeDraft(draft) }, signal));
      onDocumentChange(updated);
      setDraft({});
      setFeedback(`${fieldCount} ${fieldCount === 1 ? 'setting' : 'settings'} saved.`);
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 409) {
        setError('These settings changed in another session. Reload before saving again.');
      } else {
        setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'save these settings' }));
      }
    }
  };

  if (groups.length === 0) return <div className="portico-settings-state"><LockKeyhole /><strong>No settings are available here</strong><p>Your account or this server does not expose any configurable settings in this section.</p></div>;

  return <div className="portico-settings-form">
    {document.restartRequired && <InlineNotice tone="warn">A server restart is required before the most recent change takes effect.</InlineNotice>}
    {groups.map((group) => {
      const capability = capabilityFor(group, summary);
      const readOnly = viewer.role === 'user' || capability?.readOnly || capability?.implemented === false;
      return <SettingsGroup key={group.id} title={group.title} description={group.description}>
        {capability?.requiresPorticoClaim && !capability.configured && <InlineNotice tone="info">Claim this server with a Portico account before changing this group.</InlineNotice>}
        {capability?.requiresRuntimeDependency && !capability.configured && <InlineNotice tone="warn">A required server dependency is not currently available.</InlineNotice>}
        {group.fields.filter((field) => visibilityMatches(field.visibleWhen, document, draft)).map((field) => {
          const value = fieldValue(document, draft, group, field);
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
          return <Fragment key={`${group.id}-${field.field}`}><SettingRow label={field.label} description={field.description} indicator={indicator}>{readOnly && field.kind === 'secret' ? <ReadOnlyValue>Restricted</ReadOnlyValue> : control}</SettingRow>{warning && <InlineNotice tone="warn"><strong>Storage and I/O warning.</strong> {warning}</InlineNotice>}</Fragment>;
        })}
        {readOnly && <div className="portico-settings-readonly-note"><LockKeyhole />{capability?.implemented === false ? 'This server does not currently provide this settings group.' : 'Your account can view this group but cannot change it.'}</div>}
      </SettingsGroup>;
    })}
    <SaveBar dirty={dirty} busy={busy} feedback={feedback} error={error} onSave={save} onReset={() => { setDraft({}); setFeedback(''); setError(''); setFieldErrors(new Map()); }} />
    {error.includes('another session') && <div className="portico-settings-conflict-action"><SecondaryButton onClick={onReload}><RotateCcw /> Reload current settings</SecondaryButton></div>}
  </div>;
}
