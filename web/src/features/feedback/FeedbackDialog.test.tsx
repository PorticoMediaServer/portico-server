import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { FeedbackDialog } from './FeedbackDialog';

describe('FeedbackDialog', () => {
  it('submits canonical web context and does not promise an owner response', async () => {
    const source = new FixturePorticoDataSource();
    const submit = vi.spyOn(source, 'submitViewerFeedback');
    render(<DataProvider source={source}><FeedbackDialog kind="general" onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Send a message' });
    const message = await within(dialog).findByLabelText('Your message');
    fireEvent.change(message, { target: { value: 'The guide looks out of date.' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Send message' }));

    await waitFor(() => expect(submit).toHaveBeenCalled());
    expect(submit.mock.calls[0][0]).toMatchObject({ version: 'v1', kind: 'general', category: 'other', context: { deviceClass: 'web', platform: 'web' } });
    expect(await within(dialog).findByText('The server owner will see your message in Portico.')).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent(/can reply|will reply/i);
  });

  it('shows a truthful unavailable state when the server disables feedback', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'viewerFeedbackCapabilities').mockResolvedValue({ version: 'v1', enabled: false, allowedKinds: [], messageMaxLength: 1000, retentionDays: 90 });
    render(<DataProvider source={source}><FeedbackDialog kind="media" mediaId="movie-1" onDismiss={() => undefined} /></DataProvider>);
    const dialog = await screen.findByRole('dialog', { name: 'Report a problem' });
    expect(await within(dialog).findByText("Messages aren't available")).toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: 'Send message' })).not.toBeInTheDocument();
  });

  it('never renders an unknown capability error detail', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'viewerFeedbackCapabilities').mockRejectedValue(new Error('sqlite /private/media/path secret'));
    render(<DataProvider source={source}><FeedbackDialog kind="general" onDismiss={() => undefined} /></DataProvider>);

    const dialog = await screen.findByRole('dialog', { name: 'Send a message' });
    expect(await within(dialog).findByText("Portico couldn't complete this request")).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent(/sqlite|private\/media|secret/i);
  });
});
