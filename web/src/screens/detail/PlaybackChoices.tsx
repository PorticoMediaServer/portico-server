import React, {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {PlayerOffersService, type PlayerOffersData, type PlayerOfferedStream} from '@core/player-offers.ts';
import type {PreparedChoice} from '@core/prepared-media.ts';
import {fixedQualities, heightClass, qualityRequest, type PlaybackChoices as StartChoices} from '@core/playback-v1/choices.ts';
import {dependsOn} from '@core/freshness-hub.ts';
import {useFreshDocument} from '../../app/freshness';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {Button, Select} from '../../ui';
import {PlaybackOptionsClient} from '@core/playback-v1/options.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {preplaySubtitles} from '@core/pending-track-choice.ts';
import {channelsLabel, codecLabel, resolutionLabel, subtitleTrackNames, versionNames, type VersionFacts} from '@core/presentation/index.ts';
import s from './Detail.module.css';
import {useI18n, type I18n} from '../../app/i18n';

/**
 * Pick before play (Spec — Title Pages; the `cinema` experiment's three dropdowns under the
 * title): Version, Audio and Subtitles, preselected from the server's defaults so the usual
 * viewer never touches them. The item's playback offers are read once for the page; the
 * selection travels with Play (version and quality) and through `pending-choice` (tracks).
 *
 * Subtitles are the chosen version's tracks as the playback options list them, one entry per
 * track, chosen by the track's identity: two tracks in one language, a track with no language, SDH
 * and forced tracks are each their own choice. Only when the options cannot be read are the
 * file's tracks offered by language. Untouched, the control reads "Automatic" (the viewer's
 * subtitle preference decides), never a guess at what will show. While the server is applying the
 * profile's last choice for this show or film, it can be forgotten here.
 */
export type PlaybackChoice = Readonly<{prepared?: PreparedChoice; quality?: string; audioStreamIndex?: number; subtitle?: 'off' | {language?: string; resourceId?: string};
  /** What Play sends with the start: the version and audio track picked here, and for a prepared
   * copy the quality it was made at (the server then plays that copy). */
  choices?: StartChoices}>;

export function PlaybackChoices({libraryId, itemId, onChange, compact, facts}: {libraryId: string; itemId: string; onChange: (choice: PlaybackChoice) => void; /** Phones: one row that wraps. */ compact?: boolean; /** What tells one version from another, by its file id (the title's Files list): its edition, bit rate and size. Two versions never read the same. */ facts?: Readonly<Record<string, VersionFacts>>}) {
  const {api} = useSession();
  const scope = useViewerScope();
  const service = useMemo(() => new PlayerOffersService({api, scope: {serverId: scope.serverId, viewerId: scope.viewerId}}), [api, scope.serverId, scope.viewerId]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot);
  useEffect(() => { void service.select({libraryId, itemId}).catch(() => {}); return () => { try { service.cancel(); } catch { /* disposed */ } }; }, [service, libraryId, itemId]);
  const data = snapshot.data;
  useFreshDocument([dependsOn.content(libraryId)], async () => {
    await service.refresh({quiet: true});
    const current = service.getSnapshot();
    if (current.error) throw current.error;
  }, () => !!service.getSnapshot().data);
  const i18n = useI18n();
  const {t} = i18n;
  const [version, setVersion] = useState<string>('');
  const [audio, setAudio] = useState<string>('');
  const [subtitle, setSubtitle] = useState<string>('');
  // The title's playback options: each version's subtitle tracks, and whether the profile's last choice is in force.
  const optionsClient = useMemo(() => new PlaybackOptionsClient(localV1Http(api)), [api]);
  const [optionsRead, setOptionsRead] = useState(0);
  const [options, setOptions] = useState<{itemId: string; options: Awaited<ReturnType<PlaybackOptionsClient['options']>>}>();
  const [forgetting, setForgetting] = useState(false);
  useEffect(() => {
    let live = true;
    void optionsClient.options(itemId).then(o => { if (live) setOptions({itemId, options: o}); }, () => {});
    return () => { live = false; };
  }, [optionsClient, itemId, optionsRead]);
  const current = options?.itemId === itemId ? options.options : undefined;
  useEffect(() => {
    if (data && version && !data.sources.some(s => s.id === version) && !data.preparedVersions.some(v => `prepared:${v.id}` === version && v.selectable)) {
      setVersion(''); setAudio(''); setSubtitle('');
    }
  }, [data, version]);
  // Defaults from the offers: the current (or first available) source, its default audio stream, subtitles off.
  const source = useMemo(() => data ? (data.sources.find(x => x.id === (version || data.current?.sourceId)) ?? data.sources.find(x => x.available) ?? data.sources[0]) : undefined, [data, version]);
  const audioStreams = useMemo(() => source?.streams.filter(x => x.type === 'audio' && x.enabled) ?? [], [source]);
  const subtitleResources = data?.subtitlePlan?.resources ?? [];
  const subtitleStreams = useMemo(() => source?.streams.filter(x => x.type === 'subtitle' && x.language && x.language !== 'und') ?? [], [source]);
  const optionTracks = useMemo(() => {
    const listed = current?.versions.find(v => v.id === source?.id) ?? (current?.versions.length === 1 ? current.versions[0] : undefined);
    return listed ? preplaySubtitles(listed.subtitles, i18n.t, i18n.locale) : undefined;
  }, [current, source?.id, i18n]);
  useEffect(() => {
    if (!data) return;
    const choice: PlaybackChoice = {
      ...(version && version.startsWith('prepared:') ? preparedChoice(data, version.slice('prepared:'.length)) : {}),
      ...(audio ? {audioStreamIndex: Number(audio)} : {}),
      ...(subtitle === '' ? {} : subtitle === 'off' ? {subtitle: 'off' as const} : subtitle.startsWith('lang:') ? {subtitle: {language: subtitle.slice(5)}} : {subtitle: {resourceId: subtitle}}),
      ...startChoices(data, version, audio),
    };
    onChange(choice);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, version, audio, subtitle]);
  if (!data) return null;
  const versions = [
    ...versionNames(data.sources.map(x => ({id: x.id, label: sourceLabel(i18n, x.height, x.videoCodec, x.container)})), facts ?? {}, t).map(x => ({value: x.id, label: x.label})),
    ...data.preparedVersions.filter(v => v.selectable && v.state === 'published').map(v => ({value: `prepared:${v.id}`, label: `${sourceLabel(i18n, v.facts.height, v.facts.videoCodec, v.facts.container)} · ${t('title.preparedVersion')}`})),
  ];
  const audioOptions = audioStreams.map((x, i) => ({value: String(x.index), label: audioLabel(i18n, x, i + 1)}));
  const subtitleTracks = subtitleResources.some(r => r.enabled)
    ? (names => subtitleResources.filter(r => r.enabled).map(r => ({value: r.id, label: names.get(r.id) ?? r.id})))(subtitleTrackNames(subtitleResources.filter(r => r.enabled), t, i18n.locale))
    : optionTracks ? optionTracks.map(x => ({value: x.id, label: x.label}))
    : [...new Map(subtitleStreams.map(x => [x.language!, {value: `lang:${x.language}`, label: i18n.languageName(x.language!)}])).values()];
  const lastUsed = current?.preferred?.reason === 'last_used';
  const forget = () => {
    setForgetting(true);
    void optionsClient.forgetTrackMemory(itemId).then(() => setOptionsRead(n => n + 1), () => {}).finally(() => setForgetting(false));
  };
  const subtitleOptions = [{value: '', label: t('title.subtitlesAutomatic')}, {value: 'off', label: t('title.subtitlesOff')}, ...subtitleTracks];
  // One control is noise; the row appears when there is a choice to make somewhere.
  if (versions.length < 2 && audioOptions.length < 2 && !subtitleTracks.length && !lastUsed) return null;
  return (
    <div className={compact ? s.choicesCompact : s.choicesRow} aria-label={t('title.playbackChoices')}>
      {versions.length > 1 ? <Select label={t('title.version')} value={version || source?.id || ''} options={versions} onChange={e => { setVersion(e.target.value); setAudio(''); setSubtitle(''); }} /> : null}
      {audioOptions.length > 1 ? <Select label={t('title.audio')} value={audio || String(audioStreams.find(x => x.default)?.index ?? audioStreams[0]?.index ?? '')} options={audioOptions} onChange={e => setAudio(e.target.value)} /> : null}
      {subtitleTracks.length ? <Select label={t('title.subtitles')} value={subtitle} options={subtitleOptions} onChange={e => setSubtitle(e.target.value)} /> : null}
      {lastUsed ? <Button variant="ghost" size="sm" label={t('title.forgetLastChoice')} title={t('title.forgetLastChoiceHelp')} disabled={forgetting} onClick={forget} /> : null}
    </div>
  );
}

/** The picks as the start request carries them; nothing when everything is left at its default. */
function startChoices(data: PlayerOffersData, version: string, audio: string): {choices?: StartChoices} {
  const out: {versionId?: string; audioTrackId?: string; quality?: StartChoices['quality']} = {};
  if (version.startsWith('prepared:')) {
    const prepared = data.preparedVersions.find(x => x.id === version.slice('prepared:'.length));
    const step = prepared?.facts.height ? fixedQualities.find(q => q.height === heightClass(prepared.facts.height)) : undefined;
    if (step) out.quality = qualityRequest(step.id);
  } else if (version && version !== data.current?.sourceId && data.sources.length > 1) out.versionId = version;
  if (audio && /^\d{1,4}$/.test(audio)) out.audioTrackId = 'a' + Number(audio);
  return Object.keys(out).length ? {choices: out} : {};
}

function preparedChoice(data: PlayerOffersData, id: string): {prepared?: PreparedChoice} {
  const v = data.preparedVersions.find(x => x.id === id);
  return v ? {prepared: {versionId: v.id, expectedRevision: v.revision, offersRevision: data.preparedOffersRevision}} : {};
}

function sourceLabel(i18n: I18n, height: number, videoCodec: string, container: string): string {
  return [height ? resolutionLabel(height, i18n.t) : undefined, videoCodec ? codecLabel(videoCodec) : undefined, container ? container.toUpperCase() : undefined].filter(Boolean).join(' · ');
}

function audioLabel(i18n: I18n, x: PlayerOfferedStream, n: number): string {
  const language = x.language && x.language !== 'und' ? i18n.languageName(x.language) : undefined;
  const layout = channelsLabel({channels: x.channels, channelLayout: x.channelLayout}, i18n.t);
  return [language ?? x.title ?? i18n.t('mediaInfo.track', {number: n}), x.codec ? codecLabel(x.codec) : undefined, layout].filter(Boolean).join(' · ');
}
