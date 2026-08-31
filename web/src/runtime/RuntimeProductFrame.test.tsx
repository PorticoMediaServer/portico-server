import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { DataProvider } from '../data/DataProvider';
import { FixturePorticoDataSource } from '../data/fixtureSource';
import { RuntimeProductFrame } from './RuntimeProductFrame';

const runtime = vi.hoisted(() => ({
  config: { mode: 'hosted', hostedApiBaseUrl: 'https://web.getportico.tv', routeProbeTimeoutMs: 3500, buildId: 'test' },
  state: { id: 'server-ready', startedAt: 0, retryable: false, recoveryActions: [], mode: 'fixtures', serverName: 'Portico Review' },
  restoredPresentation: { accountId: 'fixture-account', displayName: 'Portico Review' },
  connectionWarning: undefined,
  accountSettingsOpen: false,
  openAccountSettings: vi.fn(() => { runtime.accountSettingsOpen = true; }),
  closeAccountSettings: vi.fn(() => { runtime.accountSettingsOpen = false; }),
  reselectServer: vi.fn().mockResolvedValue(undefined),
  hostedLogout: vi.fn().mockResolvedValue(undefined),
  retry: vi.fn(),
}));

vi.mock('./RuntimeContext', () => ({
  useRuntime: () => runtime,
  useOptionalRuntime: () => runtime,
}));

describe('RuntimeProductFrame', () => {
  beforeEach(() => {
    runtime.state = { id: 'server-ready', startedAt: 0, retryable: false, recoveryActions: [], mode: 'fixtures', serverName: 'Portico Review' } as never;
    runtime.restoredPresentation = { accountId: 'fixture-account', displayName: 'Portico Review' } as never;
    runtime.reselectServer.mockClear();
    runtime.hostedLogout.mockClear();
    runtime.retry.mockClear();
    runtime.accountSettingsOpen = false;
  });

  it('keeps restored chrome in place until connected services take ownership', () => {
    const view = render(<MemoryRouter><RuntimeProductFrame connected><div aria-label="Connected services loading" /></RuntimeProductFrame></MemoryRouter>);

    expect(view.container.querySelector('.sidebar')).toHaveTextContent('Libraries');
    expect(view.container.querySelector('.topbar')).toHaveTextContent('Portico Review');
    expect(view.container.querySelector('.server-card')).toHaveTextContent('Connecting');
  });

  it('retains the exact sidebar, topbar, and main DOM nodes while connected services activate', async () => {
    const source = new FixturePorticoDataSource();
    const view = render(<MemoryRouter><RuntimeProductFrame><div aria-label="Connection reservation" /></RuntimeProductFrame></MemoryRouter>);
    const sidebar = view.container.querySelector('.sidebar');
    const topbar = view.container.querySelector('.topbar');
    const main = view.container.querySelector('#main-content');

    view.rerender(<MemoryRouter><RuntimeProductFrame connected><DataProvider source={source}><App /></DataProvider></RuntimeProductFrame></MemoryRouter>);
    await screen.findAllByRole('navigation', { name: 'Primary navigation' });

    expect(view.container.querySelector('.sidebar')).toBe(sidebar);
    expect(view.container.querySelector('.topbar')).toBe(topbar);
    expect(view.container.querySelector('#main-content')).toBe(main);
    expect(sidebar).toHaveTextContent('Libraries');
    expect(topbar).toHaveTextContent('Portico Review');
    expect(view.container.querySelector('.server-card')).not.toBeInTheDocument();
  });

  it('keeps account and server controls available while the selected server is unavailable', async () => {
    render(<MemoryRouter><RuntimeProductFrame><div aria-label="Server recovery" /></RuntimeProductFrame></MemoryRouter>);

    const profileButton = screen.getByRole('button', { name: 'Open account menu for Portico Review' });
    expect(profileButton).toBeEnabled();
    fireEvent.click(profileButton);

    expect(screen.getByRole('menuitem', { name: 'Account settings' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Account settings' }));
    expect(await screen.findByRole('heading', { name: 'Portico Account settings' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Close account settings' }));

    fireEvent.click(profileButton);
    fireEvent.click(screen.getByRole('menuitem', { name: 'Choose another server' }));
    expect(runtime.reselectServer).toHaveBeenCalledOnce();

    fireEvent.click(profileButton);
    fireEvent.click(screen.getByRole('menuitem', { name: 'Sign out' }));
    expect(runtime.hostedLogout).toHaveBeenCalledOnce();
  });

  it('renders the zero-server account inside normal interactive product chrome', async () => {
    runtime.state = { id: 'no-memberships', startedAt: 0, retryable: false, recoveryActions: ['sign-out'] } as never;
    const view = render(<MemoryRouter><RuntimeProductFrame><div aria-label="Claim or accept a server" /></RuntimeProductFrame></MemoryRouter>);

    expect(view.container.querySelector('.sidebar')).toBeVisible();
    expect(screen.getByRole('navigation', { name: 'Primary navigation' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /No server connected.*Claim or accept access/ })).toBeInTheDocument();
    expect(screen.getByLabelText('Claim or accept a server')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Libraries' }));
    expect(screen.getByRole('heading', { name: 'No libraries available' })).toBeInTheDocument();
    expect(screen.getByText('Claim a server or accept an invitation to browse its libraries.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Add a server' }));
    expect(screen.getByLabelText('Claim or accept a server')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Open account menu for Portico Review' }));
    expect(screen.getByRole('menuitem', { name: 'Account settings' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Account settings' }));
    expect(await screen.findByRole('heading', { name: 'Portico Account settings' })).toBeInTheDocument();
  });

  it('keeps full interactive chrome and a flat retry canvas when an authenticated server is unreachable', async () => {
    runtime.state = {
      id: 'runtime-recovery',
      startedAt: 0,
      retryable: true,
      recoveryActions: ['retry', 'reselect-server', 'sign-out'],
      classification: 'server-offline',
      messageId: 'problem.server-unavailable',
      serverName: 'Portico Review',
      selectedServer: { id: 'server-1', name: 'Portico Review', assignedHostname: 'review.direct.getportico.tv', remoteAccessEnabled: true, preferredAuthMode: 'portico' },
    } as never;
    const view = render(<MemoryRouter><RuntimeProductFrame><div aria-label="Legacy recovery card" /></RuntimeProductFrame></MemoryRouter>);

    expect(screen.getByRole('navigation', { name: 'Primary navigation' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Portico Review.*Connection unavailable/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Portico Review is unavailable' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Legacy recovery card')).not.toBeInTheDocument();
    expect(view.container.querySelector('.runtime-unavailable-destination')?.parentElement).toHaveAttribute('id', 'main-content');

    fireEvent.click(screen.getByRole('button', { name: 'Libraries' }));
    expect(screen.getByRole('heading', { name: 'No libraries available' })).toBeInTheDocument();
    expect(screen.getByText(/Retry the secure connection/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry connection' }));
    expect(runtime.retry).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole('button', { name: 'Open account menu for Portico Review' }));
    expect(screen.getByRole('menuitem', { name: 'Account settings' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'Choose another server' })).toBeInTheDocument();
  });

  it('never fabricates a Portico-named account or server when presentation data is absent', () => {
    runtime.restoredPresentation = undefined as never;
    runtime.state = { id: 'runtime-recovery', startedAt: 0, retryable: true, recoveryActions: ['retry', 'sign-out'], classification: 'server-offline', messageId: 'problem.server-unavailable' } as never;
    const view = render(<MemoryRouter><RuntimeProductFrame><div aria-label="Account recovery" /></RuntimeProductFrame></MemoryRouter>);

    expect(view.container.querySelector('.server-card')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Account menu unavailable' })).toBeDisabled();
    expect(screen.getByText('No server selected')).toBeInTheDocument();
  });
});
