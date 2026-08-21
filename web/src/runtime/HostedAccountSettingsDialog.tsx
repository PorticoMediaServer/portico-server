import { createHostedServicesClient, createPorticoClient, type PorticoAccountUser } from '@portico/client-core';
import { RefreshCw, X } from '#portico-icons';
import { useCallback, useMemo, useState } from 'react';
import { IconButton, SecondaryButton } from '../components/controls/Buttons';
import { ModalOverlay } from '../components/overlay/OverlayPortal';
import { AccountSettings } from '../features/settings/PersonalSettings';
import { HttpSettingsDataSource } from '../features/settings/HttpSettingsDataSource';
import { SettingsError, SettingsLoading } from '../features/settings/SettingsControls';
import { useSettingsQuery } from '../features/settings/settingsHooks';
import type { SettingsDataSource, SettingsViewer } from '../features/settings/settingsTypes';
import { hostedCSRFToken, rememberHostedCSRFToken } from './hostedBrowserSecurity';
import { useRuntime } from './RuntimeContext';
import '../features/settings/settings.css';

const unavailableServerClient = createPorticoClient({
  transport: {
    fetch: async () => {
      throw new TypeError('No server connection is available for this account-only action.');
    },
  },
});

function accountViewer(user: PorticoAccountUser): SettingsViewer {
  return {
    id: user.id,
    displayName: user.username,
    email: user.email,
    role: 'user',
    serverName: 'Portico Account',
    profileImageUrl: user.profileImageUrl,
    authOrigin: 'portico',
    authProvider: 'portico',
    hasLocalPassword: false,
    permissions: {},
  };
}

function parseAccountUser(value: unknown): PorticoAccountUser {
  if (!value || typeof value !== 'object') throw new TypeError('Portico Account data is unavailable.');
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.id !== 'string' || typeof candidate.email !== 'string') {
    throw new TypeError('Portico Account data is incomplete.');
  }
  if (typeof candidate.username !== 'string' && typeof candidate.displayName !== 'string') {
    throw new TypeError('Portico Account username is unavailable.');
  }
  return value as PorticoAccountUser;
}

export function HostedAccountSettingsDialog({ onDismiss }: { onDismiss: () => void }) {
  const runtime = useRuntime();
  const [revision, setRevision] = useState(0);
  const hosted = useMemo(() => createHostedServicesClient({
    hostedApiBaseUrl: runtime.config.hostedApiBaseUrl,
    csrfToken: hostedCSRFToken,
    onCSRFToken: rememberHostedCSRFToken,
  }), [runtime.config.hostedApiBaseUrl]);
  const source = useMemo(() => new HttpSettingsDataSource(
    unavailableServerClient,
    hosted,
    { syncHostedIdentityToServer: false },
  ), [hosted]);
  const load = useCallback(async (_source: SettingsDataSource, signal: AbortSignal) => {
    const response = await hosted.account();
    if (signal.aborted) throw signal.reason;
    return accountViewer(parseAccountUser(response.user));
  }, [hosted]);
  const query = useSettingsQuery(load, source, revision);

  return <ModalOverlay className="portico-settings-dialog runtime-account-settings-dialog" labelledBy="runtime-account-settings-title" onDismiss={onDismiss}>
    <header>
      <div>
        <h2 id="runtime-account-settings-title">Portico Account settings</h2>
        <p>These settings remain available even when your media server is offline.</p>
      </div>
      <IconButton label="Close account settings" onClick={onDismiss}><X /></IconButton>
    </header>
    <div className="runtime-account-settings-content">
      {query.status === 'loading' && <SettingsLoading label="Loading Portico Account settings" />}
      {query.status === 'error' && <SettingsError title="Account settings are unavailable" message="Portico couldn’t load your account settings right now." onRetry={() => setRevision((value) => value + 1)} />}
      {query.status === 'success' && query.data && <AccountSettings viewer={query.data} source={source} />}
    </div>
    {query.status === 'error' && <footer><SecondaryButton onClick={() => setRevision((value) => value + 1)}><RefreshCw /> Try again</SecondaryButton></footer>}
  </ModalOverlay>;
}
