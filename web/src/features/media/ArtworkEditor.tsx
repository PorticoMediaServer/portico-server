import type { MediaImage } from '@porticomediaserver/client-core';
import { NavigationBackIcon, NavigationForwardIcon, ActionConfirmIcon, StatusArtworkUnavailableIcon, ActionAddIcon, ActionDeleteIcon } from '#portico-icons';
import { useMemo, useRef, useState } from 'react';
import { IconButton, SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';

const artworkTypes = ['poster', 'backdrop', 'thumb', 'logo', 'banner', 'disc', 'clearart'] as const;
type ArtworkType = (typeof artworkTypes)[number];

const artworkLabels: Record<ArtworkType, string> = {
  poster: 'Poster',
  backdrop: 'Backdrop',
  thumb: 'Thumbnail',
  logo: 'Logo',
  banner: 'Banner',
  disc: 'Disc',
  clearart: 'Clear art',
};

function normalizedArtworkType(value: string): ArtworkType | undefined {
  return artworkTypes.find((type) => type === value.toLocaleLowerCase());
}

function artworkSource(image: MediaImage, fallbackUrl: string | undefined) {
  return image.preferred ? fallbackUrl : undefined;
}

function artworkOrigin(image: MediaImage) {
  if (image.source === 'manual' && image.provider === 'upload') return 'Uploaded';
  if (image.provider) return image.provider.toLocaleUpperCase();
  if (image.source === 'local') return 'Local media';
  return image.source || 'Artwork';
}

function artworkDetail(image: MediaImage) {
  const dimensions = image.width && image.height ? `${image.width} × ${image.height}` : '';
  return [dimensions, image.language?.toLocaleUpperCase()].filter(Boolean).join(' · ') || 'Dimensions unavailable';
}

export function ArtworkEditor({
  images,
  fallbackUrls,
  onUpload,
  onDelete,
  onPreferred,
  onReorder,
}: {
  images: MediaImage[];
  fallbackUrls: Partial<Record<ArtworkType, string>>;
  onUpload: (type: ArtworkType, file: File) => Promise<void>;
  onDelete: (imageId: string) => Promise<void>;
  onPreferred: (imageId: string) => Promise<void>;
  onReorder: (imageIds: string[]) => Promise<void>;
}) {
  const initialType = normalizedArtworkType(images.find((image) => image.preferred)?.type ?? images[0]?.type ?? '') ?? 'poster';
  const [selectedType, setSelectedType] = useState<ArtworkType>(initialType);
  const [busy, setBusy] = useState('');
  const [confirmingDelete, setConfirmingDelete] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const fileInput = useRef<HTMLInputElement>(null);
  const visibleImages = useMemo(() => images
    .filter((image) => normalizedArtworkType(image.type) === selectedType)
    .sort((left, right) => Number(right.preferred) - Number(left.preferred) || (left.sortOrder ?? 0) - (right.sortOrder ?? 0) || left.id.localeCompare(right.id)), [images, selectedType]);

  const run = async (key: string, success: string, operation: () => Promise<void>) => {
    setBusy(key);
    setError('');
    setNotice('');
    try {
      await operation();
      setNotice(success);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'media.update-failed', { featureName: 'Artwork' }));
    } finally {
      setBusy('');
    }
  };

  const upload = async (file: File | undefined) => {
    if (!file) return;
    if (file.size > 5 * 1024 * 1024) {
      setError('Choose an artwork file smaller than 5 MB.');
      return;
    }
    const supportedType = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'].includes(file.type) || /\.(jpe?g|png|gif|webp)$/i.test(file.name);
    if (!supportedType) {
      setError('Artwork must be a JPEG, PNG, GIF, or WebP image.');
      return;
    }
    await run('upload', `${artworkLabels[selectedType]} uploaded and selected.`, () => onUpload(selectedType, file));
    if (fileInput.current) fileInput.current.value = '';
  };

  const move = async (index: number, offset: -1 | 1) => {
    const destination = index + offset;
    if (destination < 0 || destination >= visibleImages.length || visibleImages[index].preferred || visibleImages[destination].preferred) return;
    const next = [...visibleImages];
    [next[index], next[destination]] = [next[destination], next[index]];
    await run(`order:${visibleImages[index].id}`, `${artworkLabels[selectedType]} order updated.`, () => onReorder(next.map((image) => image.id)));
  };

  return <section className={`artwork-editor artwork-${selectedType}`} aria-label="Artwork editor">
    <div className="artwork-toolbar">
      <div className="artwork-type-tabs" role="tablist" aria-label="Artwork type">
        {artworkTypes.map((type) => {
          const count = images.filter((image) => normalizedArtworkType(image.type) === type).length;
          return <button key={type} type="button" role="tab" aria-selected={selectedType === type} className={selectedType === type ? 'active' : ''} onClick={() => { setSelectedType(type); setError(''); setNotice(''); }}>
            {artworkLabels[type]}{count > 1 && <span>{count}</span>}
          </button>;
        })}
      </div>
      <input ref={fileInput} className="artwork-file-input" type="file" accept="image/jpeg,image/png,image/gif,image/webp" aria-hidden="true" tabIndex={-1} disabled={Boolean(busy)} onChange={(event) => void upload(event.currentTarget.files?.[0])} />
      <SecondaryButton disabled={Boolean(busy)} onClick={() => fileInput.current?.click()}><ActionAddIcon /> {busy === 'upload' ? 'Uploading…' : `Upload ${artworkLabels[selectedType].toLocaleLowerCase()}`}</SecondaryButton>
    </div>

    {(error || notice) && <p className={`artwork-feedback ${error ? 'error' : ''}`} role={error ? 'alert' : 'status'}>{error || notice}</p>}

    {visibleImages.length === 0 ? <div className="artwork-empty">
      <StatusArtworkUnavailableIcon />
      <strong>No {artworkLabels[selectedType].toLocaleLowerCase()} artwork</strong>
      <p>Upload an image to use it for this media item.</p>
      <SecondaryButton disabled={Boolean(busy)} onClick={() => fileInput.current?.click()}><ActionAddIcon /> Choose image</SecondaryButton>
    </div> : <div className="artwork-grid">
      {visibleImages.map((image, index) => {
        const source = artworkSource(image, fallbackUrls[selectedType]);
        const removable = image.source === 'manual' && image.provider === 'upload';
        const working = busy.endsWith(image.id);
        const origin = artworkOrigin(image);
        const canReorder = visibleImages.filter((candidate) => !candidate.preferred).length > 1;
        const hasActions = !image.preferred || removable || canReorder;
        return <article key={image.id} className={`artwork-card ${image.preferred ? 'preferred' : ''}`} aria-busy={working}>
          <div className="artwork-preview">
            {source ? <img src={source} alt="" /> : <span><StatusArtworkUnavailableIcon /><small>Preview unavailable</small></span>}
            {image.preferred && <span className="artwork-current"><ActionConfirmIcon /> Current</span>}
          </div>
          <div className="artwork-card-copy"><strong>{origin}</strong><small>{artworkDetail(image)}</small></div>
          {hasActions && <div className="artwork-card-actions">
            {!image.preferred && <button type="button" aria-label={`Use ${origin} ${artworkLabels[selectedType].toLocaleLowerCase()}`} disabled={Boolean(busy)} onClick={() => void run(`prefer:${image.id}`, `${artworkLabels[selectedType]} selected.`, () => onPreferred(image.id))}>Use this</button>}
            {canReorder && <span className="artwork-order-actions">
              <IconButton label={`Move ${artworkLabels[selectedType].toLocaleLowerCase()} earlier`} disabled={Boolean(busy) || index === 0 || image.preferred || visibleImages[index - 1]?.preferred} onClick={() => void move(index, -1)}><NavigationBackIcon /></IconButton>
              <IconButton label={`Move ${artworkLabels[selectedType].toLocaleLowerCase()} later`} disabled={Boolean(busy) || index === visibleImages.length - 1 || image.preferred} onClick={() => void move(index, 1)}><NavigationForwardIcon /></IconButton>
            </span>}
            {removable && <IconButton label={`Remove ${artworkLabels[selectedType].toLocaleLowerCase()}`} disabled={Boolean(busy)} onClick={() => setConfirmingDelete(image.id)}><ActionDeleteIcon /></IconButton>}
          </div>}
          {confirmingDelete === image.id && <div className="artwork-delete-confirmation">
            <strong>Remove this uploaded {artworkLabels[selectedType].toLocaleLowerCase()}?</strong>
            <span>This uploaded file will be deleted from the server.</span>
            <div><button type="button" onClick={() => setConfirmingDelete('')}>Cancel</button><button type="button" className="danger" disabled={Boolean(busy)} onClick={() => void run(`delete:${image.id}`, 'Artwork removed.', async () => { await onDelete(image.id); setConfirmingDelete(''); })}>Remove</button></div>
          </div>}
        </article>;
      })}
    </div>}
  </section>;
}
