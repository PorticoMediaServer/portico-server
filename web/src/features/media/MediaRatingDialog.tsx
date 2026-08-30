import { StatusWarningIcon, ActionRateIcon, ActionCloseIcon } from '#portico-icons';
import { useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import './media-rating.css';

export function MediaRatingDialog({
  title,
  value,
  onDismiss,
  onSave,
}: {
  title: string;
  value: number;
  onDismiss: () => void;
  onSave: (rating: number) => Promise<void>;
}) {
  const [selected, setSelected] = useState(value);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const headingId = 'media-rating-dialog-title';
  const save = async (rating = selected) => {
    setBusy(true);
    setError('');
    try {
      await onSave(rating);
      onDismiss();
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'media.update-failed', { featureName: 'Your rating' }));
    } finally {
      setBusy(false);
    }
  };

  return <ModalOverlay labelledBy={headingId} className="media-rating-dialog" onDismiss={() => { if (!busy) onDismiss(); }}>
    <header>
      <div><p>Your rating</p><h2 id={headingId}>Rate {title}</h2></div>
      <IconButton label="Close rating dialog" disabled={busy} onClick={onDismiss}><ActionCloseIcon /></IconButton>
    </header>
    <div className="media-rating-body">
      <div className="media-rating-score" aria-live="polite"><ActionRateIcon fill={selected ? 'currentColor' : 'none'} /><strong>{selected || '—'}</strong><span>out of 10</span></div>
      <div className="media-rating-options" role="radiogroup" aria-label={`Rating for ${title}`}>
        {Array.from({ length: 10 }, (_, index) => index + 1).map((rating) => <button
          type="button"
          role="radio"
          aria-checked={selected === rating}
          className={selected === rating ? 'selected' : ''}
          disabled={busy}
          key={rating}
          onClick={() => setSelected(rating)}
        >{rating}</button>)}
      </div>
      <p>Ratings help Portico tune recommendations for this account.</p>
      {error && <p className="media-rating-error" role="alert"><StatusWarningIcon /> {error}</p>}
    </div>
    <footer>
      {value > 0 && <button type="button" className="media-rating-clear" disabled={busy} onClick={() => void save(0)}>Clear rating</button>}
      <SecondaryButton disabled={busy} onClick={onDismiss}>Cancel</SecondaryButton>
      <PrimaryButton disabled={busy || selected < 1} onClick={() => void save()}>{busy ? 'Saving…' : 'Save rating'}</PrimaryButton>
    </footer>
  </ModalOverlay>;
}
