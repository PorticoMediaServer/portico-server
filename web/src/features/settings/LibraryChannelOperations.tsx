import type { LibraryChannelAggregate, LibraryChannelBlock, LibraryChannelBlockPreset, LibraryChannelConfigurationRequest, LibraryChannelRule } from '@portico/client-core';
import { AlertTriangle, CalendarClock, Pencil, Plus, RefreshCw, RotateCcw, Sparkles, Trash2, TvMinimalPlay, Upload, X } from '#portico-icons';
import { type ChangeEvent, useCallback, useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { ChoiceControl, InlineNotice, NumberControl, SettingsError, SettingsGroup, SettingsLoading, TextControl, ToggleControl } from './SettingsControls';
import { useAbortableMutation, useSettingsQuery } from './settingsHooks';
import type { SettingsDataSource, SettingsViewer } from './settingsTypes';
import { canManageServer } from '../../data/authority';
import { requestError } from '../live-tv/liveFormat';

const weekdayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const defaultTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

function id(prefix: string) {
  return `${prefix}-${globalThis.crypto.randomUUID()}`;
}

function csv(value: string) {
  return value.split(',').map((item) => item.trim()).filter(Boolean);
}

function queryValue(rule: LibraryChannelRule, field: string): string[] {
  const root = rule.query as { all?: Array<{ field?: string; value?: unknown }> };
  const value = root.all?.find((candidate) => candidate.field === field)?.value;
  return Array.isArray(value) ? value.map(String) : value == null ? [] : [String(value)];
}

function ruleQuery(existing: LibraryChannelRule['query'], entityKinds: string[], genres: string[], tags: string[], decade: string): LibraryChannelRule['query'] {
  const all: Array<Record<string, unknown>> = [];
  if (entityKinds.length === 1) all.push({ field: 'entityKind', operator: 'equals', value: entityKinds[0] });
  else if (entityKinds.length > 1) all.push({ field: 'entityKind', operator: 'in', value: entityKinds });
  if (genres.length === 1) all.push({ field: 'genre', operator: 'contains', value: genres[0] });
  else if (genres.length > 1) all.push({ field: 'genre', operator: 'contains-any', value: genres });
  if (tags.length === 1) all.push({ field: 'tag', operator: 'contains', value: tags[0] });
  else if (tags.length > 1) all.push({ field: 'tag', operator: 'contains-any', value: tags });
  if (decade) all.push({ field: 'decade', operator: 'equals', value: Number(decade) });
  const root = existing && typeof existing === 'object' ? existing as Record<string, unknown> : {};
  const existingAll = Array.isArray(root.all) ? root.all as Array<Record<string, unknown>> : [];
  const preserved = existingAll.filter((candidate) => !['entityKind', 'genre', 'tag', 'decade'].includes(String(candidate.field ?? '')));
  return { ...root, all: [...preserved, ...all] } as LibraryChannelRule['query'];
}

function defaultRule(): LibraryChannelRule {
  return { id: id('rule'), name: 'Main programming', enabled: true, sortOrder: 0, query: {}, selectionMode: 'shuffle_bag', episodeMode: 'in_order', exhaustionMode: 'loop', dedupeWindow: 12, maxConsecutive: 4, config: {} };
}

function defaultBlock(ruleId: string, order: number): LibraryChannelBlock {
  return { id: id('block'), ruleId, name: 'Programming block', enabled: true, weekdayMask: 127, startMinute: 19 * 60, endMinute: 22 * 60, priority: 10, anchored: true, allowOverrun: false, sortOrder: order };
}

function blankAggregate(): LibraryChannelAggregate {
  const rule = defaultRule();
  const now = new Date().toISOString();
  return {
    channel: { id: '', sourceType: 'library-channel', name: '', description: '', enabled: true, sortOrder: 0, timezone: defaultTimezone, defaultRuleId: rule.id, qualityProfile: 'auto', logo: { source: 'none', bugEnabled: false, bugOverheadAccepted: false, bugCorner: 'top_right', bugWidthPercent: 9, bugInsetPercent: 2, bugTreatment: 'color' }, configRevision: 0, healthState: 'pending', createdAt: now, updatedAt: now },
    rules: [rule], blocks: [],
  };
}

function ChannelEditor({ aggregate: initial, presets, source, onDismiss, onSaved }: { aggregate: LibraryChannelAggregate; presets: LibraryChannelBlockPreset[]; source: SettingsDataSource; onDismiss: () => void; onSaved: (message: string) => void }) {
  const mutation = useAbortableMutation();
  const [aggregate, setAggregate] = useState(initial);
  const [error, setError] = useState('');
  const [genres, setGenres] = useState(() => queryValue(initial.rules[0], 'genre').join(', '));
  const [tags, setTags] = useState(() => queryValue(initial.rules[0], 'tag').join(', '));
  const [decade, setDecade] = useState(() => queryValue(initial.rules[0], 'decade')[0] ?? '');
  const initialFilterState = useState(() => ({ genres, tags, decade, episodeMode: initial.rules[0].episodeMode }))[0];
  const channel = aggregate.channel;
  const rule = aggregate.rules[0];

  const setChannel = (patch: Partial<typeof channel>) => setAggregate((current) => ({ ...current, channel: { ...current.channel, ...patch } }));
  const setRule = (patch: Partial<LibraryChannelRule>) => setAggregate((current) => ({ ...current, rules: current.rules.map((item, index) => index === 0 ? { ...item, ...patch } : item) }));
  const updateBlock = (index: number, patch: Partial<LibraryChannelBlock>) => setAggregate((current) => ({ ...current, blocks: current.blocks.map((block, blockIndex) => blockIndex === index ? { ...block, ...patch } : block) }));

  const uploadLogo = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    setError('');
    try {
      const asset = await mutation.run((signal) => source.uploadLibraryChannelLogo(file, signal));
      setChannel({ logo: { ...channel.logo, source: 'custom', ref: asset.id, url: asset.url, mimeType: asset.mimeType } });
    } catch (reason) {
      setError(requestError(reason, 'problem.request-failed'));
    } finally {
      event.target.value = '';
    }
  };

  const save = async () => {
    if (!channel.name.trim()) { setError('Enter a channel name.'); return; }
    if (channel.logo.bugEnabled && !channel.logo.bugOverheadAccepted) { setError('Accept the transcode overhead before enabling the on-screen logo.'); return; }
    const kinds = rule.episodeMode === 'none' ? ['movie'] : ['show'];
    const filtersChanged = genres !== initialFilterState.genres || tags !== initialFilterState.tags || decade !== initialFilterState.decade || rule.episodeMode !== initialFilterState.episodeMode;
    const rules = aggregate.rules.map((item, index) => index === 0 ? { ...item, name: item.name.trim() || 'Main programming', query: filtersChanged ? ruleQuery(item.query, kinds, csv(genres), csv(tags), decade) : item.query } : item);
    const input: LibraryChannelConfigurationRequest = {
      expectedRevision: channel.configRevision,
      reshuffle: false,
      name: channel.name.trim(), description: channel.description.trim(), enabled: channel.enabled,
      sortOrder: channel.sortOrder, timezone: channel.timezone || defaultTimezone, defaultRuleId: rule.id,
      qualityProfile: channel.qualityProfile, logo: channel.logo, rules, blocks: aggregate.blocks,
      templateKey: channel.templateKey, templateVersion: channel.templateVersion,
    };
    setError('');
    try {
      if (channel.id) await mutation.run((signal) => source.updateLibraryChannel(channel.id, input, signal));
      else await mutation.run((signal) => source.createLibraryChannel(input, signal));
      onSaved(channel.id ? `${channel.name} saved and queued for schedule refresh.` : `${channel.name} created.`);
    } catch (reason) {
      setError(requestError(reason, 'problem.request-failed'));
    }
  };

  return <ModalOverlay labelledBy="library-channel-editor-title" className="portico-settings-dialog library-channel-editor" onDismiss={onDismiss}>
    <header><div><h2 id="library-channel-editor-title">{channel.id ? `Edit ${channel.name}` : 'Create Library Channel'}</h2><p>Library rules, programming order, schedule blocks, and stream branding</p></div><IconButton label="Close" onClick={onDismiss}><X /></IconButton></header>
    <div className="portico-settings-dialog-fields">
      <fieldset><legend>Channel</legend><div className="library-channel-editor-grid">
        <label><span>Name</span><TextControl label="Channel name" value={channel.name} onChange={(name) => setChannel({ name })} /></label>
        <label><span>Timezone</span><TextControl label="Channel timezone" value={channel.timezone} onChange={(timezone) => setChannel({ timezone })} /></label>
        <label className="wide"><span>Description</span><TextControl label="Channel description" multiline value={channel.description} onChange={(description) => setChannel({ description })} /></label>
        <label><span>Published</span><ToggleControl label="Publish channel" value={channel.enabled} onChange={(enabled) => setChannel({ enabled })} /></label>
        <div><span>Stream quality</span><ChoiceControl label="Stream quality" value={channel.qualityProfile} options={['auto', 'original', '1080p-medium', '720p-medium', '480p'].map((value) => ({ value, label: value === 'auto' ? 'Automatic' : value }))} onChange={(qualityProfile) => setChannel({ qualityProfile: qualityProfile as typeof channel.qualityProfile })} /></div>
      </div></fieldset>

      <fieldset><legend>Library rule</legend><div className="library-channel-editor-grid">
        <label><span>Rule name</span><TextControl label="Rule name" value={rule.name} onChange={(name) => setRule({ name })} /></label>
        <div><span>Programming mode</span><ChoiceControl label="Programming mode" value={rule.episodeMode} options={[{ value: 'none', label: 'Movies' }, { value: 'in_order', label: 'TV sequential' }, { value: 'marathon', label: 'TV marathon' }, { value: 'randomized', label: 'TV random' }]} onChange={(episodeMode) => setRule({ episodeMode: episodeMode as LibraryChannelRule['episodeMode'] })} /></div>
        <div><span>Item order</span><ChoiceControl label="Item order" value={rule.selectionMode} options={[{ value: 'sequential', label: 'Library sort order' }, { value: 'shuffle_bag', label: 'Shuffle without repeats' }, { value: 'weighted_random', label: 'Deterministic random' }]} onChange={(selectionMode) => setRule({ selectionMode: selectionMode as LibraryChannelRule['selectionMode'] })} /></div>
        <label><span>Genres <small>Comma separated</small></span><TextControl label="Genres" value={genres} placeholder="Comedy, Animation" onChange={setGenres} /></label>
        <label><span>Tags <small>Comma separated</small></span><TextControl label="Tags" value={tags} placeholder="family, classics" onChange={setTags} /></label>
        <label><span>Decade</span><TextControl label="Decade" value={decade} placeholder="1990" onChange={(value) => setDecade(value.replace(/\D/g, '').slice(0, 4))} /></label>
        <label><span>Avoid repeats</span><NumberControl label="Avoid recent repeats" value={rule.dedupeWindow} min={0} max={1000} unit="items" onChange={(dedupeWindow) => setRule({ dedupeWindow: dedupeWindow ?? 0 })} /></label>
        <label><span>Max same series</span><NumberControl label="Maximum consecutive series items" value={rule.maxConsecutive} min={0} max={1000} onChange={(maxConsecutive) => setRule({ maxConsecutive: maxConsecutive ?? 0 })} /></label>
      </div></fieldset>

      <fieldset><legend>Programming blocks <small>Optional weekly overrides</small></legend>
        {presets.length > 0 && <div className="library-channel-block-presets" aria-label="Programming block presets">{presets.map((preset) => <button key={preset.key} type="button" onClick={() => setAggregate((current) => ({ ...current, blocks: [...current.blocks, { ...defaultBlock(rule.id, current.blocks.length), name: preset.name, weekdayMask: preset.weekdays, startMinute: preset.startMinute, endMinute: preset.endMinute, anchored: preset.anchored, allowOverrun: preset.allowOverrun, templateKey: preset.key, templateVersion: preset.version }] }))}><strong>{preset.name}</strong><span>{preset.description}</span></button>)}</div>}
        <div className="library-channel-blocks">{aggregate.blocks.map((block, index) => <article key={block.id}>
          <div className="library-channel-block-heading"><CalendarClock /><TextControl label="Block name" value={block.name} onChange={(name) => updateBlock(index, { name })} /><IconButton label={`Remove ${block.name}`} onClick={() => setAggregate((current) => ({ ...current, blocks: current.blocks.filter((_, blockIndex) => blockIndex !== index) }))}><Trash2 /></IconButton></div>
          <div className="library-channel-weekdays">{weekdayNames.map((day, dayIndex) => <label key={day}><input type="checkbox" checked={(block.weekdayMask & (1 << dayIndex)) !== 0} onChange={(event) => updateBlock(index, { weekdayMask: event.target.checked ? block.weekdayMask | (1 << dayIndex) : block.weekdayMask & ~(1 << dayIndex) })} />{day}</label>)}</div>
          <div className="library-channel-block-times"><label>Starts <NumberControl label="Block start minute" value={block.startMinute} min={0} max={1439} unit="minute" onChange={(startMinute) => updateBlock(index, { startMinute: startMinute ?? 0 })} /></label><label>Ends <NumberControl label="Block end minute" value={block.endMinute} min={0} max={1439} unit="minute" onChange={(endMinute) => updateBlock(index, { endMinute: endMinute ?? 0 })} /></label><label>Priority <NumberControl label="Block priority" value={block.priority} min={0} max={1000} onChange={(priority) => updateBlock(index, { priority: priority ?? 0 })} /></label></div>
          <div className="library-channel-block-options"><label><input type="checkbox" checked={block.anchored} onChange={(event) => updateBlock(index, { anchored: event.target.checked })} /> Start on schedule</label><label><input type="checkbox" checked={block.allowOverrun} onChange={(event) => updateBlock(index, { allowOverrun: event.target.checked })} /> Allow final program to finish</label></div>
        </article>)}</div>
        <SecondaryButton onClick={() => setAggregate((current) => ({ ...current, blocks: [...current.blocks, defaultBlock(rule.id, current.blocks.length)] }))}><Plus /> Add programming block</SecondaryButton>
      </fieldset>

      <fieldset><legend>Logo and on-screen bug</legend><div className="library-channel-editor-grid">
        <label className="library-channel-logo-upload"><span>Channel logo</span><input type="file" accept="image/png,image/webp,image/svg+xml" onChange={(event) => void uploadLogo(event)} /><span><Upload /> Upload PNG, WebP, or SVG</span></label>
        <SecondaryButton disabled={channel.logo.source === 'none'} onClick={() => setChannel({ logo: { ...channel.logo, source: 'none', ref: undefined, url: undefined, mimeType: undefined, bugEnabled: false, bugOverheadAccepted: false } })}>Remove logo</SecondaryButton>
        <label><span>Show logo over video</span><ToggleControl label="Show logo over video" value={channel.logo.bugEnabled} onChange={(bugEnabled) => setChannel({ logo: { ...channel.logo, bugEnabled, bugOverheadAccepted: bugEnabled ? channel.logo.bugOverheadAccepted : false } })} /></label>
        {channel.logo.bugEnabled && <><div><span>Corner</span><ChoiceControl label="Logo bug corner" value={channel.logo.bugCorner} options={[{ value: 'top_left', label: 'Top left' }, { value: 'top_right', label: 'Top right' }, { value: 'bottom_left', label: 'Bottom left' }, { value: 'bottom_right', label: 'Bottom right' }]} onChange={(bugCorner) => setChannel({ logo: { ...channel.logo, bugCorner: bugCorner as typeof channel.logo.bugCorner } })} /></div><label><span>Width</span><NumberControl label="Logo bug width" value={channel.logo.bugWidthPercent} min={2} max={20} step={0.5} unit="%" onChange={(bugWidthPercent) => setChannel({ logo: { ...channel.logo, bugWidthPercent: bugWidthPercent ?? 9 } })} /></label><label><span>Edge inset</span><NumberControl label="Logo bug edge inset" value={channel.logo.bugInsetPercent} min={0} max={10} step={0.5} unit="%" onChange={(bugInsetPercent) => setChannel({ logo: { ...channel.logo, bugInsetPercent: bugInsetPercent ?? 2 } })} /></label><div><span>Color</span><ChoiceControl label="Logo bug color" value={channel.logo.bugTreatment} options={[{ value: 'color', label: 'Original color' }, { value: 'white', label: 'White' }, { value: 'black', label: 'Black' }]} onChange={(bugTreatment) => setChannel({ logo: { ...channel.logo, bugTreatment: bugTreatment as typeof channel.logo.bugTreatment } })} /></div></>}
      </div>
      {channel.logo.bugEnabled && <InlineNotice tone="warn">Adding a logo to the video forces a transcoded rendition and uses extra server capacity. <label><input type="checkbox" checked={channel.logo.bugOverheadAccepted} onChange={(event) => setChannel({ logo: { ...channel.logo, bugOverheadAccepted: event.target.checked } })} /> I accept the transcode overhead.</label></InlineNotice>}
      </fieldset>
      {error && <InlineNotice tone="error"><AlertTriangle /> {error}</InlineNotice>}
    </div>
    <footer><SecondaryButton disabled={mutation.busy} onClick={onDismiss}>Cancel</SecondaryButton><PrimaryButton disabled={mutation.busy} onClick={() => void save()}>{mutation.busy ? <><RefreshCw className="portico-settings-spinner" /> Saving…</> : 'Save channel'}</PrimaryButton></footer>
  </ModalOverlay>;
}

export function LibraryChannelOperations({ source, viewer }: { source: SettingsDataSource; viewer: SettingsViewer }) {
  const [revision, setRevision] = useState(0);
  const [editor, setEditor] = useState<LibraryChannelAggregate>();
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const load = useCallback((next: SettingsDataSource, signal: AbortSignal) => next.libraryChannels(signal), []);
  const channels = useSettingsQuery(load, source, revision);
  const templatesLoad = useCallback((next: SettingsDataSource, signal: AbortSignal) => next.libraryChannelTemplates(signal), []);
  const templates = useSettingsQuery(templatesLoad, source, revision);

  if (!canManageServer(viewer)) return null;

  const open = async (channelId?: string) => {
    setError('');
    if (!channelId) { setEditor(blankAggregate()); return; }
    try { setEditor(await mutation.run((signal) => source.libraryChannel(channelId, signal))); }
    catch (reason) { setError(requestError(reason, 'library-channel.load-failed')); }
  };

  const restore = async (mode: 'recommended' | 'all') => {
    setError(''); setNotice('');
    try {
      const restored = await mutation.run((signal) => source.restoreLibraryChannelDefaults({ timezone: defaultTimezone, mode }, signal));
      const parts = [`${restored.createdCount} created`, `${restored.existingCount} already present`];
      if (restored.skippedCount) parts.push(`${restored.skippedCount} skipped`);
      setNotice(`Default channels: ${parts.join(', ')}.`);
      setRevision((value) => value + 1);
    } catch (reason) { setError(requestError(reason, 'problem.request-failed')); }
  };

  const regenerate = async (aggregate: LibraryChannelAggregate) => {
    setError(''); setNotice('');
    try { await mutation.run((signal) => source.regenerateLibraryChannel(aggregate.channel.id, signal)); setNotice(`${aggregate.channel.name} schedule update started.`); setRevision((value) => value + 1); }
    catch (reason) { setError(requestError(reason, 'problem.request-failed')); }
  };

  const remove = async (aggregate: LibraryChannelAggregate) => {
    if (!window.confirm(`Delete ${aggregate.channel.name}? This removes its guide and schedule, not library media.`)) return;
    setError(''); setNotice('');
    try { await mutation.run((signal) => source.deleteLibraryChannel(aggregate.channel.id, aggregate.channel.configRevision, signal)); setNotice(`${aggregate.channel.name} deleted.`); setRevision((value) => value + 1); }
    catch (reason) { setError(requestError(reason, 'problem.request-failed')); }
  };

  return <SettingsGroup title="Library Channels" description="Owner-only channels scheduled from library metadata. These remain separate from real tuner Live TV." actions={<PrimaryButton disabled={mutation.busy} onClick={() => void open()}><Plus /> Create channel</PrimaryButton>}>
    <InlineNotice><TvMinimalPlay /> The rolling guide is generated deterministically for seven days. Viewers see these channels only inside the Library Channels guide.</InlineNotice>
    <div className="library-channel-defaults"><div><Sparkles /><span><strong>{templates.status === 'success' ? `${templates.data.templates.length} packaged channel templates` : 'Packaged channel templates'}</strong><small>Eras, genres, documentaries, cartoons, movie nights, and more. Existing channels are never overwritten.</small></span></div><SecondaryButton disabled={mutation.busy} onClick={() => void restore('recommended')}><RotateCcw /> Restore recommended</SecondaryButton><SecondaryButton disabled={mutation.busy} onClick={() => void restore('all')}><RotateCcw /> Add all defaults</SecondaryButton></div>
    {notice && <InlineNotice tone="success">{notice}</InlineNotice>}
    {error && <InlineNotice tone="error"><AlertTriangle /> {error}</InlineNotice>}
    {channels.status === 'loading' && <SettingsLoading label="Loading Library Channels" />}
    {channels.status === 'error' && <SettingsError title="Library Channels are unavailable" message={requestError(channels.error, 'library-channel.load-failed')} onRetry={() => setRevision((value) => value + 1)} />}
    {channels.status === 'success' && !channels.data.items.length && <div className="portico-settings-state"><TvMinimalPlay /><strong>No Library Channels</strong><p>Start blank or restore the recommended defaults.</p></div>}
    {channels.status === 'success' && <div className="library-channel-admin-list">{channels.data.items.map((channel) => <article key={channel.id}>
      <span className="library-channel-admin-logo">{channel.logo.url ? <img src={channel.logo.url} alt="" /> : <TvMinimalPlay />}</span>
      <div><strong>{channel.name}</strong><small>{channel.description || 'No description'}</small><span className={channel.healthState}>{channel.enabled ? channel.healthState : 'disabled'} · {channel.generatedThrough ? `scheduled through ${new Date(channel.generatedThrough).toLocaleDateString()}` : 'schedule pending'}</span></div>
      <div className="library-channel-admin-actions"><IconButton label={`Edit ${channel.name}`} disabled={mutation.busy} onClick={() => void open(channel.id)}><Pencil /></IconButton><IconButton label={`Update ${channel.name} schedule`} disabled={mutation.busy} onClick={() => void (async () => { try { await regenerate(await source.libraryChannel(channel.id, new AbortController().signal)); } catch (reason) { setError(requestError(reason, 'problem.request-failed')); } })()}><RefreshCw /></IconButton><IconButton label={`Delete ${channel.name}`} disabled={mutation.busy} onClick={() => void (async () => { try { await remove(await source.libraryChannel(channel.id, new AbortController().signal)); } catch (reason) { setError(requestError(reason, 'library-channel.load-failed')); } })()}><Trash2 /></IconButton></div>
    </article>)}</div>}
    {editor && <ChannelEditor aggregate={editor} presets={templates.status === 'success' ? templates.data.blockPresets : []} source={source} onDismiss={() => setEditor(undefined)} onSaved={(message) => { setEditor(undefined); setNotice(message); setRevision((value) => value + 1); }} />}
  </SettingsGroup>;
}
