import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { App } from './App';
import { DataProvider } from './data/DataProvider';
import { FixturePorticoDataSource } from './data/fixtureSource';

function renderLibrary(path = '/library/fixture-movies?pivot=movies') {
  return render(
    <DataProvider source={new FixturePorticoDataSource()}>
      <MemoryRouter initialEntries={[path]}>
        <App />
        <LocationProbe />
      </MemoryRouter>
    </DataProvider>,
  );
}

function LocationProbe() {
  const location = useLocation();
  return <output aria-label="Current route">{location.pathname}{location.search}</output>;
}

describe('production library route integration', () => {
  it('resolves a stable library id and exposes every server-declared pivot', async () => {
    renderLibrary();
    expect(await screen.findByRole('heading', { name: 'Movies' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Discover' })).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'Movies' }).length).toBeGreaterThan(0);
    expect(screen.getByRole('link', { name: 'Collections' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Categories' })).toBeInTheDocument();
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/library/fixture-movies?pivot=movies');
  });

  it('keeps capability-backed multi-select actions in the production workspace', async () => {
    renderLibrary();
    await screen.findByText('Blade Runner 2049');
    fireEvent.click(screen.getByRole('button', { name: 'Select Blade Runner 2049' }));
    const selection = screen.getByRole('region', { name: '1 selected items' });
    expect((await within(selection).findAllByRole('button', { name: /watchlist/i })).length).toBeGreaterThan(0);
    expect(within(selection).getAllByRole('button', { name: /watched/i }).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Deselect Blade Runner 2049' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('uses the square artwork hierarchy for music libraries', async () => {
    renderLibrary('/library/fixture-music?pivot=albums');
    expect(await screen.findByRole('heading', { name: 'Music' })).toBeInTheDocument();
    expect(await screen.findByText('Black Sands')).toBeInTheDocument();
    expect(document.querySelector('.library-media-grid.square')).toBeInTheDocument();
  });

  it('does not reinterpret a missing stable id as a library kind', async () => {
    renderLibrary('/library/movies');
    expect(await screen.findByText('This library isn’t available')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open libraries' })).toHaveAttribute('href', '/libraries');
  });
});
