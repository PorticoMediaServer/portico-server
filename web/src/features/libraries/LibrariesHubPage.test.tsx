import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { PorticoDataSource } from '../../data/models';
import { WebDisplayPreferencesProvider } from '../../preferences/WebDisplayPreferencesProvider';
import { LibrariesHubPage } from './LibrariesHubPage';

function renderPage() {
  const source = new FixturePorticoDataSource();
  render(<DataProvider source={source as PorticoDataSource} initialViewer={{
    authenticated: true,
    setupRequired: false,
    serverName: 'Test Server',
    user: {
      id: 'fixture-owner',
      displayName: 'Owner',
      email: 'owner@example.test',
      role: 'owner',
      permissions: { manageServer: true, manageLibraries: true },
      preferences: { sidebarOrder: [] },
    },
  }}><WebDisplayPreferencesProvider><MemoryRouter><LibrariesHubPage /></MemoryRouter></WebDisplayPreferencesProvider></DataProvider>);
  return source;
}

describe('LibrariesHubPage navigation management', () => {
  it('keeps the library frame mounted without exposing a centered loading state', () => {
    const source = new FixturePorticoDataSource();
    source.libraries = () => new Promise(() => undefined);
    const { container } = render(<DataProvider source={source as PorticoDataSource}><MemoryRouter><LibrariesHubPage /></MemoryRouter></DataProvider>);

    expect(screen.getByRole('heading', { name: 'Libraries' })).toBeInTheDocument();
    expect(container.querySelector('.library-hub-reservation')).toHaveAttribute('aria-busy', 'true');
    expect(container.querySelector('.library-state')).not.toBeInTheDocument();
  });

  it('announces a reviewed load failure and recovers in place without exposing diagnostics', async () => {
    const source = new FixturePorticoDataSource();
    const load = vi.spyOn(source, 'libraries')
      .mockRejectedValue(new Error('postgres socket /private/db-name refused'));
    render(<DataProvider source={source as PorticoDataSource}><MemoryRouter><LibrariesHubPage /></MemoryRouter></DataProvider>);

    const failure = await screen.findByRole('alert');
    expect(failure).toHaveTextContent('Couldn’t load libraries');
    expect(failure).not.toHaveTextContent('postgres socket');
    const callsBeforeRetry = load.mock.calls.length;
    load.mockResolvedValue([]);
    fireEvent.click(within(failure).getByRole('button', { name: 'Try again' }));

    await waitFor(() => expect(load.mock.calls.length).toBeGreaterThan(callsBeforeRetry));
    expect(await screen.findByText('Add your first library')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('pins, unpins, and reorders libraries through persisted account preferences', async () => {
    const source = renderPage();
    const summary = await screen.findByLabelText('Library summary');
    await waitFor(() => expect(summary).toHaveTextContent('3 pinned'));

    fireEvent.click(screen.getByRole('button', { name: 'Library options for TV Shows' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: /unpin from sidebar/i }));
    await waitFor(() => expect(summary).toHaveTextContent('2 pinned'));

    fireEvent.click(screen.getByRole('button', { name: 'Library options for Movies' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: /move up/i }));

    await waitFor(async () => {
      const navigation = await source.libraryNavigation(new AbortController().signal);
      expect(navigation.pinnedLibraryIds).toEqual(['fixture-movies', 'fixture-music']);
    });

    const directory = screen.getByRole('region', { name: 'Available libraries' });
    const links = within(directory).getAllByRole('link');
    expect(links[0]).toHaveTextContent('Movies');
    expect(links[1]).toHaveTextContent('Music');
  });
});
