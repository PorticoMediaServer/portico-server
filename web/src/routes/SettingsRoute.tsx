import { StatusWarningIcon } from '#portico-icons';
import { useMemo } from 'react';
import { usePorticoDataSource } from '../data/DataProvider';
import { HttpPorticoDataSource } from '../data/httpSource';
import type { Viewer } from '../data/models';
import { HttpFilesystemSource, type FilesystemPickerSource } from '../features/filesystem';
import { HttpSettingsDataSource } from '../features/settings/HttpSettingsDataSource';
import { SettingsPage } from '../features/settings/SettingsPage';
import type { SettingsDataSource, SettingsViewer } from '../features/settings/settingsTypes';

function providesDevelopmentSettings(value: unknown): value is { settingsDataSource: () => SettingsDataSource; filesystemSource?: () => FilesystemPickerSource } {
  if (!import.meta.env.DEV || !value || typeof value !== 'object') return false;
  return typeof (value as { settingsDataSource?: unknown }).settingsDataSource === 'function';
}

export function settingsDataSourceFor(
  options: { source: unknown },
): SettingsDataSource | undefined {
  const { source } = options;
  if (source instanceof HttpPorticoDataSource) {
    return new HttpSettingsDataSource(
      source.porticoClient(),
      source.authoritativeHostedClient,
      {
        authoritativeServerId: source.authoritativeHostedServerId,
      },
    );
  }
  if (providesDevelopmentSettings(source)) return source.settingsDataSource();
  return undefined;
}

export function SettingsRoute({ viewer }: { viewer: Viewer }) {
  const source = usePorticoDataSource();
  const settingsResult = useMemo<{ source?: SettingsDataSource; error?: unknown }>(
    () => {
      try { return { source: settingsDataSourceFor({ source }) }; }
      catch (error) { return { error }; }
    },
    [source],
  );
  const settingsSource = settingsResult.source;
  const filesystemSource = useMemo<FilesystemPickerSource | undefined>(() => {
    if (source instanceof HttpPorticoDataSource) return new HttpFilesystemSource(source.porticoClient());
    if (providesDevelopmentSettings(source)) return source.filesystemSource?.();
    return undefined;
  }, [source]);
  const user = viewer.user;
  if (!user) {
    return <div className="portico-settings-page"><div className="portico-settings-state error"><StatusWarningIcon /><strong>Account settings are unavailable</strong><p>Sign in to this server again before opening Settings.</p></div></div>;
  }
  if (settingsResult.error) {
    return <div className="portico-settings-page"><div className="portico-settings-state error" role="alert"><StatusWarningIcon /><strong>Settings configuration is unavailable</strong><p>Reconnect to this server before opening Settings again.</p></div></div>;
  }
  if (!settingsSource) {
    return <div className="portico-settings-page"><div className="portico-settings-state error"><StatusWarningIcon /><strong>Settings aren’t supported by this connection</strong><p>Reconnect to a compatible Portico server and try again.</p></div></div>;
  }
  const settingsViewer: SettingsViewer = {
    id: user.id,
    serverId: viewer.viewerScope?.serverId,
    displayName: user.displayName,
    email: user.email,
    role: user.role === 'owner' ? 'owner' : 'user',
    serverName: viewer.serverName,
    profileImageUrl: user.profileImageUrl,
    authOrigin: user.authOrigin,
    authProvider: user.authProvider,
    hasLocalPassword: user.hasLocalPassword,
    permissions: user.permissions,
  };
  return <SettingsPage source={settingsSource} productSource={source} filesystemSource={filesystemSource} viewer={settingsViewer} />;
}
