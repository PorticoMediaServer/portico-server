import type {TitleFile} from '@core/detail.ts';
import {titleFileLines} from '@core/presentation/index.ts';
import {Button, KeyValue, Text} from '../../ui';
import s from './Detail.module.css';
import {useI18n} from '../../app/i18n';

/**
 * The Files list that closes a title page and an episode panel (Spec — Page Content §0.3): per
 * file, what it is ("4K · HEVC · Dolby Vision · MKV"), its size and bitrate, every audio and
 * subtitle track, and where it lives (owners only). The lines come from the shared model.
 */
export function TitleFiles({files, onSplit, onSplitFolder, folderKind, onDelete, heading = true}: {files: readonly TitleFile[]; /** Its own "Files" label; off where the caller already names the section (an episode's disclosure). */ heading?: boolean; /** Owners, for a track or audiobook part: this file's folder is an album or book of its own. */ onSplitFolder?: (file: TitleFile, summary: string) => void; folderKind?: 'album' | 'book'; /** Owners: delete this one file from disk, leaving the title and its other versions. */ onDelete?: (file: TitleFile, summary: string) => void; /** Owners, where the server can split: a version that is on disk can be made a title of its own. */ onSplit?: (file: TitleFile, summary: string) => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  if (!files.length) return null;
  // A title in several parts numbers its versions within each part.
  const parts = files.some(file => (file.partIndex ?? 0) > 0);
  const versionOf = (index: number) => files.slice(0, index + 1).filter(file => (file.partIndex ?? 0) === (files[index]!.partIndex ?? 0)).length;
  const versionsOf = (file: TitleFile) => files.filter(other => (other.partIndex ?? 0) === (file.partIndex ?? 0)).length;
  return (
    <div className={s.files}>
      {heading ? <Text variant="label" tone="tertiary">{t('title.files')}</Text> : null}
      {files.map((file, i) => {
        const lines = titleFileLines(file, t, i18n.languageName);
        const rows: [string, React.ReactNode][] = [];
        if (lines.audio.length) rows.push([t('mediaInfo.audio'), <span className={s.fileLines}>{lines.audio.map((line, n) => <span key={n}>{line}</span>)}</span>]);
        if (lines.subtitles.length) rows.push([t('mediaInfo.subtitles'), <span className={s.fileLines}>{lines.subtitles.map((line, n) => <span key={n}>{line}</span>)}</span>]);
        if (lines.path) rows.push([t('mediaInfo.location'), <span className={s.filePath}>{lines.path}</span>]);
        return (
          <div key={file.id} className={s.file}>
            <div className={s.fileHead}>
              {parts && (file.partIndex ?? 0) > 0 ? <span className={s.fileVersion}>{t('mediaInfo.part', {number: file.partIndex ?? 0})}</span> : null}
              {versionsOf(file) > 1 ? <span className={s.fileVersion}>{t('mediaInfo.version', {number: versionOf(i)})}</span> : null}
              <strong>{lines.summary}</strong>
              {file.edition ? <span>{file.edition}</span> : null}
              {lines.available ? null : <span className={s.fileMissing}>{t('mediaInfo.unavailable')}</span>}
              {(onSplit && lines.available && files.length > 1) || (onSplitFolder && lines.available) || onDelete ? <span className={s.fileAction}>{onSplit && lines.available && files.length > 1 ? <Button size="sm" variant="ghost" label={t('title.split.action')} onClick={() => onSplit(file, lines.summary)} /> : null}{onSplitFolder && lines.available ? <Button size="sm" variant="ghost" label={t(folderKind === 'book' ? 'title.splitFolder.actionBook' : 'title.splitFolder.action')} onClick={() => onSplitFolder(file, lines.summary)} /> : null}{onDelete ? <Button size="sm" variant="ghost" icon="trash" label={t('title.deleteVersion.action')} onClick={() => onDelete(file, lines.summary)} /> : null}</span> : null}
            </div>
            {lines.facts.length ? <div className={s.fileFacts}>{lines.facts.map(fact => <span key={fact}>{fact}</span>)}</div> : null}
            {rows.length ? <KeyValue rows={rows} /> : null}
          </div>
        );
      })}
    </div>
  );
}
