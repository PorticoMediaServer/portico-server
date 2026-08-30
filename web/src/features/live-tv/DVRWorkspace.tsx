import type { ActionableDVRRecording, ActionableDVRRule, ActionableLiveTVSource } from '../../data/models';
import { StatusWarningIcon, MediaCalendarIcon, MetadataTimeIcon, PlaybackPauseIcon, ActionEditIcon, PlaybackPlayIcon, NavigationSearchIcon, ActionDeleteIcon, MediaVideoIcon, ActionCloseIcon } from '#portico-icons';
import { type ComponentType, type FormEvent, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productText, reviewedProductErrorText } from '../../components/ProductLanguage';
import { useDVR, useLiveTVMutations } from '../../data/DataProvider';
import { usePlaybackSession } from '../player/PlayerSurface';
import { LiveChoiceMenu } from './LiveControls';
import { hasAction, liveActions } from './liveActions';
import { dateTimeLabel, formatBytes, productState, requestError, timeLabel } from './liveFormat';
import { useDVRStatus } from './liveQueries';
import type { DVRConsumerStatus } from './liveTypes';

type DVRSection = 'recordings' | 'schedule' | 'rules' | 'issues';
type Confirmation = { kind: 'recording'; item: ActionableDVRRecording } | { kind: 'rule'; item: ActionableDVRRule };

function normalizedStatus(recording: ActionableDVRRecording) {
  const status = recording.status.toLocaleLowerCase();
  return status === 'complete' ? 'completed' : status;
}

function includesQuery(title: string, query: string) {
  return !query.trim() || title.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase());
}

export function DVRWorkspace({
  sources,
  sourceId,
  StateSurface,
}: {
  sources: ActionableLiveTVSource[];
  sourceId: string;
  StateSurface: ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }>;
}) {
  const [parameters, setParameters] = useSearchParams();
  const requestedSection = parameters.get('section');
  const section: DVRSection = requestedSection === 'schedule' || requestedSection === 'rules' || requestedSection === 'issues' ? requestedSection : 'recordings';
  const navigate = useNavigate();
  const player = usePlaybackSession();
  const mutations = useLiveTVMutations();
  const [revision, setRevision] = useState(0);
  const [query, setQuery] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');
  const [confirmation, setConfirmation] = useState<Confirmation>();
  const [createRuleOpen, setCreateRuleOpen] = useState(false);
  const dvr = useDVR(revision);
  const operational = useDVRStatus(sourceId || undefined, revision);
  const recordings = dvr.status === 'success' ? dvr.data.recordings : [];
  const rules = dvr.status === 'success' ? dvr.data.rules : [];
  const runningAll = recordings.filter((recording) => normalizedStatus(recording) === 'running');
  const scheduledAll = recordings.filter((recording) => normalizedStatus(recording) === 'scheduled');
  const completedAll = recordings.filter((recording) => normalizedStatus(recording) === 'completed');
  const incompleteAll = recordings.filter((recording) => normalizedStatus(recording) === 'incomplete');
  const failedAll = recordings.filter((recording) => normalizedStatus(recording) === 'failed');
  const running = runningAll.filter((recording) => includesQuery(recording.title, query));
  const scheduled = scheduledAll.filter((recording) => includesQuery(recording.title, query));
  const completed = completedAll.filter((recording) => includesQuery(recording.title, query));
  const incomplete = incompleteAll.filter((recording) => includesQuery(recording.title, query));
  const failed = failedAll.filter((recording) => includesQuery(recording.title, query));
  const shownRules = rules.filter((rule) => includesQuery(rule.title, query));
  const operations = operational.supported && operational.query.status === 'success' ? operational.query.data : undefined;
  const issuesCount = failedAll.length + (operations?.conflicts.length ?? 0);
  const canCreateRule = Boolean(operations?.capabilities.canCreateOwnRules && operations.capabilities.actions.includes(liveActions.ruleCreate));

  const selectSection = (next: DVRSection) => {
    const nextParameters = new URLSearchParams(parameters);
    nextParameters.set('tab', 'dvr');
    nextParameters.set('section', next);
    if (sourceId) nextParameters.set('source', sourceId);
    setParameters(nextParameters);
  };

  const playRecording = async (recording: ActionableDVRRecording) => {
    setError('');
    setBusy(`play:${recording.id}`);
    try {
      const playback = await player.startDVR(recording.id);
      if (playback) navigate(`/watch/${playback.media.id}`);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'open this recording' }));
    } finally {
      setBusy('');
    }
  };

  const removeRecording = async (recording: ActionableDVRRecording) => {
    setError('');
    setNotice('');
    setBusy(`recording:${recording.id}`);
    try {
      await mutations.deleteRecording(recording.id);
      setNotice(productText(normalizedStatus(recording) === 'scheduled' ? 'dvr.recording-cancelled' : 'dvr.recording-deleted'));
      setRevision((current) => current + 1);
      setConfirmation(undefined);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'change this recording' }));
    } finally {
      setBusy('');
    }
  };

  const removeRule = async (rule: ActionableDVRRule) => {
    setError('');
    setNotice('');
    setBusy(`rule:${rule.id}`);
    try {
      await mutations.deleteRule(rule.id);
      setNotice(productText('dvr.rule-deleted'));
      setRevision((current) => current + 1);
      setConfirmation(undefined);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'remove this recording rule' }));
    } finally {
      setBusy('');
    }
  };

  const updateRule = async (rule: ActionableDVRRule, patch: Partial<ActionableDVRRule>) => {
    setError('');
    setNotice('');
    setBusy(`rule:${rule.id}`);
    try {
      await mutations.updateRule(rule.id, { ...patch, sourceId: rule.sourceId, title: rule.title, revision: rule.revision });
      setNotice(productText('dvr.rule-updated'));
      setRevision((currentRevision) => currentRevision + 1);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'update this recording rule' }));
      throw reason;
    } finally {
      setBusy('');
    }
  };

  const createRule = async (input: { sourceId: string; title: string; matchType: string; priority?: number }) => {
    setError('');
    setNotice('');
    setBusy('create-rule');
    try {
      await mutations.recordSeries(input);
      setNotice(productText('dvr.series-recording-scheduled'));
      setRevision((current) => current + 1);
      setCreateRuleOpen(false);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'create this recording rule' }));
      throw reason;
    } finally {
      setBusy('');
    }
  };

  const storedBytes = operations?.storage.usedBytes ?? recordings.reduce((total, recording) => total + recording.sizeBytes, 0);
  const openConfirmation = (next: Confirmation) => {
    setError('');
    setNotice('');
    setConfirmation(next);
  };
  return <div className="live-workspace dvr-workspace">
    <div className="dvr-command-row">
      <div className="dvr-summary">
        <div><span>In progress</span><strong>{runningAll.length}</strong></div>
        <div><span>Scheduled</span><strong>{scheduledAll.length}</strong></div>
        <div><span>Completed</span><strong>{completedAll.length}</strong></div>
        <div className={incompleteAll.length ? 'attention' : ''}><span>Incomplete</span><strong>{incompleteAll.length}</strong></div>
        <div className={failedAll.length ? 'attention' : ''}><span>Failed</span><strong>{failedAll.length}</strong></div>
        <div><span>Stored</span><strong>{formatBytes(storedBytes)}</strong></div>
        {operations && <div className={operations.storage.state === 'healthy' ? '' : 'attention'}><span>Available</span><strong>{formatBytes(operations.storage.availableBytes)}</strong></div>}
      </div>
      {canCreateRule && <PrimaryButton onClick={() => setCreateRuleOpen(true)}><MediaCalendarIcon /> {productText('action.create-rule')}</PrimaryButton>}
    </div>
    <div className="dvr-toolbar">
      <nav className="dvr-tabs" aria-label="DVR views">
        <button className={section === 'recordings' ? 'active' : ''} onClick={() => selectSection('recordings')}>Recordings <span>{runningAll.length + completedAll.length + incompleteAll.length}</span></button>
        <button className={section === 'schedule' ? 'active' : ''} onClick={() => selectSection('schedule')}>Schedule <span>{scheduledAll.length}</span></button>
        {(canCreateRule || rules.length > 0) && <button className={section === 'rules' ? 'active' : ''} onClick={() => selectSection('rules')}>Rules <span>{rules.length}</span></button>}
        {(operational.supported || failedAll.length > 0) && <button className={section === 'issues' ? 'active' : ''} onClick={() => selectSection('issues')}>Issues {issuesCount > 0 && <span>{issuesCount}</span>}</button>}
      </nav>
      <label className="dvr-search"><NavigationSearchIcon /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`Filter ${section}`} aria-label={`Filter ${section}`} /></label>
    </div>
    {error && <p className="live-action-message error" role="alert">{error}</p>}
    {notice && <p className="live-action-message" role="status">{notice}</p>}
    {dvr.status === 'loading' && <StateSurface kind="loading" {...productState('dvr.loading')} />}
    {dvr.status === 'error' && (() => {
      const restricted = (dvr.error as Error & { status?: number }).status === 401 || (dvr.error as Error & { status?: number }).status === 403;
      const state = productState(restricted ? 'dvr.restricted' : 'dvr.unavailable');
      return <StateSurface kind={restricted ? 'permission' : 'error'} title={state.title} message={reviewedProductErrorText(dvr.error, restricted ? 'dvr.restricted' : 'dvr.unavailable')} onRetry={() => setRevision((current) => current + 1)} />;
    })()}
    {dvr.status === 'success' && section === 'recordings' && <RecordingsView running={running} completed={completed} incomplete={incomplete} busy={busy} onPlay={playRecording} onConfirm={openConfirmation} StateSurface={StateSurface} />}
    {dvr.status === 'success' && section === 'schedule' && <ScheduleView recordings={scheduled} onConfirm={openConfirmation} StateSurface={StateSurface} />}
    {dvr.status === 'success' && section === 'rules' && <RulesView rules={shownRules} busy={busy} onUpdate={updateRule} onConfirm={openConfirmation} StateSurface={StateSurface} />}
    {dvr.status === 'success' && section === 'issues' && <IssuesView failed={failed} operational={operations} busy={busy} onPlay={playRecording} onConfirm={openConfirmation} StateSurface={StateSurface} />}
    {confirmation && <DVRConfirmationDialog confirmation={confirmation} busy={Boolean(busy)} error={error} onDismiss={() => setConfirmation(undefined)} onConfirm={() => confirmation.kind === 'recording' ? removeRecording(confirmation.item) : removeRule(confirmation.item)} />}
    {createRuleOpen && <CreateRuleDialog sources={sources} sourceId={sourceId} busy={busy === 'create-rule'} error={error} onDismiss={() => setCreateRuleOpen(false)} onCreate={createRule} />}
  </div>;
}

function RecordingsView({ running, completed, incomplete, busy, onPlay, onConfirm, StateSurface }: { running: ActionableDVRRecording[]; completed: ActionableDVRRecording[]; incomplete: ActionableDVRRecording[]; busy: string; onPlay: (recording: ActionableDVRRecording) => Promise<void>; onConfirm: (confirmation: Confirmation) => void; StateSurface: ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }> }) {
  if (!running.length && !completed.length && !incomplete.length) return <StateSurface kind="empty" {...productState('dvr.empty')} />;
  return <div className="dvr-section-stack">
    {running.length > 0 && <section><header><h2>In progress</h2><span>{running.length} recording</span></header><RecordingRows recordings={running} busy={busy} onPlay={onPlay} onConfirm={onConfirm} /></section>}
    {completed.length > 0 && <section><header><h2>Completed</h2><span>{completed.length} {completed.length === 1 ? 'recording' : 'recordings'}</span></header><RecordingRows recordings={completed} busy={busy} onPlay={onPlay} onConfirm={onConfirm} /></section>}
    {incomplete.length > 0 && <section className="failed-recordings"><header><h2>Incomplete recordings</h2><span>{incomplete.length} partial {incomplete.length === 1 ? 'recording' : 'recordings'} kept</span></header><p>Portico kept the playable portion captured before recording stopped.</p><RecordingRows recordings={incomplete} busy={busy} onPlay={onPlay} onConfirm={onConfirm} /></section>}
  </div>;
}

function RecordingRows({ recordings, busy, onPlay, onConfirm }: { recordings: ActionableDVRRecording[]; busy: string; onPlay: (recording: ActionableDVRRecording) => Promise<void>; onConfirm: (confirmation: Confirmation) => void }) {
  return <div className="dvr-list recording-list">{recordings.map((recording) => <article key={recording.id}>
    <span className={`recording-status ${normalizedStatus(recording)}`}><MediaVideoIcon /></span>
    <div><strong>{recording.title}</strong><span>{dateTimeLabel(recording.startsAt)} · {formatBytes(recording.sizeBytes)}</span>{recording.failureMessageId && <span className="recording-error"><StatusWarningIcon /> {requestError({ messageId: recording.failureMessageId }, 'dvr.recording-failed')}</span>}</div>
    <span className={`status-label ${normalizedStatus(recording)}`}>{normalizedStatus(recording)}</span>
    <div className="recording-actions">
      {hasAction(recording, liveActions.recordingPlay) && <SecondaryButton disabled={busy === `play:${recording.id}`} onClick={() => void onPlay(recording)}><PlaybackPlayIcon /> {productText('action.play-recording')}</SecondaryButton>}
      {hasAction(recording, liveActions.recordingDelete) && <IconButton label={`Delete ${recording.title}`} onClick={() => onConfirm({ kind: 'recording', item: recording })}><ActionDeleteIcon /></IconButton>}
    </div>
  </article>)}</div>;
}

function ScheduleView({ recordings, onConfirm, StateSurface }: { recordings: ActionableDVRRecording[]; onConfirm: (confirmation: Confirmation) => void; StateSurface: ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }> }) {
  if (!recordings.length) return <StateSurface kind="empty" {...productState('dvr.schedule-empty')} />;
  return <section className="dvr-section-stack"><header><h2>Upcoming</h2><span>{recordings.length} scheduled</span></header><div className="dvr-list schedule-list">{recordings.map((recording) => <article key={recording.id}>
    <MetadataTimeIcon />
    <div><strong>{recording.title}</strong><span>{dateTimeLabel(recording.startsAt)}–{timeLabel(recording.endsAt)}</span></div>
    <span className="status-label scheduled">Scheduled</span>
    {hasAction(recording, liveActions.recordingCancel) && <IconButton label={`Cancel ${recording.title}`} onClick={() => onConfirm({ kind: 'recording', item: recording })}><ActionCloseIcon /></IconButton>}
  </article>)}</div></section>;
}

function RulesView({ rules, busy, onUpdate, onConfirm, StateSurface }: { rules: ActionableDVRRule[]; busy: string; onUpdate: (rule: ActionableDVRRule, patch: Partial<ActionableDVRRule>) => Promise<void>; onConfirm: (confirmation: Confirmation) => void; StateSurface: ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }> }) {
  const [editingRuleId, setEditingRuleId] = useState('');
  if (!rules.length) return <StateSurface kind="empty" {...productState('dvr.rules-empty')} />;
  return <div className="dvr-list rule-list">{rules.map((rule) => <div className="rule-item" key={rule.id}>
    <article><MediaCalendarIcon /><div><strong>{rule.title}</strong><span>{rule.matchType === 'series' ? 'Series' : 'Single program'} · Priority {rule.priority ?? 50} · {rule.retentionDays ? `Keep ${rule.retentionDays} days` : 'Keep until deleted'} · {rule.maxRecordingsPerSeries ? `Up to ${rule.maxRecordingsPerSeries}` : 'No episode limit'}</span></div><span className={`status-label ${rule.enabled ? 'enabled' : 'paused'}`}>{rule.enabled ? 'Enabled' : 'Paused'}</span><div className="rule-actions">
      {(hasAction(rule, liveActions.ruleEnable) || hasAction(rule, liveActions.ruleDisable)) && <IconButton disabled={busy === `rule:${rule.id}`} label={`${rule.enabled ? 'Pause' : 'Enable'} rule ${rule.title}`} onClick={() => void onUpdate(rule, { enabled: !rule.enabled })}>{rule.enabled ? <PlaybackPauseIcon /> : <PlaybackPlayIcon />}</IconButton>}
      {hasAction(rule, liveActions.ruleEdit) && <IconButton label={`Edit rule ${rule.title}`} className={editingRuleId === rule.id ? 'selected' : ''} onClick={() => setEditingRuleId((current) => current === rule.id ? '' : rule.id)}><ActionEditIcon /></IconButton>}
      {hasAction(rule, liveActions.ruleDelete) && <IconButton label={`Delete rule ${rule.title}`} onClick={() => onConfirm({ kind: 'rule', item: rule })}><ActionDeleteIcon /></IconButton>}
    </div></article>
    {editingRuleId === rule.id && <RuleEditor rule={rule} onCancel={() => setEditingRuleId('')} onSave={async (patch) => { await onUpdate(rule, patch); setEditingRuleId(''); }} />}
  </div>)}</div>;
}

function IssuesView({ failed, operational, busy, onPlay, onConfirm, StateSurface }: { failed: ActionableDVRRecording[]; operational?: DVRConsumerStatus; busy: string; onPlay: (recording: ActionableDVRRecording) => Promise<void>; onConfirm: (confirmation: Confirmation) => void; StateSurface: ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }> }) {
  const hasIssues = failed.length || operational?.conflicts.length;
  if (!hasIssues) return <StateSurface kind="empty" {...productState('dvr.issues-empty')} />;
  return <div className="dvr-issues">
    {operational?.conflicts.map((conflict) => <section className="dvr-issue-row" key={conflict.id}><StatusWarningIcon /><div><strong>{productState('dvr.conflict').title}</strong><span>{requestError({ messageId: conflict.messageId, details: { capacity: conflict.capacity, demand: conflict.demand } }, 'dvr.conflict', { capacity: conflict.capacity, demand: conflict.demand })} · {dateTimeLabel(conflict.startsAt)}–{timeLabel(conflict.endsAt)}</span></div></section>)}
    {failed.length > 0 && <section className="failed-recordings"><header><h2>Failed recordings</h2><span>{failed.length}</span></header><RecordingRows recordings={failed} busy={busy} onPlay={onPlay} onConfirm={onConfirm} /></section>}
  </div>;
}

function RuleEditor({ rule, onCancel, onSave }: { rule: ActionableDVRRule; onCancel: () => void; onSave: (patch: Partial<ActionableDVRRule>) => Promise<void> }) {
  const [startPaddingMinutes, setStartPaddingMinutes] = useState(rule.startPaddingMinutes);
  const [endPaddingMinutes, setEndPaddingMinutes] = useState(rule.endPaddingMinutes);
  const [retentionDays, setRetentionDays] = useState(rule.retentionDays);
  const [maxRecordingsPerSeries, setMaxRecordingsPerSeries] = useState(rule.maxRecordingsPerSeries);
  const [priority, setPriority] = useState(rule.priority ?? 50);
  const [requiredKeywords, setRequiredKeywords] = useState((rule.requiredKeywords ?? []).join(', '));
  const [blockedKeywords, setBlockedKeywords] = useState((rule.blockedKeywords ?? []).join(', '));
  const [busy, setBusy] = useState(false);
  const keywords = (value: string) => value.split(',').map((keyword) => keyword.trim()).filter(Boolean);
  const save = async () => {
    setBusy(true);
    try {
      await onSave({ startPaddingMinutes, endPaddingMinutes, retentionDays, maxRecordingsPerSeries, priority, revision: rule.revision, requiredKeywords: keywords(requiredKeywords), blockedKeywords: keywords(blockedKeywords) });
    } catch {
      setBusy(false);
    }
  };
  return <form className="rule-editor" onSubmit={(event) => { event.preventDefault(); void save(); }}>
    <label><span>Start padding</span><span className="rule-number-field"><input aria-label="Start padding" type="number" min={0} max={120} value={startPaddingMinutes} onChange={(event) => setStartPaddingMinutes(Number(event.target.value))} /> minutes</span></label>
    <label><span>End padding</span><span className="rule-number-field"><input aria-label="End padding" type="number" min={0} max={120} value={endPaddingMinutes} onChange={(event) => setEndPaddingMinutes(Number(event.target.value))} /> minutes</span></label>
    <label><span>Retention</span><span className="rule-number-field"><input aria-label="Retention" type="number" min={0} max={3650} value={retentionDays} onChange={(event) => setRetentionDays(Number(event.target.value))} /> days</span></label>
    <label><span>Maximum episodes</span><span className="rule-number-field"><input aria-label="Maximum episodes" type="number" min={0} max={1000} value={maxRecordingsPerSeries} onChange={(event) => setMaxRecordingsPerSeries(Number(event.target.value))} /> recordings</span></label>
    <label><span>Priority</span><span className="rule-number-field"><input aria-label="Recording priority" type="number" min={0} max={100} value={priority} onChange={(event) => setPriority(Number(event.target.value))} /> of 100</span></label>
    <label className="rule-wide-field"><span>Required keywords</span><input value={requiredKeywords} onChange={(event) => setRequiredKeywords(event.target.value)} placeholder="Separate keywords with commas" /></label>
    <label className="rule-wide-field"><span>Blocked keywords</span><input value={blockedKeywords} onChange={(event) => setBlockedKeywords(event.target.value)} placeholder="Separate keywords with commas" /></label>
    <div><SecondaryButton disabled={busy} onClick={onCancel}>{productText('action.cancel')}</SecondaryButton><PrimaryButton disabled={busy} type="submit">{busy ? productText('state.saving') : productText('action.save-rule')}</PrimaryButton></div>
  </form>;
}

function DVRConfirmationDialog({ confirmation, busy, error, onDismiss, onConfirm }: { confirmation: Confirmation; busy: boolean; error: string; onDismiss: () => void; onConfirm: () => Promise<void> }) {
  const recording = confirmation.kind === 'recording' ? confirmation.item : undefined;
  const scheduled = recording && normalizedStatus(recording) === 'scheduled';
  const title = confirmation.kind === 'rule' ? `Delete “${confirmation.item.title}”?` : scheduled ? `Cancel “${recording.title}”?` : `Delete “${recording?.title}”?`;
  const description = confirmation.kind === 'rule'
    ? 'This deletes the rule and removes recordings it scheduled that have not started. Recordings already in progress continue, and stored recordings are kept.'
    : scheduled
      ? 'This removes the scheduled recording. It does not change any series rule that may have created it.'
      : recording && normalizedStatus(recording) === 'incomplete'
        ? 'This deletes the incomplete DVR recording and the playable partial file Portico kept. This cannot be undone.'
        : 'This deletes the DVR recording and its stored recording file. This cannot be undone.';
  return <ModalOverlay className="dvr-confirm-dialog" labelledBy="dvr-confirm-title" onDismiss={onDismiss}>
    <header><div><h2 id="dvr-confirm-title">{title}</h2><p>{confirmation.kind === 'rule' ? 'Recording rule' : scheduled ? 'Scheduled recording' : 'DVR recording'}</p></div><IconButton label="Close confirmation" onClick={onDismiss}><ActionCloseIcon /></IconButton></header>
    <p>{description}</p>
    {error && <p className="dvr-dialog-error" role="alert">{error}</p>}
    <footer><SecondaryButton disabled={busy} onClick={onDismiss}>{productText('action.cancel')}</SecondaryButton><button type="button" className="button danger" disabled={busy} onClick={() => void onConfirm()}><ActionDeleteIcon /> {busy ? 'Working' : confirmation.kind === 'recording' && scheduled ? productText('action.cancel-recording') : 'Delete'}</button></footer>
  </ModalOverlay>;
}

function CreateRuleDialog({ sources, sourceId, busy, error, onDismiss, onCreate }: { sources: ActionableLiveTVSource[]; sourceId: string; busy: boolean; error: string; onDismiss: () => void; onCreate: (input: { sourceId: string; title: string; matchType: string; priority?: number }) => Promise<void> }) {
  const [title, setTitle] = useState('');
  const [selectedSourceId, setSelectedSourceId] = useState(sourceId || sources[0]?.id || '');
  const [priority, setPriority] = useState(50);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (title.trim() && selectedSourceId) void onCreate({ sourceId: selectedSourceId, title: title.trim(), matchType: 'series', priority });
  };
  return <ModalOverlay className="dvr-rule-dialog" labelledBy="create-rule-title" onDismiss={onDismiss}>
    <form onSubmit={submit}>
      <header><div><h2 id="create-rule-title">New recording rule</h2><p>Schedule matching programs automatically</p></div><IconButton label="Close new rule" onClick={onDismiss}><ActionCloseIcon /></IconButton></header>
      <div className="dvr-rule-fields">
        <label><span>Program or series title</span><input autoFocus value={title} onChange={(event) => setTitle(event.target.value)} /></label>
        <LiveChoiceMenu label="Source" value={selectedSourceId} choices={sources.map((source) => ({ id: source.id, label: source.name }))} onChange={setSelectedSourceId} />
        <p className="dvr-rule-hint">Records every matching episode available to this profile.</p>
        <label><span>Priority</span><input aria-label="Recording priority" type="number" min={0} max={100} value={priority} onChange={(event) => setPriority(Number(event.target.value))} /></label>
        {error && <p className="dvr-dialog-error" role="alert">{error}</p>}
      </div>
      <footer><SecondaryButton disabled={busy} onClick={onDismiss}>{productText('action.cancel')}</SecondaryButton><PrimaryButton disabled={busy || !title.trim() || !selectedSourceId} type="submit">{busy ? 'Creating' : productText('action.create-rule')}</PrimaryButton></footer>
    </form>
  </ModalOverlay>;
}
