import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ArtworkEditor } from './ArtworkEditor';

const images = [
  { id: 'poster-current', type: 'poster', source: 'provider', provider: 'tmdb', width: 1000, height: 1500, sortOrder: 0, preferred: true, createdAt: '2026-07-01T12:00:00Z' },
  { id: 'poster-upload', type: 'poster', source: 'manual', provider: 'upload', width: 1200, height: 1800, sortOrder: 1, preferred: false, createdAt: '2026-07-02T12:00:00Z' },
  { id: 'poster-alternate', type: 'poster', source: 'provider', provider: 'fanart', width: 900, height: 1350, sortOrder: 2, preferred: false, createdAt: '2026-07-03T12:00:00Z' },
];

function setup() {
  const actions = {
    onUpload: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn().mockResolvedValue(undefined),
    onPreferred: vi.fn().mockResolvedValue(undefined),
    onReorder: vi.fn().mockResolvedValue(undefined),
  };
  render(<ArtworkEditor images={images} fallbackUrls={{ poster: '/poster.jpg' }} {...actions} />);
  return actions;
}

describe('ArtworkEditor', () => {
  it('selects, reorders, uploads, and confirms deletion through real action callbacks', async () => {
    const actions = setup();

    fireEvent.click(screen.getByRole('button', { name: 'Use Uploaded poster' }));
    await waitFor(() => expect(actions.onPreferred).toHaveBeenCalledWith('poster-upload'));

    fireEvent.click(screen.getAllByRole('button', { name: 'Move poster earlier' }).find((button) => !button.hasAttribute('disabled'))!);
    await waitFor(() => expect(actions.onReorder).toHaveBeenCalledWith(['poster-current', 'poster-alternate', 'poster-upload']));

    const file = new File(['image'], 'replacement.png', { type: 'image/png' });
    fireEvent.change(document.querySelector('.artwork-file-input')!, { target: { files: [file] } });
    await waitFor(() => expect(actions.onUpload).toHaveBeenCalledWith('poster', file));

    fireEvent.click(screen.getByRole('button', { name: 'Remove poster' }));
    expect(screen.getByText('Remove this uploaded poster?')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(actions.onDelete).toHaveBeenCalledWith('poster-upload'));
  });

  it('switches artwork types and exposes an honest empty state', () => {
    setup();
    fireEvent.click(screen.getByRole('tab', { name: 'Backdrop' }));
    expect(screen.getByText('No backdrop artwork')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Choose image' })).toBeInTheDocument();
  });
});
