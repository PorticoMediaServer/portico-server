import {useEffect, useMemo, useState, useSyncExternalStore, type RefObject} from 'react';
import type {DetailProjection, FileInfoDisc} from '@core/detail.ts';
import {fileInfoTrackKey, type FileInfoLane, type FileInfoSession, type FileInfoTrackLane} from '@core/file-info-session.ts';
import type {FileInfoFileRow, FileInfoGroupRow, FileInfoSurface} from '@core/file-info-pages.ts';
import {fileInfoPanelModel} from '@core/presentation/file-info-versions.ts';
import {Dialog, DiscTitles, FileInfo, type FileInfoAction, type FileInfoContinuation, type FileInfoMore} from '../../kit/overlays';
import {StateView} from '../../kit/states';
import type {DetailBindings} from './bindings';
import {ScreenFailure} from './Detail';
import {DetailLoading} from './Loading';
import {t, environment} from './text';

/** What an owner's Delete, Split or Merge was chosen from: the opening, and the versions and
 * files that were on screen. Commands send the exact occurrence; the server rechecks the rest. */
export type FileInfoManageContext = Readonly<{
  action: FileInfoAction; opening: string; surface: FileInfoSurface;
  groups: readonly FileInfoGroupRow[];
  files: ReadonlyMap<string, Readonly<{group: FileInfoGroupRow; file: FileInfoFileRow}>>;
}>;
type Props = {
  binding: DetailBindings; data: DetailProjection; surface: 'file' | 'titles'; phone: boolean;
  returnFocusRef: RefObject<HTMLButtonElement | null>; onClose: () => void; onManage: (context: FileInfoManageContext) => void;
};

/** File info and the disc's Titles for one opening of one accepted Detail. Versions, their
 * tracks and the owner's files are read window by window as they are shown; the session and
 * every lease it holds go with the panel. */
export function TitleFileInfo({binding, data, surface, phone, returnFocusRef, onClose, onManage}: Props) {
  const [session, setSession] = useState<FileInfoSession | null>();
  useEffect(() => {
    const opened = binding.openFileInfo(data);
    setSession(opened ?? null);
    void opened?.groups();
    return () => opened?.retire();
  }, [binding, data]);
  // A Detail that is not ready yet has no opening; reading it again brings one.
  if (session === null) return <Dialog open onOpenChange={open => { if (!open) onClose(); }} title={t(surface === 'titles' ? 'next.overlays.discTitles' : 'next.overlays.fileInfo')} returnFocusRef={returnFocusRef}>
    <ScreenFailure retry={() => { void binding.service.revalidate(); }} />
  </Dialog>;
  return session ? <Opened session={session} binding={binding} data={data} surface={surface} phone={phone} returnFocusRef={returnFocusRef} onClose={onClose} onManage={onManage} /> : null;
}

function Opened({session, binding, data, surface, phone, returnFocusRef, onClose, onManage}: Props & {session: FileInfoSession}) {
  const snapshot = useSyncExternalStore(session.subscribe, session.getSnapshot, session.getSnapshot);
  const groups = snapshot.groups.rows?.rows, manage = snapshot.surface?.manage;
  const close = (open: boolean) => { if (!open) onClose(); };
  // The owner's disc block shows in File info and in Titles; members never have one.
  useEffect(() => {
    if (snapshot.surface?.ownerDiscOccurrence && !snapshot.disc) void session.disc();
  }, [session, snapshot.surface, snapshot.disc]);

  const more = <Row,>(lane: FileInfoLane<Row> | undefined, next: () => void): FileInfoMore | undefined =>
    lane && (lane.rows?.more || lane.status === 'failed') ? {busy: lane.status === 'loading' || lane.status === 'preparing', failed: lane.status === 'failed', onPress: next} : undefined;
  const byId = useMemo(() => new Map(groups?.map(group => [group.id, group])), [groups]);
  const continuation = useMemo((): FileInfoContinuation | undefined => groups && {
    versionCount: snapshot.surface?.groupCount ?? groups.length,
    more: more(snapshot.groups, () => { void session.groups(snapshot.groups.status === 'failed' && !snapshot.groups.rows?.more ? 'restart' : 'next'); }),
    earlier: snapshot.groups.rows?.earlier ? {busy: false, failed: false, onPress: () => { void session.groups('restart'); }} : undefined,
    onVersionVisible: id => {
      const group = byId.get(id);
      if (!group) return;
      if (group.audioCount > 0) void session.tracks(group.headOccurrence, 'audio');
      if (group.subtitleCount > 0) void session.tracks(group.headOccurrence, 'subtitles');
      if (manage) void session.members(group.id);
    },
    tracks: (id, kind: FileInfoTrackLane) => {
      const group = byId.get(id);
      if (!group || (kind === 'audio' ? group.audioCount : group.subtitleCount) === 0) return undefined;
      const lane = snapshot.tracks.get(fileInfoTrackKey(group.headOccurrence, kind));
      return {pending: !lane?.rows, more: more(lane, () => { void session.tracks(group.headOccurrence, kind, lane?.rows?.more ? 'next' : 'open'); })};
    },
    locationsMore: id => more(snapshot.members.get(id), () => { void session.members(id, snapshot.members.get(id)?.rows?.more ? 'next' : 'open'); }),
  }, [groups, byId, manage, session, snapshot]);

  // Web lists a version head's raw tracks; Merge is drawn in the phone sheet only (phone-detail 12).
  const model = useMemo(() => fileInfoPanelModel(snapshot, {tracks: 'raw', context: phone ? 'phone' : 'desktop', manageSurface: phone ? 'phone' : 'desktop', split: true, merge: phone}, environment), [phone, snapshot]);

  // The disc block names its version by the version whose head file it was read from.
  const disc = useMemo((): FileInfoDisc | undefined => {
    const lane = snapshot.disc, titles = lane?.rows?.rows;
    if (!lane?.occurrence || !lane.discKind || !titles?.length) return undefined;
    const version = groups?.find(group => group.headOccurrence === lane.occurrence);
    return {fileId: version?.id ?? lane.occurrence, kind: lane.discKind, titles};
  }, [snapshot.disc, groups]);

  const failed = snapshot.groups.status === 'failed' && !groups;
  // A stale opening (409) is refreshed through the Detail: the newer Detail is a new opening.
  const stale = (error: unknown) => (error as {code?: string} | undefined)?.code === 'stale_continuation';
  const retry = () => { if (stale(snapshot.groups.error)) void binding.service.revalidate(); else void session.groups('restart'); };
  if (surface === 'titles') {
    const owner = snapshot.surface && !!snapshot.surface.ownerDiscOccurrence;
    if (disc) return <DiscTitles open onOpenChange={close} disc={disc} file={model?.files.find(file => file.id === disc.fileId)?.display.heading.text} more={more(snapshot.disc, () => { void session.disc('next'); })} returnFocusRef={returnFocusRef} />;
    return <Dialog open onOpenChange={close} title={t('next.overlays.discTitles')} returnFocusRef={returnFocusRef}>
      {failed || snapshot.disc?.status === 'failed' ? <ScreenFailure retry={failed ? retry : () => { void session.disc('restart'); }} />
        : snapshot.surface && !owner ? <StateView title={t('title.nothingHere')} /> : <DetailLoading shape="panel" />}
    </Dialog>;
  }
  if (!model) return <Dialog open onOpenChange={close} title={t('next.overlays.fileInfo')} returnFocusRef={returnFocusRef}>{failed ? <ScreenFailure retry={retry} /> : <DetailLoading shape="panel" />}</Dialog>;
  return <FileInfo model={model} subject={data.item.title} disc={disc} continuation={continuation} layout={phone ? 'phone' : 'desktop'} open onOpenChange={close} returnFocusRef={returnFocusRef}
    onManage={action => {
      binding.current(data);
      const files = new Map<string, Readonly<{group: FileInfoGroupRow; file: FileInfoFileRow}>>();
      for (const group of groups ?? []) for (const file of snapshot.members.get(group.id)?.rows?.rows ?? []) files.set(file.occurrence, Object.freeze({group, file}));
      onManage(Object.freeze({action, opening: session.opening.token, surface: snapshot.surface!, groups: groups ?? [], files}));
    }} />;
}
