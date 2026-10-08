import React, {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {PlayerOffersService, type PlayerOffersData, type PlayerOfferedStream} from '@core/player-offers.ts';
import type {PreparedChoice} from '@core/prepared-media.ts';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {currentI18n} from '../../app/i18n';
import {Select} from '../../ui';
import {channelsLabel, codecLabel, resolutionLabel} from '@core/presentation/index.ts';
import {languageName} from './TitleFiles';
import s from './Detail.module.css';

/**
 * Pick before play (Spec — Title Pages; the `cinema` experiment's three dropdowns under the
 * title): Version, Audio and Subtitles, preselected from the server's defaults so the usual
 * viewer never touches them. The item's playback offers are read once for the page; the
 * selection travels with Play (version and quality) and through `pending-choice` (tracks).
 *
 * Subtitles are the chosen file's own tracks, picked by language: the server builds its subtitle
 * plan inside a session, so before Play there is no plan to list. Untouched, the control reads
 * "Automatic" (the viewer's subtitle preference decides), never a guess at what will show.
 */
export type PlaybackChoice = Readonly<{prepared?: PreparedChoice; quality?: string; audioStreamIndex?: number; subtitle?: 'off' | {language?: string; resourceId?: string}}>;

export function PlaybackChoices({libraryId, itemId, onChange, compact}: {libraryId: string; itemId: string; onChange: (choice: PlaybackChoice) => void; /** Phones: one row that wraps. */ compact?: boolean}) {
  const {api} = useSession();
  const scope = useViewerScope();
  const service = useMemo(() => new PlayerOffersService({api, scope: {serverId: scope.serverId, viewerId: scope.viewerId}}), [api, scope.serverId, scope.viewerId]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot);
  useEffect(() => { void service.select({libraryId, itemId}).catch(() => {}); return () => { try { service.cancel(); } catch { /* disposed */ } }; }, [service, libraryId, itemId]);
  const data = snapshot.data;
  const t = currentI18n().t;
  const [version, setVersion] = useState<string>('');
  const [audio, setAudio] = useState<string>('');
  const [subtitle, setSubtitle] = useState<string>('');
  // Defaults from the offers: the current (or first available) source, its default audio stream, subtitles off.
  const source = useMemo(() => data ? (data.sources.find(x => x.id === (version || data.current?.sourceId)) ?? data.sources.find(x => x.available) ?? data.sources[0]) : undefined, [data, version]);
  const audioStreams = useMemo(() => source?.streams.filter(x => x.type === 'audio' && x.enabled) ?? [], [source]);
  const subtitleResources = data?.subtitlePlan?.resources ?? [];
  const subtitleStreams = useMemo(() => source?.streams.filter(x => x.type === 'subtitle' && x.language && x.language !== 'und') ?? [], [source]);
  useEffect(() => {
    if (!data) return;
    const choice: PlaybackChoice = {
      ...(version && version.startsWith('prepared:') ? preparedChoice(data, version.slice('prepared:'.length)) : {}),
      ...(audio ? {audioStreamIndex: Number(audio)} : {}),
      ...(subtitle === '' ? {} : subtitle === 'off' ? {subtitle: 'off' as const} : subtitle.startsWith('lang:') ? {subtitle: {language: subtitle.slice(5)}} : {subtitle: {resourceId: subtitle}}),
    };
    onChange(choice);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, version, audio, subtitle]);
  if (!data) return null;
  const versions = [
    ...data.sources.map(x => ({value: x.id, label: sourceLabel(x.height, x.videoCodec, x.container)})),
    ...data.preparedVersions.filter(v => v.selectable && v.state === 'published').map(v => ({value: `prepared:${v.id}`, label: `${sourceLabel(v.facts.height, v.facts.videoCodec, v.facts.container)} · ${t('title.preparedVersion')}`})),
  ];
  const audioOptions = audioStreams.map((x, i) => ({value: String(x.index), label: audioLabel(x, i + 1)}));
  const subtitleTracks = subtitleResources.some(r => r.enabled)
    ? subtitleResources.filter(r => r.enabled).map(r => ({value: r.id, label: [r.language && r.language !== 'und' ? languageName(r.language) : undefined, r.title].filter(Boolean).join(' · ') || r.id}))
    : [...new Map(subtitleStreams.map(x => [x.language!, {value: `lang:${x.language}`, label: languageName(x.language!)}])).values()];
  const subtitleOptions = [{value: '', label: t('title.subtitlesAutomatic')}, {value: 'off', label: t('title.subtitlesOff')}, ...subtitleTracks];
  // One control is noise; the row appears when there is a choice to make somewhere.
  if (versions.length < 2 && audioOptions.length < 2 && !subtitleTracks.length) return null;
  return (
    <div className={compact ? s.choicesCompact : s.choicesRow} aria-label={t('title.playbackChoices')}>
      {versions.length > 1 ? <Select label={t('title.version')} value={version || source?.id || ''} options={versions} onChange={e => { setVersion(e.target.value); setAudio(''); setSubtitle(''); }} /> : null}
      {audioOptions.length > 1 ? <Select label={t('title.audio')} value={audio || String(audioStreams.find(x => x.default)?.index ?? audioStreams[0]?.index ?? '')} options={audioOptions} onChange={e => setAudio(e.target.value)} /> : null}
      {subtitleTracks.length ? <Select label={t('title.subtitles')} value={subtitle} options={subtitleOptions} onChange={e => setSubtitle(e.target.value)} /> : null}
    </div>
  );
}

function preparedChoice(data: PlayerOffersData, id: string): {prepared?: PreparedChoice} {
  const v = data.preparedVersions.find(x => x.id === id);
  return v ? {prepared: {versionId: v.id, expectedRevision: v.revision, offersRevision: data.preparedOffersRevision}} : {};
}

function sourceLabel(height: number, videoCodec: string, container: string): string {
  return [height ? resolutionLabel(height, currentI18n().t) : undefined, videoCodec ? codecLabel(videoCodec) : undefined, container ? container.toUpperCase() : undefined].filter(Boolean).join(' · ');
}

function audioLabel(x: PlayerOfferedStream, n: number): string {
  const language = x.language && x.language !== 'und' ? languageName(x.language) : undefined;
  const layout = channelsLabel({channels: x.channels, channelLayout: x.channelLayout}, currentI18n().t);
  return [language ?? x.title ?? currentI18n().t('mediaInfo.track', {number: n}), x.codec ? codecLabel(x.codec) : undefined, layout].filter(Boolean).join(' · ');
}
