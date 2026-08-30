import { StatusWarningIcon } from '#portico-icons';
import { productMessage, type MediaViewModel } from '@porticomediaserver/client-core';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import type { MediaItem } from '../../data/models';
import { MediaMetadataEditor } from '../media/MediaActionDialogs';
import { actionPresentation, MediaActionIcon, useMediaActionPresentations } from '../media/MediaActionPresentation';
import { detailLibraryDestination } from './detailModel';

function availabilityCopy(availability: NonNullable<MediaViewModel['availability']>) {
  const files = availability.fileCount ?? 0;
  const missing = availability.missingFileCount ?? 0;
  const available = Math.max(0, files - missing);
  if (availability.status === 'partial') {
    if (files > 0 && missing > 0) return productMessage(files === 1 ? 'media.availability-partial-count-single' : 'media.availability-partial-count-plural', { available, files }).text ?? '';
    return productMessage('media.availability-partial-generic').text ?? '';
  }
  if (files > 0) return productMessage(files === 1 ? 'media.availability-unreachable-count-single' : 'media.availability-unreachable-count-plural', { missing: missing || files, files }).text ?? '';
  return productMessage('media.availability-unreachable').text ?? '';
}

export function AvailabilityNotice({ item, availability, onMetadataChange }: { item: MediaItem; availability?: MediaViewModel['availability']; onMetadataChange: () => void }) {
  const [editingMetadata, setEditingMetadata] = useState(false);
  const presentedActions = useMediaActionPresentations(item.actions ?? []);
  const editMetadata = actionPresentation(presentedActions, 'metadata.edit');
  if (!availability || availability.status === 'available') return null;
  const partial = availability.status === 'partial';
  const library = detailLibraryDestination(item);
  return <>
    <div className={`portico-detail-availability ${partial ? 'partial' : 'unavailable'}`} role="status">
      <StatusWarningIcon />
      <span>
        <strong>{productMessage(partial ? 'media.availability-partial-title' : 'media.availability-unavailable-title').text}</strong>
        <small>{availabilityCopy(availability)}</small>
      </span>
      <div className="portico-detail-availability-actions">
        <Link className="button secondary" to={library.path}>{productMessage('action.open-destination', { destination: library.label }).text}</Link>
        {editMetadata && <SecondaryButton onClick={() => setEditingMetadata(true)}><MediaActionIcon action={editMetadata} /> {editMetadata.label}</SecondaryButton>}
      </div>
    </div>
    {editingMetadata && <MediaMetadataEditor mediaIds={[item.id]} initialItems={[item]} onDismiss={() => setEditingMetadata(false)} onSaved={onMetadataChange} />}
  </>;
}
