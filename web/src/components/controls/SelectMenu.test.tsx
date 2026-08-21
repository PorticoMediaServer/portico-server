import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { SelectMenu } from './SelectMenu';

describe('SelectMenu listbox accessibility', () => {
  it('labels the popup, focuses selection, supports navigation and returns focus on Escape', async () => {
    render(<SelectMenu label="Sort" value="b" options={[{ id: 'a', label: 'Alpha' }, { id: 'b', label: 'Beta' }, { id: 'c', label: 'Gamma' }]} onChange={vi.fn()} />);
    const trigger = screen.getByRole('button', { name: 'Sort' });

    fireEvent.click(trigger);
    const listbox = screen.getByRole('listbox', { name: 'Sort' });
    expect(trigger).toHaveAttribute('aria-controls', listbox.id);
    await waitFor(() => expect(screen.getByRole('option', { name: 'Beta' })).toHaveFocus());
    expect(screen.getByRole('option', { name: 'Beta' })).toHaveAttribute('aria-selected', 'true');

    fireEvent.keyDown(window, { key: 'ArrowDown' });
    expect(screen.getByRole('option', { name: 'Gamma' })).toHaveFocus();
    fireEvent.keyDown(window, { key: 'Home' });
    expect(screen.getByRole('option', { name: 'Alpha' })).toHaveFocus();
    fireEvent.keyDown(window, { key: 'End' });
    expect(screen.getByRole('option', { name: 'Gamma' })).toHaveFocus();
    fireEvent.keyDown(window, { key: 'a' });
    expect(screen.getByRole('option', { name: 'Alpha' })).toHaveFocus();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
