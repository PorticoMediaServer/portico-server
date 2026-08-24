import { describe, expect, it } from 'vitest';
import { FixturePorticoDataSource } from './fixtureSource';

function input(overrides = {}) {
  return {
    kind: 'movies' as const,
    pivot: 'Movies',
    filter: 'All items',
    sort: 'Title',
    direction: 'ascending' as const,
    ...overrides,
  };
}

describe('FixturePorticoDataSource', () => {
  it('returns only the requested library kind in catalogue order', async () => {
    const result = await new FixturePorticoDataSource().browseLibrary(input(), new AbortController().signal);
    expect(result.items.length).toBeGreaterThan(5);
    expect(result.items.every((item) => item.type === 'movie')).toBe(true);
    expect(result.items[0].title).toBe('Blade Runner 2049');
  });

  it('supports filter and direction without changing the fixture collection', async () => {
    const source = new FixturePorticoDataSource();
    const descending = await source.browseLibrary(input({ direction: 'descending' as const }), new AbortController().signal);
    const unwatched = await source.browseLibrary(input({ filter: 'Unwatched' }), new AbortController().signal);
    expect(descending.items[0].title).toBe('Run Lola Run');
    expect(unwatched.total).toBe(descending.total);
  });

  it('rejects an already aborted request', async () => {
    const controller = new AbortController();
    controller.abort();
    await expect(new FixturePorticoDataSource().browseLibrary(input(), controller.signal)).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('publishes independently loadable Home row descriptors', async () => {
    const source = new FixturePorticoDataSource();
    const signal = new AbortController().signal;
    const manifest = await source.home(signal);
    expect(manifest.rows[0]).toMatchObject({ critical: true, cursorCapable: true, defaultVisible: true });
    expect(manifest.rows[0].endpoint).toBe('/api/home/rows/continue-watching');
    const row = await source.homeRow(manifest.rows[0].id, undefined, signal, 2);
    expect(row.items).toHaveLength(2);
    expect(row.hasMore).toBe(true);
    expect(row.nextCursor).toBe('fixture:2');
  });

  it('keeps search grouping and continuation scoped to the requested group', async () => {
    const source = new FixturePorticoDataSource();
    const signal = new AbortController().signal;
    const first = await source.searchPage({ query: 'the', entityKinds: ['movie'], limit: 2 }, signal);
    expect(first.groups).toHaveLength(1);
    expect(first.groups[0]).toMatchObject({ id: 'movies', entityKind: 'movie', hasMore: true });
    const next = await source.searchPage({ query: 'the', group: 'movies', entityKinds: ['movie'], cursor: first.groups[0].nextCursor ?? undefined, limit: 2 }, signal);
    expect(next.groups).toHaveLength(1);
    expect(next.groups[0].items.map((item) => item.id)).not.toContain(first.groups[0].items[0].id);
  });

  it('groups search results by the canonical kinds advertised by its Search Contract', async () => {
    const source = new FixturePorticoDataSource();
    const signal = new AbortController().signal;
    const contract = await source.searchContract(signal);
    const result = await source.searchPage({ query: 'Bonobo', entityKinds: ['artist'] }, signal);

    expect(contract.groups.find((group) => group.id === 'music')?.resultKinds).toContain('artist');
    expect(result.groups).toHaveLength(1);
    expect(result.groups[0]).toMatchObject({ id: 'music', entityKind: 'track', title: 'Music' });
    expect(result.groups[0].items).not.toHaveLength(0);
    expect(result.groups[0].items.every((item) => item.kind === 'artist')).toBe(true);
  });

  it('keeps detail operations functional without inventing completed server work', async () => {
    const source = new FixturePorticoDataSource();
    const signal = new AbortController().signal;
    const item = (await source.browseLibrary(input(), signal)).items[0];
    const refresh = await source.queueMediaJob(item.id, 'metadata_refresh', {}, signal);
    const optimized = await source.createOptimizedVersion(item.id, '720p-medium', signal);
    const options = await source.mediaDownloadOptions(item.id, signal);

    expect(refresh).toMatchObject({ type: 'metadata_refresh', status: 'queued', resourceId: item.id, progress: 0 });
    expect(optimized).toMatchObject({ type: 'optimize_version', status: 'queued', metadata: { profile: '720p-medium' } });
    expect(options.options.find((option) => option.profile === '720p-medium')?.job).toMatchObject({ id: optimized.id, status: 'queued' });
    expect(await source.createMediaDownloadURL(item.id, 'source', signal)).toMatch(/^data:audio\/wav/);
  });

  it('models upload, preferred selection, ordering, and removal for artwork review', async () => {
    const source = new FixturePorticoDataSource();
    const signal = new AbortController().signal;
    const item = (await source.browseLibrary(input(), signal)).items[0];

    await source.uploadMediaImage(item.id, 'poster', new File(['image'], 'poster.png', { type: 'image/png' }), 1, signal);
    let detail = await source.media(item.id, signal);
    const uploaded = detail.mediaImages?.find((image) => image.source === 'manual');
    const provider = detail.mediaImages?.find((image) => image.type === 'poster' && image.source === 'provider');
    expect(uploaded).toMatchObject({ type: 'poster', provider: 'upload', preferred: true });
    expect(provider?.preferred).toBe(false);

    await source.setPreferredMediaImage(item.id, provider!.id, 1, signal);
    await source.reorderMediaImages(item.id, [uploaded!.id, provider!.id], 1, signal);
    detail = await source.media(item.id, signal);
    expect(detail.mediaImages?.find((image) => image.id === provider!.id)).toMatchObject({ preferred: true, sortOrder: 1 });

    await source.deleteMediaImage(item.id, uploaded!.id, 1, signal);
    detail = await source.media(item.id, signal);
    expect(detail.mediaImages?.some((image) => image.id === uploaded!.id)).toBe(false);
  });

  it('models technical media, subtitle, lyrics, and optimized-version mutations', async () => {
    const source = new FixturePorticoDataSource();
    const signal = new AbortController().signal;

    let detail = await source.media('fargo', signal);
    expect(detail.streams?.map((stream) => stream.kind)).toEqual(['video', 'audio', 'subtitle']);
    expect(detail.attachments?.[0]).toMatchObject({ filename: 'Inter-SemiBold.ttf' });
    expect(detail.optimizedVersions?.[0]).toMatchObject({ profile: '720p-medium' });

    await source.updateSubtitle('fargo', 'fargo-subtitle-1', 1200, signal);
    await source.uploadSubtitle('fargo', new File(['WEBVTT'], 'fr.vtt'), 'fr', 'Français', signal);
    detail = await source.media('fargo', signal);
    expect(detail.streams?.find((stream) => stream.id === 'fargo-subtitle-1')?.subtitleOffsetMs).toBe(1200);
    const uploadedSubtitle = detail.streams?.find((stream) => stream.displayTitle === 'Français');
    expect(uploadedSubtitle?.sourceUrl).toContain('fr.vtt');
    await source.deleteSubtitle('fargo', uploadedSubtitle!.id, signal);
    await source.deleteOptimizedVersion('fargo', '720p-medium', signal);
    detail = await source.media('fargo', signal);
    expect(detail.streams?.some((stream) => stream.id === uploadedSubtitle!.id)).toBe(false);
    expect(detail.optimizedVersions).toEqual([]);

    await source.uploadLyrics('track-kiara', new File(['lyrics'], 'kiara.txt'), 'en', signal);
    const candidates = await source.searchLyrics('track-kiara', 'Kiara Bonobo', signal);
    await source.applyLyrics('track-kiara', candidates[0], signal);
    detail = await source.media('track-kiara', signal);
    expect(detail.lyrics?.some((lyric) => lyric.provider === 'upload')).toBe(true);
    const applied = detail.lyrics?.find((lyric) => lyric.id.startsWith('fixture-applied-'));
    expect(applied).toMatchObject({ provider: 'lrclib', synced: true });
    await source.deleteLyrics('track-kiara', applied!.id, signal);
    detail = await source.media('track-kiara', signal);
    expect(detail.lyrics?.some((lyric) => lyric.id === applied!.id)).toBe(false);
  });
});
