import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { MediaRatingDialog } from './MediaRatingDialog';

describe('MediaRatingDialog', () => {
  it('saves the selected account rating', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const onDismiss = vi.fn();
    render(<MediaRatingDialog title="Arrival" value={7} onSave={onSave} onDismiss={onDismiss} />);
    const dialog = screen.getByRole('dialog', { name: 'Rate Arrival' });

    fireEvent.click(within(dialog).getByRole('radio', { name: '9' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save rating' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(9));
    expect(onDismiss).toHaveBeenCalled();
  });

  it('keeps the dialog open when the server rejects a change', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('The server is unavailable.'));
    const onDismiss = vi.fn();
    render(<MediaRatingDialog title="Arrival" value={0} onSave={onSave} onDismiss={onDismiss} />);
    const dialog = screen.getByRole('dialog', { name: 'Rate Arrival' });

    fireEvent.click(within(dialog).getByRole('radio', { name: '6' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save rating' }));

    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Review the media information and try again.');
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it('clears an existing rating through the same authoritative mutation', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<MediaRatingDialog title="Arrival" value={8} onSave={onSave} onDismiss={() => undefined} />);

    fireEvent.click(screen.getByRole('button', { name: 'Clear rating' }));
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(0));
  });
});
