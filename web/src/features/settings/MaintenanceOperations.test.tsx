import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { MaintenanceOperations, pollRestoreStatus } from './MaintenanceOperations';
import type { RestoreWorkflowResponse, SettingsDataSource } from './settingsTypes';

const initial: RestoreWorkflowResponse = {
  ok: true,
  name: 'portico-backup.db',
  operationId: 'restore-test',
  state: 'staged',
  phase: 'staged',
  progress: 25,
  instruction: 'staged',
  recoveryRequired: false,
  statusToken: 'only-on-enqueue',
};

describe('supervised restore status polling', () => {
	it('shows truthful update availability without rendering a dead mutation control', () => {
		render(<MaintenanceOperations tasks={[]} backups={[]} storage={{ totalBytes: 0, categories: [], generatedAt: new Date().toISOString() }} source={{} as SettingsDataSource} onChanged={vi.fn()} />);
		expect(screen.queryByRole('button', { name: /update/i })).not.toBeInTheDocument();
		expect(screen.getByText('Updates unavailable')).toBeInTheDocument();
		expect(screen.getByText(/Use the documented update procedure/)).toBeInTheDocument();
	});

  it('keeps the out-of-band capability across tokenless nonterminal responses', async () => {
    const requests: Array<{ operationId: string; statusToken: string }> = [];
    const responses: RestoreWorkflowResponse[] = [
      { ...initial, statusToken: undefined, state: 'quiescing', phase: 'quiescing', progress: 35 },
      { ...initial, statusToken: undefined, state: 'health-checking', phase: 'health-checking', progress: 92 },
      { ...initial, statusToken: undefined, state: 'complete', phase: 'complete', progress: 100, ok: true },
    ];
    const source: Pick<SettingsDataSource, 'restoreStatus'> = {
      restoreStatus: async (operationId: string, statusToken: string) => {
        requests.push({ operationId, statusToken });
        const response = responses.shift();
        if (!response) throw new Error('unexpected extra status request');
        return response;
      },
    };
    const sleeps: number[] = [];
    const result = await pollRestoreStatus(source, initial, {
      operationId: 'restore-test',
      statusToken: 'only-on-enqueue',
      name: 'portico-backup.db',
    }, new AbortController().signal, { sleep: async (milliseconds) => { sleeps.push(milliseconds); } });

    expect(result.state).toBe('complete');
    expect(requests).toEqual([
      { operationId: 'restore-test', statusToken: 'only-on-enqueue' },
      { operationId: 'restore-test', statusToken: 'only-on-enqueue' },
      { operationId: 'restore-test', statusToken: 'only-on-enqueue' },
    ]);
    expect(sleeps).toEqual([1000, 1000, 1000]);
  });

  it('retains the capability through transient transport and 503 failures', async () => {
    let attempts = 0;
    const retryReasons: unknown[] = [];
    const source: Pick<SettingsDataSource, 'restoreStatus'> = {
      restoreStatus: async () => {
        attempts += 1;
        if (attempts === 1) throw new TypeError('connection reset');
        if (attempts === 2) throw Object.assign(new Error('maintenance'), { status: 503 });
        return { ...initial, statusToken: undefined, state: 'complete', phase: 'complete', progress: 100 };
      },
    };
    const sleeps: number[] = [];
    const result = await pollRestoreStatus(source, initial, {
      operationId: 'restore-test',
      statusToken: 'only-on-enqueue',
      name: 'portico-backup.db',
    }, new AbortController().signal, {
      sleep: async (milliseconds) => { sleeps.push(milliseconds); },
      onRetry: (_response, reason) => retryReasons.push(reason),
    });

    expect(result.state).toBe('complete');
    expect(attempts).toBe(3);
    expect(retryReasons).toHaveLength(2);
    expect(sleeps).toEqual([1000, 1000, 2000]);
  });
});
