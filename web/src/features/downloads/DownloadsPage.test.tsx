import type { DownloadPreparation } from '@portico/client-core';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DownloadsPage } from './DownloadsPage';

const mockUseMediaOperations = vi.hoisted(() => vi.fn());

vi.mock('../../data/DataProvider', () => ({ useMediaOperations: mockUseMediaOperations }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

function preparation(overrides: Partial<DownloadPreparation> = {}): DownloadPreparation {
  return {
    id: 'download-1',
    mediaId: 'media-1',
    mediaTitle: 'The Example',
    qualityProfile: 'source',
    state: 'queued',
    progress: 20,
    sizeKind: 'unknown',
    canPause: true,
    canCancel: true,
    canRetry: false,
    canRemove: true,
    createdAt: '2026-08-17T12:00:00.000Z',
    updatedAt: '2026-08-17T12:00:00.000Z',
    ...overrides,
  };
}

describe('DownloadsPage queue polling and actions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('waits for each queue refresh before scheduling the next poll', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date('2026-08-17T12:00:00.000Z'));
    const first = deferred<DownloadPreparation[]>();
    const second = deferred<DownloadPreparation[]>();
    const operations = {
      downloadPreparations: vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise),
      updateDownloadPreparation: vi.fn(),
      downloadPreparationURL: vi.fn(),
    };
    mockUseMediaOperations.mockReturnValue(operations);
    render(<DownloadsPage />);

    expect(operations.downloadPreparations).toHaveBeenCalledTimes(1);
    await act(async () => {
      first.resolve([preparation()]);
      await first.promise;
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_500);
    });
    expect(operations.downloadPreparations).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4_500);
    });
    expect(operations.downloadPreparations).toHaveBeenCalledTimes(2);

    await act(async () => {
      second.resolve([preparation({ progress: 80, state: 'running' })]);
      await second.promise;
    });
    expect(screen.getByText(/80%/)).toBeInTheDocument();
  });

  it('surfaces a failed queue action and retries the same operation', async () => {
    const item = preparation({ state: 'paused', canPause: false, canCancel: true });
    const operations = {
      downloadPreparations: vi.fn().mockResolvedValue([item]),
      updateDownloadPreparation: vi.fn().mockRejectedValue(new Error('private server detail')),
      downloadPreparationURL: vi.fn(),
    };
    mockUseMediaOperations.mockReturnValue(operations);
    render(<DownloadsPage />);
    expect(await screen.findByText('The Example')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent("Portico couldn't cancel this download. Nothing was changed. Try again.");
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeEnabled();
    expect(screen.getByText('The Example')).toBeInTheDocument();

    fireEvent.click(withinAlertRetry(alert));
    await waitFor(() => expect(operations.updateDownloadPreparation).toHaveBeenCalledTimes(2));
  });
});

function withinAlertRetry(alert: HTMLElement) {
  return alert.querySelector('button') as HTMLButtonElement;
}
