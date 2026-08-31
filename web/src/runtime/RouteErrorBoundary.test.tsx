import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { productMessage } from '@porticomediaserver/client-core';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { ConnectionStatusToast } from '../app/AppShell';
import { logRouteRenderFailure, RouteErrorBoundary } from './RouteErrorBoundary';
import { readStoredRouteDiagnostics, recordRouteDataState } from './routeDiagnostics';

function Broken({ broken }: { broken: boolean }) {
  if (broken) throw new Error('Sensitive implementation detail');
  return <h1>Library ready</h1>;
}

function Harness() {
  const [broken, setBroken] = useState(true);
  return <><button type="button" onClick={() => setBroken(false)}>Repair</button><RouteErrorBoundary routeKey="/library"><Broken broken={broken} /></RouteErrorBoundary></>;
}

function BlockingPresentationHarness() {
  const [blocking, setBlocking] = useState(false);
  return <>
    <ConnectionStatusToast presentation={productMessage('problem.cloud-unavailable')} blocking={blocking} />
    <RouteErrorBoundary routeKey="/" onBlockingStateChange={setBlocking}><Broken broken /></RouteErrorBoundary>
  </>;
}

describe('route error boundary', () => {
  it('keeps failure copy safe and can retry the current route', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    render(<Harness />);
    expect(screen.getByRole('alert')).toHaveTextContent("Portico couldn't complete this request");
    expect(screen.queryByText('Sensitive implementation detail')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Repair' }));
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(screen.getByRole('heading', { name: 'Library ready' })).toBeInTheDocument();
  });

  it('does not duplicate a centered blocking failure with the transient connection toast', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    render(<BlockingPresentationHarness />);

    expect(screen.getByRole('alert')).toHaveTextContent("Portico couldn't complete this request");
    await waitFor(() => expect(screen.queryByRole('status')).not.toBeInTheDocument());
    expect(document.querySelector('.connection-durability-warning')).not.toBeInTheDocument();
  });

  it('logs production diagnostics without route identifiers or error messages', () => {
    window.localStorage.clear();
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    recordRouteDataState('media:med_secret_identifier', 'error', { requestId: 'request-abc' });
    logRouteRenderFailure(
      new Error('secret title and token'),
      { componentStack: '\n at PrivateView' },
      '/media/med_secret_identifier?token=private',
      false,
    );
    expect(spy).toHaveBeenCalledWith('Portico route render failed.', expect.objectContaining({
      routeFamily: '/media/:id',
      fingerprint: expect.stringMatching(/^view-[a-f0-9]{8}$/),
    }));
    expect(JSON.stringify(spy.mock.calls)).not.toContain('secret title');
    expect(JSON.stringify(spy.mock.calls)).not.toContain('med_secret_identifier');
    expect(JSON.stringify(spy.mock.calls)).not.toContain('token=private');
    expect(readStoredRouteDiagnostics()).toEqual([expect.objectContaining({
      routeFamily: '/media/:id',
      dataState: expect.objectContaining({ error: 1, recentResources: expect.arrayContaining(['media']) }),
      correlationIds: ['request-abc'],
    })]);
    expect(window.localStorage.getItem('portico.route-diagnostics.v1')).not.toContain('med_secret_identifier');
    expect(window.localStorage.getItem('portico.route-diagnostics.v1')).not.toContain('secret title');
  });
});
