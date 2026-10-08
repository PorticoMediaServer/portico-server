import type {TitleFile} from '@core/detail.ts';
import {titleFileLines} from '@core/presentation/index.ts';
import {currentI18n} from '../../app/i18n';
import {KeyValue, Text} from '../../ui';
import s from './Detail.module.css';

/**
 * The Files list that closes a title page and an episode panel (Spec — Page Content §0.3): per
 * file, what it is ("4K · HEVC · Dolby Vision · MKV"), its size and bitrate, every audio and
 * subtitle track, and where it lives (owners only). The lines come from the shared model.
 */
export function TitleFiles({files}: {files: readonly TitleFile[]}) {
  const t = currentI18n().t;
  if (!files.length) return null;
  return (
    <div className={s.files}>
      <Text variant="label" tone="tertiary">{t('title.files')}</Text>
      {files.map((file, i) => {
        const lines = titleFileLines(file, t, languageName);
        const rows: [string, React.ReactNode][] = [];
        if (lines.audio.length) rows.push([t('mediaInfo.audio'), <span className={s.fileLines}>{lines.audio.map((line, n) => <span key={n}>{line}</span>)}</span>]);
        if (lines.subtitles.length) rows.push([t('mediaInfo.subtitles'), <span className={s.fileLines}>{lines.subtitles.map((line, n) => <span key={n}>{line}</span>)}</span>]);
        if (lines.path) rows.push([t('mediaInfo.location'), <span className={s.filePath}>{lines.path}</span>]);
        return (
          <div key={file.id} className={s.file}>
            <div className={s.fileHead}>
              {files.length > 1 ? <span className={s.fileVersion}>{t('mediaInfo.version', {number: i + 1})}</span> : null}
              <strong>{lines.summary}</strong>
              {lines.available ? null : <span className={s.fileMissing}>{t('mediaInfo.unavailable')}</span>}
            </div>
            {lines.facts.length ? <div className={s.fileFacts}>{lines.facts.map(fact => <span key={fact}>{fact}</span>)}</div> : null}
            {rows.length ? <KeyValue rows={rows} /> : null}
          </div>
        );
      })}
    </div>
  );
}

/** "en" → "English", in the viewer's language; an unknown code stays as it is. */
export function languageName(code: string): string {
  try { return new Intl.DisplayNames(undefined, {type: 'language'}).of(code) ?? code; } catch { return code; }
}
