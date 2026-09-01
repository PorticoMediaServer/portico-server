import { useEffect, useState } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { createMemoryRouter, MemoryRouter, RouterProvider, useLocation, useNavigate } from 'react-router-dom';
import { SaveBar, SettingsSaveCoordinator, useSettingsNavigationGuard } from './SettingsControls';
import { SettingsNavigationBlocker } from './SettingsNavigationBlocker';
import { setSettingsNavigationDirty } from './settingsNavigationGuard';

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((next) => { resolve = next; });
  return { promise, resolve };
}

function RegisteredSave({ wait, fail = false, label }: { wait: Promise<void>; fail?: boolean; label: string }) {
  const [dirty, setDirty] = useState(true);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  return <SaveBar
    dirty={dirty}
    busy={busy}
    feedback={feedback}
    error={error}
    onReset={() => setDirty(false)}
    onSave={async () => {
      setBusy(true);
      await wait;
      if (fail) setError(`${label} failed`);
      else {
        setDirty(false);
        setFeedback(`${label} saved`);
      }
      setBusy(false);
    }}
  />;
}

function GuardedNavigation() {
  const requestNavigation = useSettingsNavigationGuard();
  const location = useLocation();
  return <><span>{location.pathname}</span><button type="button" onClick={() => requestNavigation?.('/settings/account')}>Open account</button></>;
}

function ProgrammaticNavigation() {
  const navigate = useNavigate();
  const location = useLocation();
  return <><span>{location.pathname}</span><button type="button" onClick={() => navigate('/profile')}>Open profile</button></>;
}

function SensitiveAPIKeyToken() {
  useEffect(() => {
    setSettingsNavigationDirty(true, 'api-key-token');
    return () => setSettingsNavigationDirty(false, 'api-key-token');
  }, []);
  return null;
}

describe('SettingsSaveCoordinator', () => {
  it('waits for every dirty registration before confirming success', async () => {
    const first = deferred();
    const second = deferred();
    render(<MemoryRouter><SettingsSaveCoordinator>
      <RegisteredSave label="First" wait={first.promise} />
      <RegisteredSave label="Second" wait={second.promise} />
    </SettingsSaveCoordinator></MemoryRouter>);

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    first.resolve();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Saving…' })).toBeDisabled());
    expect(screen.queryByText('Settings Saved')).not.toBeInTheDocument();

    second.resolve();
    expect(await screen.findByText('Settings Saved')).toBeInTheDocument();
  });

  it('does not confirm success when any registered save reports an error', async () => {
    const first = deferred();
    const second = deferred();
    render(<MemoryRouter><SettingsSaveCoordinator>
      <RegisteredSave label="First" wait={first.promise} />
      <RegisteredSave label="Second" wait={second.promise} fail />
    </SettingsSaveCoordinator></MemoryRouter>);

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    first.resolve();
    second.resolve();

    expect(await screen.findAllByText('Second failed')).not.toHaveLength(0);
    expect(screen.queryByText('Settings Saved')).not.toBeInTheDocument();
  });

  it('requires an explicit decision before internal navigation with dirty settings', async () => {
    render(<MemoryRouter initialEntries={['/settings/playback']}><SettingsSaveCoordinator>
      <RegisteredSave label="Playback" wait={Promise.resolve()} />
      <GuardedNavigation />
    </SettingsSaveCoordinator></MemoryRouter>);

    fireEvent.click(screen.getByRole('button', { name: 'Open account' }));
    expect(screen.getByRole('dialog', { name: 'Unsaved settings' })).toHaveAttribute('aria-describedby', 'settings-unsaved-description');
    expect(screen.getByText('/settings/playback')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Stay' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Open account' }));
    fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
    expect(await screen.findByText('/settings/account')).toBeInTheDocument();
  });

  it('treats Escape as Stay and restores focus to the navigation trigger', async () => {
    render(<MemoryRouter initialEntries={['/settings/playback']}><SettingsSaveCoordinator>
      <RegisteredSave label="Playback" wait={Promise.resolve()} />
      <GuardedNavigation />
    </SettingsSaveCoordinator></MemoryRouter>);

    const trigger = screen.getByRole('button', { name: 'Open account' });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = screen.getByRole('dialog', { name: 'Unsaved settings' });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Stay' })).toHaveFocus());
    expect(dialog).toHaveAttribute('aria-modal', 'true');

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Unsaved settings' })).not.toBeInTheDocument();
    expect(screen.getByText('/settings/playback')).toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('blocks browser history and programmatic navigation outside Settings', async () => {
    const router = createMemoryRouter([{
      path: '*',
      element: <><SettingsNavigationBlocker /><SettingsSaveCoordinator><RegisteredSave label="Playback" wait={Promise.resolve()} /><ProgrammaticNavigation /></SettingsSaveCoordinator></>,
    }], { initialEntries: ['/home', '/settings/playback'], initialIndex: 1 });
    render(<RouterProvider router={router} />);

    await router.navigate(-1);
    expect(await screen.findByRole('dialog', { name: 'Unsaved settings' })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe('/settings/playback');
    fireEvent.click(screen.getByRole('button', { name: 'Stay' }));
    expect(router.state.location.pathname).toBe('/settings/playback');

    fireEvent.click(screen.getByRole('button', { name: 'Open profile' }));
    expect(await screen.findByRole('dialog', { name: 'Unsaved settings' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
    await waitFor(() => expect(router.state.location.pathname).toBe('/profile'));
  });

  it('does not offer a destructive navigation choice while an API-key token is visible', async () => {
    const router = createMemoryRouter([{
      path: '*',
      element: <><SettingsNavigationBlocker /><SettingsSaveCoordinator><SensitiveAPIKeyToken /><ProgrammaticNavigation /></SettingsSaveCoordinator></>,
    }], { initialEntries: ['/settings/people'] });
    render(<RouterProvider router={router} />);

    fireEvent.click(screen.getByRole('button', { name: 'Open profile' }));
    expect(await screen.findByRole('dialog', { name: 'Save your API key' })).toBeInTheDocument();
    expect(screen.getByText(/copy it or confirm that you saved it before leaving/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Discard' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save and continue' })).not.toBeInTheDocument();
    expect(router.state.location.pathname).toBe('/settings/people');

    fireEvent.click(screen.getByRole('button', { name: 'Stay' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(router.state.location.pathname).toBe('/settings/people');
  });
});
