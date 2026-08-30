import type { MediaAttachment } from '@porticomediaserver/client-core';
import { ActionDownloadIcon, ViewDetailsIcon, MediaMovieIcon, PlaybackQualityIcon, DeviceStorageIcon, MediaAudiobookIcon, StatusLoadingIcon, ActionAddIcon, ActionRefreshIcon, ActionDeleteIcon, ActionSendIcon } from '#portico-icons';
import { useEffect, useRef, useState } from 'react';
import { IconButton, SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import type {
  MediaDownloadOptions,
  MediaItem,
  MediaOptimizedVersion,
  MediaStream,
} from '../../data/models';
import './technical-media.css';

type OptionsState =
  | { status: 'loading' }
  | { status: 'error'; error: string }
  | { status: 'ready'; data: MediaDownloadOptions };

function formatBytes(value: number | undefined) {
  if (!value || value < 1) return 'Size unavailable';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount >= 10 || unit === 0 ? Math.round(amount) : amount.toFixed(1)} ${units[unit]}`;
}

function formatBitrate(value: number | undefined) {
  if (!value) return undefined;
  return value >= 1_000_000
    ? `${(value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1)} Mbps`
    : `${Math.round(value / 1000)} kbps`;
}

function formatDuration(value: number | undefined) {
  if (!value || value < 1) return 'Duration unavailable';
  const hours = Math.floor(value / 3600);
  const minutes = Math.floor((value % 3600) / 60);
  const seconds = Math.floor(value % 60);
  return [hours || undefined, String(minutes).padStart(hours ? 2 : 1, '0'), String(seconds).padStart(2, '0')]
    .filter((part) => part !== undefined)
    .join(':');
}

function streamFacts(stream: MediaStream) {
  if (stream.kind === 'video') {
    return [
      stream.width && stream.height ? `${stream.width} × ${stream.height}` : undefined,
      stream.aspectRatio ? `${stream.aspectRatio} aspect` : undefined,
      stream.frameRate ? `${stream.frameRate} fps` : undefined,
      stream.profile ? `Profile ${stream.profile}` : undefined,
      stream.level ? `Level ${stream.level}` : undefined,
      stream.pixelFormat,
      stream.dynamicRange,
      stream.dolbyVisionProfile ? `Dolby Vision ${stream.dolbyVisionProfile}` : undefined,
      stream.bitDepth ? `${stream.bitDepth}-bit` : undefined,
      stream.colorSpace,
      stream.colorPrimaries,
      stream.colorTransfer,
      stream.chromaLocation,
      stream.fieldOrder,
      formatBitrate(stream.bitrate),
      stream.index !== undefined ? `Stream ${stream.index}` : undefined,
    ];
  }
  if (stream.kind === 'audio') {
    return [
      stream.channels ? `${stream.channels} channels` : undefined,
      stream.channelLayout,
      stream.sampleRate ? `${(stream.sampleRate / 1000).toFixed(stream.sampleRate % 1000 === 0 ? 0 : 1)} kHz` : undefined,
      stream.language?.toLocaleUpperCase(),
      formatBitrate(stream.bitrate),
      stream.index !== undefined ? `Stream ${stream.index}` : undefined,
    ];
  }
  return [
    stream.language?.toLocaleUpperCase(),
    stream.default ? 'Default' : undefined,
    stream.forced ? 'Forced' : undefined,
    stream.hearingImpaired ? 'SDH/CC' : undefined,
    stream.index !== undefined ? `Stream ${stream.index}` : undefined,
    stream.subtitleOffsetMs
      ? `${stream.subtitleOffsetMs > 0 ? '+' : ''}${stream.subtitleOffsetMs} ms`
      : undefined,
  ];
}

function SubtitleRow({
  stream,
  busy,
  onOffset,
  onDelete,
}: {
  stream: MediaStream;
  busy: boolean;
  onOffset: (offsetMs: number) => Promise<boolean>;
  onDelete: () => Promise<boolean>;
}) {
  const managed = Boolean(stream.sourceUrl);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [offset, setOffset] = useState(String(stream.subtitleOffsetMs ?? 0));

  return (
    <div className="technical-stream-row subtitle-row">
      <span className="technical-kind"><ViewDetailsIcon /> Subtitle</span>
      <span className="technical-copy">
        <strong>{stream.displayTitle || 'Subtitle'}</strong>
        <small>{[stream.codec.toLocaleUpperCase(), ...streamFacts(stream)].filter(Boolean).join(' · ')}</small>
      </span>
      {managed && !editing && !confirming && (
        <span className="technical-row-actions">
          <button type="button" disabled={busy} onClick={() => setEditing(true)}>Timing</button>
          <IconButton
            label={`Remove ${stream.displayTitle || 'subtitle'}`}
            disabled={busy}
            onClick={() => setConfirming(true)}
          >
            <ActionDeleteIcon />
          </IconButton>
        </span>
      )}
      {!managed && <small className="technical-managed-note">Managed by the media source</small>}
      {editing && (
        <form
          className="subtitle-offset-form"
          onSubmit={(event) => {
            event.preventDefault();
            const value = Number(offset);
            if (!Number.isInteger(value) || value < -300000 || value > 300000) return;
            void onOffset(value).then((updated) => {
              if (updated) setEditing(false);
            });
          }}
        >
          <label>
            <span>Offset (ms)</span>
            <input
              autoFocus
              type="number"
              min={-300000}
              max={300000}
              step={50}
              value={offset}
              disabled={busy}
              onChange={(event) => setOffset(event.target.value)}
            />
          </label>
          <button
            type="button"
            onClick={() => {
              setOffset(String(stream.subtitleOffsetMs ?? 0));
              setEditing(false);
            }}
          >
            Cancel
          </button>
          <button type="submit" disabled={busy}>Save timing</button>
        </form>
      )}
      {confirming && (
        <div className="technical-inline-confirm">
          <span>Remove {stream.displayTitle || 'this uploaded subtitle'}?</span>
          <button type="button" onClick={() => setConfirming(false)}>Cancel</button>
          <button
            type="button"
            className="danger"
            disabled={busy}
            onClick={() => void onDelete().then((deleted) => {
              if (deleted) setConfirming(false);
            })}
          >
            Remove
          </button>
        </div>
      )}
    </div>
  );
}

export function TechnicalMediaEditor({
  item,
  attachments = [],
  loadOptions,
  onUploadSubtitle,
  onUpdateSubtitle,
  onDeleteSubtitle,
  onCreateVersion,
  onDeleteVersion,
}: {
  item: MediaItem;
  attachments?: MediaAttachment[];
  loadOptions: () => Promise<MediaDownloadOptions>;
  onUploadSubtitle: (file: File, language: string, label: string) => Promise<void>;
  onUpdateSubtitle: (streamId: string, offsetMs: number) => Promise<void>;
  onDeleteSubtitle: (streamId: string) => Promise<void>;
  onCreateVersion: (profile: string) => Promise<void>;
  onDeleteVersion: (profile: string) => Promise<void>;
}) {
  const [optionsRevision, setOptionsRevision] = useState(0);
  const [options, setOptions] = useState<OptionsState>({ status: 'loading' });
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [subtitleLanguage, setSubtitleLanguage] = useState('en');
  const [subtitleLabel, setSubtitleLabel] = useState('');
  const [selectedProfile, setSelectedProfile] = useState('');
  const subtitleInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let current = true;
    setOptions({ status: 'loading' });
    loadOptions().then(
      (data) => current && setOptions({ status: 'ready', data }),
      (reason: unknown) => current && setOptions({
        status: 'error',
        error: reviewedProductErrorText(reason, 'media.load-failed', { featureName: 'Media options' }),
      }),
    );
    return () => { current = false; };
  }, [loadOptions, optionsRevision]);

  const run = async (
    key: string,
    success: string,
    operation: () => Promise<void>,
    reloadOptions = false,
  ) => {
    setBusy(key);
    setError('');
    setNotice('');
    try {
      await operation();
      setNotice(success);
      if (reloadOptions) setOptionsRevision((value) => value + 1);
      return true;
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'media.update-failed', { featureName: 'Media' }));
      return false;
    } finally {
      setBusy('');
    }
  };

  const uploadSubtitle = async (file: File | undefined) => {
    if (!file) return;
    if (file.size > 2 * 1024 * 1024) {
      setError('Choose a subtitle file smaller than 2 MB.');
      return;
    }
    if (!/\.(vtt|srt|ass|ssa|sbv|sub|ttml|dfxp)$/i.test(file.name)) {
      setError('Choose a VTT, SRT, ASS, SSA, SBV, SUB, TTML, or DFXP subtitle file.');
      return;
    }
    const uploaded = await run(
      'subtitle-upload',
      'Subtitle uploaded.',
      () => onUploadSubtitle(file, subtitleLanguage.trim() || 'und', subtitleLabel.trim()),
    );
    if (uploaded) setSubtitleLabel('');
    if (subtitleInput.current) subtitleInput.current.value = '';
  };

  const streams = [...(item.streams ?? [])].sort(
    (left, right) => ['video', 'audio', 'subtitle'].indexOf(left.kind)
      - ['video', 'audio', 'subtitle'].indexOf(right.kind),
  );
  const fileStreams = (item.mediaFiles ?? []).flatMap((file) => file.streams ?? []);
  const analyzedStreams = [...new Map([...streams, ...fileStreams].map((stream) => [stream.id, stream])).values()];
  const primaryStreams = analyzedStreams.filter((stream) => stream.kind !== 'subtitle');
  const subtitles = streams.filter((stream) => stream.kind === 'subtitle');
  const canOptimize = item.actions?.includes('media.optimize') ?? false;
  const readyOptions = options.status === 'ready' ? options.data : undefined;
  const sourceOption = readyOptions?.options.find((option) => option.kind === 'source');
  const sourceFiles = item.mediaFiles ?? [];
  const versions = readyOptions?.optimizedVersions ?? item.optimizedVersions ?? [];
  const configuredProfiles = readyOptions?.profiles.slice(0, 5) ?? [];
  const selectedOptimizationProfile = configuredProfiles.find((profile) => profile.id === selectedProfile)
    ?? configuredProfiles[0];
  const availability = item.availability === 'unavailable'
    ? 'Unavailable'
    : item.availability === 'partial'
      ? 'Partially available'
      : 'Available';

  return (
    <section className="technical-media-editor" aria-label="Technical media editor">
      {(error || notice) && (
        <p className={`technical-feedback ${error ? 'error' : ''}`} role={error ? 'alert' : 'status'}>
          {error || notice}
        </p>
      )}

      <section className="technical-section source-summary">
        <header>
          <span><MediaMovieIcon /><strong>Source media</strong></span>
          <small className={item.availability === 'unavailable' ? 'unavailable' : ''}>{availability}</small>
        </header>
        <div className="source-facts">
          <span>
            <small>Files</small>
            <strong>{item.fileCount ?? '—'}</strong>
            {Boolean(item.missingFileCount) && <em>{item.missingFileCount} missing</em>}
          </span>
          {options.status === 'loading' && (
            <span className="technical-loading"><StatusLoadingIcon /> Loading source details</span>
          )}
          {options.status === 'error' && (
            <span className="technical-load-error">
              <strong>Source details unavailable</strong>
              <button type="button" onClick={() => setOptionsRevision((value) => value + 1)}>
                <ActionRefreshIcon /> Retry
              </button>
            </span>
          )}
          {options.status === 'ready' && (
            <>
              <span><small>Storage</small><strong>{sourceOption?.sourceKind || 'Unknown'}</strong></span>
              <span>
                <small>Format</small>
                <strong>
                  {[
                    sourceOption?.container?.toLocaleUpperCase(),
                    sourceOption?.videoCodec?.toLocaleUpperCase(),
                    sourceOption?.audioCodec?.toLocaleUpperCase(),
                  ].filter(Boolean).join(' · ') || 'Not analyzed'}
                </strong>
              </span>
              <span><small>Size</small><strong>{formatBytes(sourceOption?.sizeBytes)}</strong></span>
            </>
          )}
        </div>
      </section>

      <section className="technical-section">
        <header>
          <span><DeviceStorageIcon /><strong>Files and versions</strong></span>
          <small>{sourceFiles.length} {sourceFiles.length === 1 ? 'file' : 'files'}</small>
        </header>
        {sourceFiles.length ? (
          <div className="technical-file-list">
            {sourceFiles.map((file, index) => (
              <details key={file.id} open={sourceFiles.length === 1 || file.selected}>
                <summary>
                  <span>
                    <strong>{file.versionLabel || file.originalFilename || `Source ${index + 1}`}</strong>
                    <small>{[
                      file.container?.toLocaleUpperCase(),
                      file.resolution,
                      file.videoCodec?.toLocaleUpperCase(),
                      file.audioCodec?.toLocaleUpperCase(),
                      formatBytes(file.sizeBytes),
                    ].filter(Boolean).join(' · ')}</small>
                  </span>
                  <em className={file.available ? 'available' : 'unavailable'}>
                    {file.available ? file.selected ? 'Selected' : 'Available' : 'Unavailable'}
                  </em>
                </summary>
                <dl>
                  {file.path && <><dt>Path</dt><dd title={file.path}>{file.path}</dd></>}
                  {file.originalFilename && <><dt>File name</dt><dd>{file.originalFilename}</dd></>}
                  <dt>Container</dt><dd>{file.container?.toLocaleUpperCase() || 'Unknown'}</dd>
                  <dt>Size</dt><dd>{formatBytes(file.sizeBytes)}</dd>
                  <dt>Duration</dt><dd>{formatDuration(file.durationSeconds || item.durationSeconds)}</dd>
                  <dt>Overall bitrate</dt><dd>{formatBitrate(file.bitrate) || 'Not analyzed'}</dd>
                  <dt>Video</dt><dd>{[
                    file.videoCodec?.toLocaleUpperCase(),
                    file.width && file.height ? `${file.width} × ${file.height}` : file.resolution,
                    file.aspectRatio ? `${file.aspectRatio} aspect` : undefined,
                    file.frameRate ? `${file.frameRate} fps` : undefined,
                    file.videoProfile ? `Profile ${file.videoProfile}` : undefined,
                    file.videoLevel ? `Level ${file.videoLevel}` : undefined,
                    file.bitDepth ? `${file.bitDepth}-bit` : undefined,
                    file.pixelFormat,
                    file.dynamicRange,
                  ].filter(Boolean).join(' · ') || 'Not present or not analyzed'}</dd>
                  <dt>Color</dt><dd>{[
                    file.colorSpace,
                    file.colorPrimaries,
                    file.colorTransfer,
                    file.chromaLocation,
                  ].filter(Boolean).join(' · ') || 'Not reported'}</dd>
                  <dt>Audio</dt><dd>{[
                    file.audioCodec?.toLocaleUpperCase(),
                    file.audioChannels ? `${file.audioChannels} channels` : undefined,
                    file.audioChannelLayout,
                    file.audioSampleRate ? `${file.audioSampleRate / 1000} kHz` : undefined,
                    formatBitrate(file.audioBitrate),
                  ].filter(Boolean).join(' · ') || 'Not present or not analyzed'}</dd>
                  {file.sourceType && <><dt>Source</dt><dd>{[file.sourceType, file.source].filter(Boolean).join(' · ')}</dd></>}
                  {file.streamAnalysisStatus && <><dt>Analysis</dt><dd>{file.streamAnalysisStatus}</dd></>}
                  {file.releaseGroup && <><dt>Release group</dt><dd>{file.releaseGroup}</dd></>}
                  {file.missingSince && <><dt>Missing since</dt><dd>{new Date(file.missingSince).toLocaleString()}</dd></>}
                </dl>
                {Boolean(file.streams?.length) && (
                  <div className="technical-stream-list file-streams" aria-label={`Streams in ${file.originalFilename || `source ${index + 1}`}`}>
                    {file.streams?.map((stream) => (
                      <div className="technical-stream-row" key={stream.id}>
                        <span className="technical-kind">
                          {stream.kind === 'video' ? <MediaMovieIcon /> : <MediaAudiobookIcon />}
                          {stream.kind === 'subtitle' ? 'Subtitle' : stream.kind === 'video' ? 'Video' : 'Audio'}
                        </span>
                        <span className="technical-copy">
                          <strong>{stream.displayTitle || stream.codec.toLocaleUpperCase()}</strong>
                          <small>{[stream.codec.toLocaleUpperCase(), ...streamFacts(stream)].filter(Boolean).join(' · ')}</small>
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </details>
            ))}
          </div>
        ) : (
          <div className="technical-empty compact">
            <strong>No indexed source files</strong>
            <p>This item may be remote, missing, or still being analyzed.</p>
          </div>
        )}
      </section>

      <section className="technical-section">
        <header>
          <span><PlaybackQualityIcon /><strong>Streams</strong></span>
          <small>{primaryStreams.length} {primaryStreams.length === 1 ? 'stream' : 'streams'}</small>
        </header>
        {primaryStreams.length ? (
          <div className="technical-stream-list">
            {primaryStreams.map((stream) => (
              <div className="technical-stream-row" key={stream.id}>
                <span className="technical-kind">
                  {stream.kind === 'video' ? <MediaMovieIcon /> : <MediaAudiobookIcon />}
                  {stream.kind === 'video' ? 'Video' : 'Audio'}
                </span>
                <span className="technical-copy">
                  <strong>{stream.displayTitle || stream.codec.toLocaleUpperCase()}</strong>
                  <small>{[stream.codec.toLocaleUpperCase(), ...streamFacts(stream)].filter(Boolean).join(' · ')}</small>
                </span>
              </div>
            ))}
          </div>
        ) : (
          <div className="technical-empty">
            <strong>No analyzed streams</strong>
            <p>Run media analysis to populate codec and stream details.</p>
          </div>
        )}
      </section>

      <section className="technical-section">
        <header>
          <span><ViewDetailsIcon /><strong>Subtitles</strong></span>
          <small>{subtitles.length} {subtitles.length === 1 ? 'track' : 'tracks'}</small>
        </header>
        <div className="subtitle-upload-row">
          <input
            ref={subtitleInput}
            className="technical-file-input"
            type="file"
            aria-hidden="true"
            tabIndex={-1}
            accept=".vtt,.srt,.ass,.ssa,.sbv,.sub,.ttml,.dfxp"
            disabled={Boolean(busy)}
            onChange={(event) => void uploadSubtitle(event.currentTarget.files?.[0])}
          />
          <label>
            <span>Language</span>
            <input
              value={subtitleLanguage}
              maxLength={16}
              disabled={Boolean(busy)}
              onChange={(event) => setSubtitleLanguage(event.target.value)}
            />
          </label>
          <label>
            <span>Label</span>
            <input
              value={subtitleLabel}
              maxLength={80}
              placeholder="Optional"
              disabled={Boolean(busy)}
              onChange={(event) => setSubtitleLabel(event.target.value)}
            />
          </label>
          <SecondaryButton disabled={Boolean(busy)} onClick={() => subtitleInput.current?.click()}>
            <ActionSendIcon />{busy === 'subtitle-upload' ? 'Uploading…' : 'Upload subtitle'}
          </SecondaryButton>
        </div>
        {subtitles.length ? (
          <div className="technical-stream-list">
            {subtitles.map((stream) => (
              <SubtitleRow
                key={stream.id}
                stream={stream}
                busy={Boolean(busy)}
                onOffset={(offsetMs) => run(
                  `subtitle-offset:${stream.id}`,
                  'Subtitle timing updated.',
                  () => onUpdateSubtitle(stream.id, offsetMs),
                )}
                onDelete={() => run(
                  `subtitle-delete:${stream.id}`,
                  'Subtitle removed.',
                  () => onDeleteSubtitle(stream.id),
                )}
              />
            ))}
          </div>
        ) : (
          <div className="technical-empty compact">
            <strong>No subtitle tracks</strong>
            <p>Upload a text subtitle file to make it available during playback.</p>
          </div>
        )}
      </section>

      <section className="technical-section">
        <header>
          <span><PlaybackQualityIcon /><strong>Optimized versions</strong></span>
          <small>{versions.length} available</small>
        </header>
        {versions.length > 0 && (
          <div className="technical-version-list">
            {versions.map((version: MediaOptimizedVersion) => (
              <div key={version.id}>
                <span>
                  <strong>{version.profileName || version.profile}</strong>
                  <small>{[
                    formatBytes(version.sizeBytes),
                    version.container?.toLocaleUpperCase(),
                    version.videoCodec?.toLocaleUpperCase(),
                    version.width && version.height ? `${version.width} × ${version.height}` : undefined,
                    formatBitrate(version.bitrate),
                    version.durationSeconds ? formatDuration(version.durationSeconds) : undefined,
                    version.available ? 'Available' : 'Unavailable',
                    version.path,
                    `Updated ${new Date(version.updatedAt).toLocaleDateString()}`,
                  ].filter(Boolean).join(' · ')}</small>
                </span>
                {canOptimize && (
                  <IconButton
                    label={`Remove ${version.profileName || version.profile}`}
                    disabled={Boolean(busy)}
                    onClick={() => void run(
                      `version-delete:${version.id}`,
                      'Optimized version removed.',
                      () => onDeleteVersion(version.profile),
                      true,
                    )}
                  >
                    <ActionDeleteIcon />
                  </IconButton>
                )}
              </div>
            ))}
          </div>
        )}
        {canOptimize && configuredProfiles.length > 0 && (
          <div className="technical-profile-list">
            <label>
              <span>New version</span>
              <select
                value={selectedOptimizationProfile?.id ?? ''}
                disabled={Boolean(busy)}
                onChange={(event) => setSelectedProfile(event.target.value)}
              >
                {configuredProfiles.map((profile) => (
                  <option key={profile.id} value={profile.id}>
                    {profile.label} · {profile.height}p · {profile.videoKbps} kbps
                  </option>
                ))}
              </select>
            </label>
            {selectedOptimizationProfile && (() => {
              const option = readyOptions?.options.find((entry) => entry.profile === selectedOptimizationProfile.id);
              const queued = Boolean(option?.job && ['queued', 'running'].includes(option.job.status));
              const failed = Boolean(option?.job && ['failed', 'cancelled', 'canceled'].includes(option.job.status));
              const existing = versions.find((version) => version.profile === selectedOptimizationProfile.id);
              return <>
                <button
                  type="button"
                  disabled={Boolean(busy) || queued}
                  onClick={() => void run(
                    `version-create:${selectedOptimizationProfile.id}`,
                    `${selectedOptimizationProfile.label} ${existing ? 'regeneration' : 'creation'} queued.`,
                    () => onCreateVersion(selectedOptimizationProfile.id),
                    true,
                  )}
                >
                  {queued ? <StatusLoadingIcon className="state-spinner" /> : failed ? <ActionRefreshIcon /> : <ActionAddIcon />}
                  <span><strong>{queued ? 'Preparing version' : failed ? 'Retry version' : existing ? 'Regenerate version' : 'Create version'}</strong></span>
                </button>
                {option?.job && <small className={`technical-job-status ${failed ? 'failed' : ''}`} role={failed ? 'alert' : 'status'}>
                  {queued
                    ? `${option.job.status === 'running' ? 'Encoding' : 'Queued'}${option.job.progress > 0 ? ` · ${Math.round(option.job.progress)}%` : ''}`
                    : failed
                      ? option.job.lastError || option.job.message || 'The previous encode did not finish.'
                      : option.job.status === 'completed'
                        ? 'Version ready'
                        : option.job.message}
                </small>}
              </>;
            })()}
          </div>
        )}
        {options.status === 'ready' && versions.length === 0 && (!canOptimize || configuredProfiles.length === 0) && (
          <div className="technical-empty compact">
            <strong>No optimized versions</strong>
            <p>No compatible version is currently stored for this item.</p>
          </div>
        )}
      </section>

      {attachments.length > 0 && (
        <section className="technical-section">
          <header>
            <span><ActionDownloadIcon /><strong>Attachments</strong></span>
            <small>{attachments.length} files</small>
          </header>
          <div className="technical-attachment-list">
            {attachments.map((attachment) => {
              const copy = (
                <>
                  <ViewDetailsIcon />
                  <span>
                    <strong>{attachment.filename}</strong>
                    <small>
                      {[attachment.codec?.toLocaleUpperCase(), attachment.mimeType, formatBytes(attachment.sizeBytes)]
                        .filter(Boolean).join(' · ')}
                    </small>
                  </span>
                </>
              );
              return attachment.url ? (
                <a key={attachment.id} href={attachment.url} download>
                  {copy}<ActionDownloadIcon />
                </a>
              ) : (
                <div key={attachment.id}>{copy}<small>Download unavailable</small></div>
              );
            })}
          </div>
        </section>
      )}
    </section>
  );
}
