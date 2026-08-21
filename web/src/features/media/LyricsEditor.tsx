import type { LyricSearchCandidate, MediaLyric } from '@porticomediaserver/client-core';
import { CloudDownload, FileMusic, RefreshCw, Search, Trash2, Upload } from '#portico-icons';
import { useRef, useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import './technical-media.css';

function lyricSource(lyric: MediaLyric) {
  if (lyric.source === 'manual' && lyric.provider === 'upload') return 'Uploaded';
  if (lyric.source === 'provider') return lyric.provider?.toLocaleUpperCase() || 'Provider';
  return 'Local file';
}

function canDeleteLyric(lyric: MediaLyric) {
  return (lyric.source === 'manual' && lyric.provider === 'upload')
    || (lyric.source === 'provider' && lyric.provider === 'lrclib');
}

function formatDuration(seconds: number | undefined) {
  if (!seconds) return undefined;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}:${String(Math.round(seconds % 60)).padStart(2, '0')}`;
}

export function LyricsEditor({
  lyrics,
  defaultQuery,
  onUpload,
  onFetch,
  onSearch,
  onApply,
  onDelete,
}: {
  lyrics: MediaLyric[];
  defaultQuery: string;
  onUpload: (file: File, language: string) => Promise<void>;
  onFetch: () => Promise<void>;
  onSearch: (query: string) => Promise<LyricSearchCandidate[]>;
  onApply: (candidate: LyricSearchCandidate) => Promise<void>;
  onDelete: (lyricId: string) => Promise<void>;
}) {
  const [language, setLanguage] = useState('en');
  const [query, setQuery] = useState(defaultQuery);
  const [results, setResults] = useState<LyricSearchCandidate[]>([]);
  const [searched, setSearched] = useState(false);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const fileInput = useRef<HTMLInputElement>(null);

  const run = async (key: string, success: string, operation: () => Promise<void>) => {
    setBusy(key);
    setError('');
    setNotice('');
    try {
      await operation();
      setNotice(success);
      return true;
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'media.update-failed', { featureName: 'Lyrics' }));
      return false;
    } finally {
      setBusy('');
    }
  };

  const upload = async (file: File | undefined) => {
    if (!file) return;
    if (file.size > 512 * 1024) {
      setError('Choose a lyrics file smaller than 512 KB.');
      return;
    }
    if (!/\.(lrc|txt)$/i.test(file.name)) {
      setError('Choose an LRC or TXT lyrics file.');
      return;
    }
    await run('upload', 'Lyrics uploaded.', () => onUpload(file, language.trim() || 'und'));
    if (fileInput.current) fileInput.current.value = '';
  };

  const search = async () => {
    if (!query.trim()) {
      setError('Enter a track or artist to search.');
      return;
    }
    setBusy('search');
    setError('');
    setNotice('');
    setSearched(true);
    try {
      setResults(await onSearch(query.trim()));
    } catch (reason) {
      setResults([]);
      setError(reviewedProductErrorText(reason, 'media.search-failed', { featureName: 'Lyrics' }));
    } finally {
      setBusy('');
    }
  };

  return (
    <section className="lyrics-editor" aria-label="Lyrics editor">
      {(error || notice) && (
        <p className={`technical-feedback ${error ? 'error' : ''}`} role={error ? 'alert' : 'status'}>
          {error || notice}
        </p>
      )}

      <div className="lyrics-editor-toolbar">
        <div className="lyrics-upload-row">
          <input
            ref={fileInput}
            className="technical-file-input"
            type="file"
            aria-hidden="true"
            tabIndex={-1}
            accept=".lrc,.txt"
            disabled={Boolean(busy)}
            onChange={(event) => void upload(event.currentTarget.files?.[0])}
          />
          <label>
            <span>Language</span>
            <input
              value={language}
              maxLength={16}
              disabled={Boolean(busy)}
              onChange={(event) => setLanguage(event.target.value)}
            />
          </label>
          <SecondaryButton disabled={Boolean(busy)} onClick={() => fileInput.current?.click()}>
            <Upload />{busy === 'upload' ? 'Uploading…' : 'Upload file'}
          </SecondaryButton>
        </div>
        <span className="lyrics-fetch-button">
          <SecondaryButton
            disabled={Boolean(busy)}
            onClick={() => void run('fetch', 'Lyrics found and applied.', onFetch)}
          >
            {busy === 'fetch' ? <RefreshCw className="state-spinner" /> : <CloudDownload />}
            {busy === 'fetch' ? 'Finding…' : 'Find automatically'}
          </SecondaryButton>
        </span>
      </div>

      <section className="lyrics-list">
        <header className="lyrics-section-heading">
          <strong>Current lyrics</strong>
          <small>{lyrics.length} {lyrics.length === 1 ? 'version' : 'versions'}</small>
        </header>
        {lyrics.length ? lyrics.map((lyric) => (
          <div className="lyrics-row" key={lyric.id}>
            <FileMusic />
            <span>
              <strong>{lyric.synced ? 'Synchronized lyrics' : 'Plain lyrics'}</strong>
              <small>{[lyricSource(lyric), lyric.language?.toLocaleUpperCase(), lyric.format.toLocaleUpperCase()].filter(Boolean).join(' · ')}</small>
            </span>
            {canDeleteLyric(lyric) && (
              <IconButton
                label="Remove lyrics"
                disabled={Boolean(busy)}
                onClick={() => void run(`delete:${lyric.id}`, 'Lyrics removed.', () => onDelete(lyric.id))}
              >
                <Trash2 />
              </IconButton>
            )}
          </div>
        )) : (
          <div className="technical-empty compact">
            <strong>No lyrics available</strong>
            <p>Find a match or upload an LRC or TXT file.</p>
          </div>
        )}
      </section>

      <form
        className="lyrics-search-form"
        onSubmit={(event) => {
          event.preventDefault();
          void search();
        }}
      >
        <label>
          <span>Search lyrics</span>
          <input
            value={query}
            disabled={Boolean(busy)}
            placeholder="Track and artist"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <PrimaryButton type="submit" disabled={Boolean(busy)}>
          {busy === 'search' ? <RefreshCw className="state-spinner" /> : <Search />}
          {busy === 'search' ? 'Searching…' : 'Search'}
        </PrimaryButton>
      </form>

      {(results.length > 0 || searched) && (
        <section className="lyrics-search-results">
          <header className="lyrics-section-heading">
            <strong>Matches</strong>
            <small>{results.length} found</small>
          </header>
          {results.length ? results.map((candidate) => (
            <div className="lyrics-result-row" key={`${candidate.provider}:${candidate.externalId}`}>
              <FileMusic />
              <span>
                <strong>{candidate.trackName}</strong>
                <small>
                  {[
                    candidate.artistName,
                    candidate.albumName,
                    candidate.synced ? 'Synchronized' : 'Plain',
                    candidate.instrumental ? 'Instrumental' : undefined,
                    formatDuration(candidate.durationSeconds),
                  ].filter(Boolean).join(' · ')}
                </small>
              </span>
              <SecondaryButton
                disabled={Boolean(busy)}
                onClick={() => void run(
                  `apply:${candidate.externalId}`,
                  'Lyrics applied.',
                  () => onApply(candidate),
                )}
              >
                Use lyrics
              </SecondaryButton>
            </div>
          )) : (
            <div className="technical-empty compact">
              <strong>No matches found</strong>
              <p>Try the track title with the artist name.</p>
            </div>
          )}
        </section>
      )}
    </section>
  );
}
