import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { HomeResult, HomeRow, MediaItem, PorticoDataSource, Viewer } from '../../data/models';
import { LibrariesHubPage } from '../libraries/LibrariesHubPage';
import { WebDisplayPreferencesProvider } from '../../preferences/WebDisplayPreferencesProvider';
import { HomePage, HomeRowSurface } from './HomePage';

const owner: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: 'Empty Portico',
  user: {
    id: 'owner',
    displayName: 'Owner',
    email: 'owner@portico.local',
    role: 'owner',
    permissions: { manageServer: true, manageLibraries: true },
    preferences: { sidebarOrder: [] },
  },
};

const member: Viewer = {
  ...owner,
  user: {
    ...owner.user!,
    id: 'member',
    displayName: 'Member',
    role: 'user',
    permissions: {},
  },
};

class EmptyPorticoSource extends FixturePorticoDataSource {
  async home(signal: AbortSignal): Promise<HomeResult> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return { pivots: ['Home'], rows: [] };
  }

  async libraries(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return [];
  }
}

class RecoveringEmptyPorticoSource extends EmptyPorticoSource {
  private attempts = 0;

  async libraries(signal: AbortSignal) {
    this.attempts += 1;
    if (this.attempts === 1) throw new Error('The library service did not answer.');
    return super.libraries(signal);
  }
}

class ProjectedHomeActionSource extends FixturePorticoDataSource {
  watchlistUpdates: boolean[] = [];
  private hero?: MediaItem;

  async home(signal: AbortSignal): Promise<HomeResult> {
    const result = await super.home(signal);
    const descriptor = result.rows[0];
    const hero = descriptor?.items[0];
    if (!descriptor || !hero) throw new Error('Fixture Home hero is unavailable.');
    this.hero = { ...hero, watchlisted: false, actions: ['play', 'watchlist.add'] };
    return { ...result, rows: [{ ...descriptor, endpoint: undefined, items: [structuredClone(this.hero)] }] };
  }

  async setWatchlist(id: string, watchlisted: boolean, signal: AbortSignal): Promise<MediaItem> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!this.hero || this.hero.id !== id) throw new Error('Unknown Home hero.');
    this.watchlistUpdates.push(watchlisted);
    this.hero = { ...this.hero, watchlisted, actions: ['play', watchlisted ? 'watchlist.remove' : 'watchlist.add'] };
    return structuredClone(this.hero);
  }
}

class UnplayableProgressHomeSource extends FixturePorticoDataSource {
  async home(signal: AbortSignal): Promise<HomeResult> {
    const result = await super.home(signal);
    const descriptor = result.rows[0];
    const hero = descriptor?.items[0];
    if (!descriptor || !hero) throw new Error('Fixture Home hero is unavailable.');
    return {
      ...result,
      rows: [{
        ...descriptor,
        endpoint: undefined,
        items: [{ ...hero, progressSeconds: 90, actions: ['watchlist.add'] }],
      }],
    };
  }
}

class CustomEmptyRowsSource extends FixturePorticoDataSource {
  async home(signal: AbortSignal): Promise<HomeResult> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return {
      pivots: ['Home'],
      rows: [
        { id: 'custom-one', title: 'Custom one', type: 'square', artworkShape: 'square', endpoint: '/custom-one', items: [], hasMore: false },
        { id: 'custom-two', title: 'Custom two', type: 'poster', endpoint: '/custom-two', items: [], hasMore: false },
      ],
    };
  }

  async homeRow(id: string, _cursor: string | undefined, signal: AbortSignal): Promise<HomeRow> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return { id, title: id === 'custom-one' ? 'Custom one' : 'Custom two', type: 'poster', items: [], hasMore: false };
  }
}

class DegradedHomeSource extends FixturePorticoDataSource {
  attempts = 0;

  async home(signal: AbortSignal): Promise<HomeResult> {
    const result = await super.home(signal);
    const ready = result.rows[0];
    if (!ready) throw new Error('Fixture Home row is unavailable.');
    return {
      ...result,
      rows: [
        { ...ready, endpoint: undefined },
        { id: 'deferred-failure', title: 'Because you watched', type: 'poster', endpoint: '/deferred-failure', items: [], hasMore: false },
      ],
    };
  }

  async homeRow(id: string, cursor: string | undefined, signal: AbortSignal): Promise<HomeRow> {
    if (id !== 'deferred-failure') return super.homeRow(id, cursor, signal);
    this.attempts += 1;
    throw new Error('Deferred row is unavailable.');
  }
}

function asSource(source: FixturePorticoDataSource) {
  return source as unknown as PorticoDataSource;
}

function renderHome(viewer: Viewer, source: FixturePorticoDataSource) {
  return render(<DataProvider source={asSource(source)} initialViewer={viewer}><WebDisplayPreferencesProvider><MemoryRouter><HomePage /></MemoryRouter></WebDisplayPreferencesProvider></DataProvider>);
}

function renderLibraries(viewer: Viewer) {
  const source = new EmptyPorticoSource(viewer);
  return render(<DataProvider source={asSource(source)} initialViewer={viewer}><MemoryRouter><LibrariesHubPage /></MemoryRouter></DataProvider>);
}

describe('first library journey', () => {
  it('takes an owner from empty Home directly into first-library creation', async () => {
    renderHome(owner, new EmptyPorticoSource(owner));
    expect(await screen.findByRole('heading', { name: 'Bring your media into Portico.' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Add first library' })).toHaveAttribute('href', '/settings/media?newLibrary=1');
    expect(screen.getByText(/keep the files where they are/)).toBeInTheDocument();
  });

  it('silently retries a transient library lookup before showing the first-library journey', async () => {
    renderHome(owner, new RecoveringEmptyPorticoSource(owner));
    expect(await screen.findByRole('link', { name: 'Add first library' })).toHaveAttribute('href', '/settings/media?newLibrary=1');
		expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('uses shared projected actions, keeps contextual episode copy, and adopts mutation-returned eligibility', async () => {
    const source = new ProjectedHomeActionSource(owner);
    renderHome(owner, source);

    expect(await screen.findByRole('button', { name: 'Play next episode' })).toBeInTheDocument();
    fireEvent.click(await screen.findByRole('button', { name: 'Add to watchlist' }));

    expect(await screen.findByRole('button', { name: 'Remove from watchlist' })).toBeInTheDocument();
    expect(source.watchlistUpdates).toEqual([true]);
  });

  it('does not synthesize Home playback from progress when the server omits play eligibility', async () => {
    renderHome(owner, new UnplayableProgressHomeSource(owner));

    expect(await screen.findByRole('button', { name: 'Add to watchlist' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument();
  });

  it('reserves every server-advertised custom row in server order when rows resolve empty', async () => {
    const { container } = renderHome(owner, new CustomEmptyRowsSource(owner));
    await waitFor(() => expect(container.querySelectorAll('[data-home-slot-resolution="empty"]')).toHaveLength(2));
    const reserved = [...container.querySelectorAll('[data-home-slot-resolution="empty"]')];
    expect(reserved.map((node) => node.querySelector('section')?.getAttribute('aria-label'))).toEqual(['Custom one', 'Custom two']);
    expect(reserved[0].querySelector('section')).toHaveAttribute('data-artwork-shape', 'square');
    expect(reserved[1].querySelector('section')).toHaveAttribute('data-artwork-shape', 'poster');
  });

  it('keeps useful Home content while surfacing an initially deferred row failure with retry', async () => {
    const source = new DegradedHomeSource(owner);
    const { container } = renderHome(owner, source);

    expect(await screen.findByRole('region')).toBeInTheDocument();
    await waitFor(() => expect(container.querySelector('[data-home-slot-resolution="failed"]')).toBeInTheDocument());
    expect(screen.getAllByText('Because you watched is unavailable')).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Try again' }).length).toBeGreaterThan(0);
    expect(container.querySelector('.portico-home-hero')).not.toContainElement(screen.getByRole('button', { name: 'Customize Home' }));
    expect(source.attempts).toBeGreaterThan(0);
  });

  it('drops a delayed continuation after its row descriptor changes', async () => {
    const source = new FixturePorticoDataSource(owner);
    const media = (id: string, title: string): MediaItem => ({ id, title, subtitle: '', year: 0, type: 'movie', kind: 'movie', poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [] });
    const oldRow: HomeRow = { id: 'continue', title: 'Old row', type: 'poster', endpoint: '/old', items: [media('old-first', 'Old first')], hasMore: true, nextCursor: 'old-cursor' };
    const newRow: HomeRow = { id: 'continue', title: 'New row', type: 'poster', endpoint: '/new', items: [media('new-first', 'New first')], hasMore: false, nextCursor: null };
    let initialCalls = 0;
    let resolveOld!: (value: HomeRow) => void;
    vi.spyOn(source, 'homeRow').mockImplementation((_id, cursor) => cursor
      ? new Promise((resolve) => { resolveOld = resolve; })
      : Promise.resolve(structuredClone(initialCalls++ === 0 ? oldRow : newRow)));
    const shell = (descriptor: HomeRow) => <DataProvider source={source} initialViewer={owner}><MemoryRouter><HomeRowSurface descriptor={descriptor} eager onResolved={vi.fn()} /></MemoryRouter></DataProvider>;
    const view = render(shell(oldRow));

    const rail = await screen.findByRole('region', { name: 'Old row' });
    Object.defineProperties(rail, {
      scrollWidth: { configurable: true, value: 1_000 },
      clientWidth: { configurable: true, value: 600 },
      scrollLeft: { configurable: true, value: 350, writable: true },
    });
    fireEvent.scroll(rail);
    await waitFor(() => expect(resolveOld).toBeTypeOf('function'));
    view.rerender(shell(newRow));
    expect(await screen.findByText('New first')).toBeInTheDocument();
    await act(async () => resolveOld({ ...oldRow, items: [media('old-late', 'Old late')], hasMore: false, nextCursor: null }));
    expect(screen.queryByText('Old late')).not.toBeInTheDocument();
  });

  it('continues across a duplicate-only cursor page without duplicating cards', async () => {
    const source = new FixturePorticoDataSource(owner);
    const item = (id: string, title: string): MediaItem => ({ id, title, subtitle: '', year: 0, type: 'movie', kind: 'movie', poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [] });
    const first = item('first', 'First title');
    const row: HomeRow = { id: 'continuous', title: 'Continuous row', type: 'poster', endpoint: '/continuous', items: [first], hasMore: true, nextCursor: 'cursor-1' };
    const cursors: Array<string | undefined> = [];
    vi.spyOn(source, 'homeRow').mockImplementation(async (_id, cursor) => {
      cursors.push(cursor);
      return cursor === 'cursor-1'
        ? { ...row, items: [first], hasMore: true, nextCursor: 'cursor-2' }
        : { ...row, items: [item('second', 'Second title')], hasMore: false, nextCursor: null };
    });
    render(<DataProvider source={source} initialViewer={owner}><MemoryRouter><HomeRowSurface descriptor={row} eager onResolved={vi.fn()} /></MemoryRouter></DataProvider>);

    const rail = await screen.findByRole('region', { name: 'Continuous row' });
    Object.defineProperties(rail, {
      scrollWidth: { configurable: true, value: 1_000 },
      clientWidth: { configurable: true, value: 600 },
      scrollLeft: { configurable: true, value: 390, writable: true },
    });
    fireEvent.scroll(rail);

    expect(await screen.findByText('Second title')).toBeInTheDocument();
    expect(screen.getAllByText('First title')).toHaveLength(1);
    expect(cursors).toEqual(['cursor-1', 'cursor-2']);
  });

  it('gives an owner a direct empty-Libraries action while a member sees sharing truth', async () => {
    const view = renderLibraries(owner);
    expect(await screen.findByText('Add your first library')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Add first library' })).toHaveAttribute('href', '/settings/media?newLibrary=1');

    view.unmount();
    renderLibraries(member);
    expect(await screen.findByText('This server has no available libraries')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Add first library' })).not.toBeInTheDocument();
  });
});
