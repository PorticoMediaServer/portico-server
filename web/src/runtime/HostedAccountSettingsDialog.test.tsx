import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { HostedAccountSettingsDialog } from './HostedAccountSettingsDialog';

const hostedRequest = vi.hoisted(() => vi.fn());
const runtime = vi.hoisted(() => ({
  config: { mode: 'hosted', hostedApiBaseUrl: 'https://web.getportico.tv' },
  selectedHostedServerId: 'srv_published',
}));

vi.mock('./RuntimeContext', () => ({
  useRuntime: () => runtime,
  useOptionalRuntime: () => runtime,
}));
vi.mock('@porticomediaserver/client-core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@porticomediaserver/client-core')>();
  return {
    ...actual,
    createHostedServicesClient: () => ({
      account: vi.fn(() => new Promise(() => undefined)),
      request: hostedRequest,
    }),
  };
});

describe('HostedAccountSettingsDialog', () => {
  beforeEach(() => {
    hostedRequest.mockReset();
    hostedRequest.mockImplementation(async (path: string) => {
      if (path === '/api/account/servers/srv_published/invites?limit=100') return { items: [] };
      throw new Error(`Unexpected Hosted request: ${path}`);
    });
  });

  it('opens People from the unavailable shell and dispatches with the publication-fenced server id while Server transport fails', async () => {
    render(<HostedAccountSettingsDialog onDismiss={vi.fn()} />);

    fireEvent.click(await screen.findByRole('button', { name: 'People' }));

    await waitFor(() => expect(hostedRequest).toHaveBeenCalledWith(
      '/api/account/servers/srv_published/invites?limit=100',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    ));
    expect(screen.queryByText('Choose a server before opening People settings.')).not.toBeInTheDocument();
  });
});
