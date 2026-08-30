import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem, PorticoDataSource } from '../../data/models';
import { MediaMetadataEditor } from './MediaActionDialogs';

function editableMedia(): MediaItem {
  return {
    id: 'metadata-lock-fixture',
    title: 'Metadata Lock Fixture',
    subtitle: '',
    year: 2026,
    entityKind: 'movie',
    poster: '/poster.jpg',
    backdrop: '/backdrop.jpg',
    rating: 'PG',
    length: '90m',
    genre: 'Drama',
    tags: ['Featured'],
    labels: ['Family'],
    availability: 'available',
    actions: ['metadata.edit'],
    people: [{ name: 'Ari Vega', role: 'Actor', character: 'Captain Sol' }],
    mediaImages: [],
    lockedFields: [],
    metadataRevision: 7,
    metadataEtag: 'metadata-lock-fixture-r7',
  };
}

function editableEpisode(): MediaItem {
  return {
    ...editableMedia(),
    id: 'metadata-episode-fixture',
    title: 'Survive the Streets',
    entityKind: 'episode',
    durationSeconds: 2520,
    communityRating: 8.4,
    criticRating: 92,
    seasonNumber: 8,
    episodeNumber: 15,
    typedMetadata: { productionCode: '815', originalAirDate: '2026-05-12' },
  };
}

describe('MediaMetadataEditor metadata locks', () => {
  it.each([
    ['track', 'Track'],
    ['book', 'Audiobook'],
    ['unsupported', 'Unsupported media'],
  ] as const)('uses canonical, neutral context for %s metadata', async (entityKind, label) => {
    const source = new FixturePorticoDataSource();
    const item = { ...editableMedia(), id: `metadata-${entityKind}`, entityKind, libraryName: 'My Library' };
    vi.spyOn(source as PorticoDataSource, 'media').mockResolvedValue(item);
    render(<DataProvider source={source}><MediaMetadataEditor mediaIds={[item.id]} initialItems={[item]} onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Edit metadata' });
    expect(within(dialog).getByText(`My Library / ${label}`)).toBeInTheDocument();
  });

  it('shows accepted identities and current normalized catalog evidence without raw provider payloads', async () => {
    const source = new FixturePorticoDataSource();
    const item: MediaItem = {
      ...editableMedia(),
      libraryName: 'Cinema',
      providerIds: [{ provider: 'tmdb', externalType: 'movie', externalId: '42', confidence: 1, source: 'provider' }],
      metadataEvidence: {
        revision: 7,
        values: [{ field: 'alternateTitle', order: 0, locale: 'fr', value: 'Le Portique', sourceKind: 'provider', provider: 'tmdb', confidence: 1, decision: 'accepted' }],
        relationships: [{ type: 'keyword', name: 'harbour', order: 0, sourceKind: 'provider', provider: 'tmdb', confidence: 1, decision: 'accepted' }],
      },
    };
    vi.spyOn(source as PorticoDataSource, 'media').mockResolvedValue(item);
    render(<DataProvider source={source}><MediaMetadataEditor mediaIds={[item.id]} initialItems={[item]} onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Edit metadata' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Matching' }));
    expect(within(dialog).getByRole('region', { name: 'Current metadata sources' })).toHaveTextContent('TMDB');
    expect(within(dialog).getByRole('region', { name: 'Current metadata sources' })).toHaveTextContent('movie · 42');
    expect(within(dialog).getByRole('region', { name: 'Current metadata sources' })).toHaveTextContent('Le Portique');
    expect(within(dialog).getByRole('region', { name: 'Current metadata sources' })).toHaveTextContent('harbour');
  });

  it('links TheTVDB attribution alongside a TheTVDB-derived identity', async () => {
    const source = new FixturePorticoDataSource();
    const item: MediaItem = { ...editableMedia(), providerIds: [{ provider: 'tvdb', externalType: 'movie', externalId: '42', confidence: 1, source: 'provider' }] };
    vi.spyOn(source as PorticoDataSource, 'media').mockResolvedValue(item);
    render(<DataProvider source={source}><MediaMetadataEditor mediaIds={[item.id]} initialItems={[item]} onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Edit metadata' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Matching' }));
    expect(within(dialog).getByRole('link', { name: 'TheTVDB' })).toHaveAttribute('href', 'https://thetvdb.com/');
  });

  it('saves artwork, tag-family, and cast locks through the canonical metadata patch', async () => {
    const source = new FixturePorticoDataSource();
    const item = editableMedia();
    vi.spyOn(source as PorticoDataSource, 'media').mockResolvedValue(item);
    const update = vi.spyOn(source as PorticoDataSource, 'updateMediaMetadata').mockResolvedValue([item]);
    render(<DataProvider source={source}><MediaMetadataEditor mediaIds={[item.id]} initialItems={[item]} onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Edit metadata' });
    expect(within(dialog).queryByRole('button', { name: 'Advanced' })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole('heading', { name: 'Locked fields' })).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Artwork' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Lock Artwork' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Tags' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Lock Genres' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Lock Tags' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Lock Labels' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cast & Crew' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Lock Cast & Crew' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      [item.id],
      expect.objectContaining({ lockedFields: ['artwork', 'genres', 'tags', 'labels', 'people'] }),
      expect.any(AbortSignal),
    ));
  });

  it('normalizes edited cast and crew before saving', async () => {
    const source = new FixturePorticoDataSource();
    const item = editableMedia();
    vi.spyOn(source as PorticoDataSource, 'media').mockResolvedValue(item);
    const update = vi.spyOn(source as PorticoDataSource, 'updateMediaMetadata').mockResolvedValue([item]);
    render(<DataProvider source={source}><MediaMetadataEditor mediaIds={[item.id]} initialItems={[item]} onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Edit metadata' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cast & Crew' }));
    fireEvent.change(within(dialog).getByDisplayValue('Ari Vega'), { target: { value: '  Ari Vega  ' } });
    fireEvent.change(within(dialog).getByDisplayValue('Actor'), { target: { value: ' Director ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      [item.id],
      expect.objectContaining({ people: [expect.objectContaining({ name: 'Ari Vega', role: 'Director', sortOrder: 0 })] }),
      expect.any(AbortSignal),
    ));
  });

  it('keeps measured duration out of descriptive edits and locks changed metadata by default', async () => {
    const source = new FixturePorticoDataSource();
    const item = editableEpisode();
    vi.spyOn(source as PorticoDataSource, 'media').mockResolvedValue(item);
    const update = vi.spyOn(source as PorticoDataSource, 'updateMediaMetadata').mockResolvedValue([item]);
    render(<DataProvider source={source}><MediaMetadataEditor mediaIds={[item.id]} initialItems={[item]} onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Edit metadata' });
    expect(within(dialog).queryByDisplayValue('2520')).not.toBeInTheDocument();
    fireEvent.change(within(dialog).getByDisplayValue('8.4'), { target: { value: '8.7' } });
    fireEvent.change(within(dialog).getByDisplayValue('92'), { target: { value: '94' } });
    fireEvent.change(within(dialog).getByDisplayValue('8'), { target: { value: '9' } });
    fireEvent.change(within(dialog).getByDisplayValue('15'), { target: { value: '1' } });
    fireEvent.change(within(dialog).getByDisplayValue('815'), { target: { value: '901' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      [item.id],
      expect.objectContaining({
        expectedRevision: 7,
        communityRating: 8.7,
        criticRating: 94,
        seasonNumber: 9,
        episodeNumber: 1,
        typedMetadata: { productionCode: '901', originalAirDate: '2026-05-12' },
        lockedFields: ['communityRating', 'criticRating', 'seasonNumber', 'episodeNumber', 'typedMetadata'],
      }),
      expect.any(AbortSignal),
    ));
  });
});
