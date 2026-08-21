import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { MediaItem } from '../../data/models';
import { MediaDeleteDialog } from './MediaDeleteDialog';

function media(id: string, title: string): MediaItem {
  return {
    id,
    title,
    subtitle: '',
    year: 2026,
    type: 'movie',
    kind: 'movie',
    poster: '',
    backdrop: '',
    rating: '',
    length: '',
    genre: '',
    fileCount: 1,
    actions: ['media.delete'],
  };
}

describe('MediaDeleteDialog', () => {
  it('requires the exact title before moving source files to server trash', async () => {
    const item = media('arrival', 'Arrival');
    const onDelete = vi.fn().mockResolvedValue({ ok: true, deletedItems: 1, trashedFiles: 2 });
    const onComplete = vi.fn();
    const onDismiss = vi.fn();
    render(<MediaDeleteDialog items={[item]} onDelete={onDelete} onComplete={onComplete} onDismiss={onDismiss} />);
    const dialog = screen.getByRole('dialog', { name: 'Arrival' });

    fireEvent.click(within(dialog).getByRole('radio', { name: /Move source files to trash/ }));
    const submit = within(dialog).getByRole('button', { name: 'Move to trash' });
    expect(submit).toBeDisabled();
    fireEvent.change(within(dialog).getByRole('textbox'), { target: { value: 'arrival' } });
    expect(submit).toBeDisabled();
    fireEvent.change(within(dialog).getByRole('textbox'), { target: { value: 'Arrival' } });
    fireEvent.click(submit);

    await waitFor(() => expect(onDelete).toHaveBeenCalledWith('arrival', { deleteFiles: true, confirmation: 'Arrival' }));
    expect(onComplete).toHaveBeenCalledWith({ deletedIds: ['arrival'], failedIds: [], deletedItems: 1, trashedFiles: 2 });
    expect(onDismiss).toHaveBeenCalled();
  });

  it('reports partial bulk failures and leaves failed items available to the caller', async () => {
    const first = media('arrival', 'Arrival');
    const second = media('contact', 'Contact');
    const onDelete = vi.fn((id: string) => id === 'contact'
      ? Promise.reject(new Error('The item is in use.'))
      : Promise.resolve({ ok: true, deletedItems: 1, trashedFiles: 0 }));
    const onComplete = vi.fn();
    const onDismiss = vi.fn();
    render(<MediaDeleteDialog items={[first, second]} onDelete={onDelete} onComplete={onComplete} onDismiss={onDismiss} />);
    const dialog = screen.getByRole('dialog', { name: 'Remove 2 selected items' });

    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove 2 items' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('1 removed; 1 could not be removed and remain selected.');
    expect(onComplete).toHaveBeenCalledWith({ deletedIds: ['arrival'], failedIds: ['contact'], deletedItems: 1, trashedFiles: 0 });
    expect(onDismiss).not.toHaveBeenCalled();
  });
});
