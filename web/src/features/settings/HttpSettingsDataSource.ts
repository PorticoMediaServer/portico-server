import {
  ApiError,
  createHostedServicesClient,
  HostedTerminalMutationCommittedError,
  type AccountSession,
  type APIKey,
  type APIKeyCreateResponse,
  type BackupInfo,
  type Device,
  type DLNAStatus,
  type Job,
  type Library,
  type AdminLibraryChannelListResponse,
  type LibraryChannelAggregate,
  type LibraryChannelConfigurationRequest,
  type LibraryChannelGeneration,
  type LibraryChannelLogoAsset,
  type LibraryChannelRestoreDefaultsRequest,
  type LibraryChannelRestoreDefaultsResponse,
  type LibraryChannelTemplatesResponse,
  type LiveTVSource,
  type LiveTVSourceRequest,
  type ListResponse,
  type LogEvent,
  type HostedServicesClient,
  type PorticoClient,
  type PorticoAccountUser,
  type PorticoMFAStatus,
  type PorticoInvite,
  type PlaybackSession,
  type RemoteAccessSettingsPatch,
  type RemoteAccessStatus,
  type RemoteStorageSource,
  type RemoteStorageAnalysisMode,
  type RemoteStorageSourceRequest,
  type ScheduledTask,
  type ScheduledTaskRunResponse,
  type ScheduledTaskUpdateRequest,
  type SettingsDocument,
  type SettingsSummaryResponse,
  type SettingsUpdateRequest,
  type SystemStorageCleanupResponse,
  type SystemStorageReport,
  type TranscodeCapacityReport,
  type User,
  type UserCreateRequest,
  type UserPatchRequest,
} from "@porticomediaserver/client-core";
import type {
  AccountIdentitySnapshot,
  AccountSignedInDevice,
  AccountMFAEnableResult,
  AccountMFASetup,
  AccountMFAStatus,
  AccountOrigin,
  LibraryMutationInput,
  LibraryScanMode,
  LibraryScanOperationsResponse,
  LibraryScanReviewResponse,
  LibraryStorageSource,
  IdentityReconciliationReview,
  PorticoMemberInviteInput,
  DVRStatusSnapshot,
  SettingsDataSource,
  SettingsOperationalSnapshot,
  SettingsOperationalScope,
  SettingsStatusSnapshot,
  RestoreWorkflowResponse,
  RestoreStepUp,
} from "./settingsTypes";
import {
  hostedCSRFToken,
  rememberHostedCSRFToken,
} from "../../runtime/hostedBrowserSecurity";
import { browserHostedTerminalMutationDurability } from "../../runtime/hostedTerminalMutationDurability";
import { reviewedProductErrorText } from "../../components/ProductLanguage";

function reasonMessage(reason: unknown, sectionName: string): string {
  return reviewedProductErrorText(reason, "settings.load-failed", {
    sectionName,
  });
}

function abortIfRequested(signal: AbortSignal): void {
  if (!signal.aborted) return;
  if (signal.reason instanceof Error) throw signal.reason;
  throw new DOMException("The request was cancelled.", "AbortError");
}

function hostedApiBaseUrl(): string {
  if (
    typeof window !== "undefined" &&
    window.__PORTICO_CONFIG__?.hostedApiBaseUrl
  ) {
    return window.__PORTICO_CONFIG__.hostedApiBaseUrl;
  }
  return String(
    import.meta.env.VITE_PORTICO_HOSTED_API_URL || "https://api.getportico.tv",
  ).replace(/\/+$/, "");
}

function mfaBoolean(value: unknown): boolean {
  return value === true;
}

function normalizeMFAStatus(value: PorticoMFAStatus): AccountMFAStatus {
  const source = value as Record<string, unknown>;
  const remaining =
    typeof source.recoveryCodesRemaining === "number" &&
    Number.isFinite(source.recoveryCodesRemaining)
      ? Math.max(0, Math.trunc(source.recoveryCodesRemaining))
      : undefined;
  return {
    enabled: mfaBoolean(source.enabled),
    setupStarted: mfaBoolean(source.setupStarted),
    recoveryCodesSupported: source.recoveryCodesSupported !== false,
    recoveryCodesRemaining: remaining,
  };
}

export class HttpSettingsDataSource implements SettingsDataSource {
  private readonly hosted: HostedServicesClient;
  private readonly syncHostedIdentityToServer: boolean;

  constructor(
    private readonly client: PorticoClient,
    hosted?: HostedServicesClient,
    options: {
      syncHostedIdentityToServer?: boolean;
      authoritativeServerId?: string;
    } = {},
  ) {
    this.hosted =
      hosted ??
      createHostedServicesClient({
        hostedApiBaseUrl,
        csrfToken: hostedCSRFToken,
        onCSRFToken: rememberHostedCSRFToken,
        terminalMutationDurabilityAdapter:
          browserHostedTerminalMutationDurability,
      });
    this.syncHostedIdentityToServer =
      options.syncHostedIdentityToServer !== false;
    const authoritativeServerId: unknown = options.authoritativeServerId;
    if (authoritativeServerId !== undefined && typeof authoritativeServerId !== "string") {
      throw new TypeError("Portico Settings server identity has an invalid runtime shape.");
    }
    this.authoritativeServerId = authoritativeServerId?.trim() ?? "";
  }

  private readonly authoritativeServerId: string;

  private hostedProfileImageUrl(value?: string): string {
    const path = value?.trim() ?? "";
    if (
      !path ||
      /^https?:\/\//i.test(path) ||
      path.startsWith("data:") ||
      path.startsWith("blob:")
    )
      return path;
    return path.startsWith("/") ? this.hosted.hostedApiUrl(path) : path;
  }

  private async syncHostedIdentity(
    user: PorticoAccountUser,
    signal: AbortSignal,
  ): Promise<AccountIdentitySnapshot> {
    const identity: AccountIdentitySnapshot = {
      displayName: user.username,
      email: user.email,
      profileImageUrl: this.hostedProfileImageUrl(user.profileImageUrl),
    };
    if (!this.syncHostedIdentityToServer) return identity;
    try {
      await this.client.request<User>("/api/account/profile", {
        method: "PATCH",
        body: identity,
        signal,
      });
      return identity;
    } catch {
      abortIfRequested(signal);
      return {
        ...identity,
        serverSyncWarning:
          "Your Portico Account was updated, but this server could not receive the latest profile yet. Try reconnecting later.",
      };
    }
  }

  settings(signal: AbortSignal): Promise<SettingsDocument> {
    return this.client.settings({ signal });
  }

  settingsSummary(signal: AbortSignal): Promise<SettingsSummaryResponse> {
    return this.client.settingsSummary({ signal });
  }

  updateSettings(
    input: SettingsUpdateRequest,
    signal: AbortSignal,
  ): Promise<SettingsDocument> {
    return this.client.updateSettings(input, { signal });
  }

  async settingsStatus(signal: AbortSignal): Promise<SettingsStatusSnapshot> {
    const keys = [
      "activity",
      "dashboard",
      "storage",
      "remoteAccess",
      "jobs",
    ] as const;
    const results = await Promise.allSettled([
      this.client.dashboardActivity({ signal }),
      this.client.dashboard({ mode: "live", period: "1h" }, { signal }),
      this.client.systemStorage(undefined, { signal }),
      this.client.remoteAccessStatus({ signal }),
      this.client
        .activity({ limit: 12 }, { signal })
        .then((response) => response.items),
    ] as const);
    abortIfRequested(signal);

    const snapshot: SettingsStatusSnapshot = {
      generatedAt: new Date().toISOString(),
    };
    const failures: SettingsStatusSnapshot["failures"] = {};
    results.forEach((result, index) => {
      const key = keys[index];
      if (result.status === "rejected")
        failures[key] = reasonMessage(result.reason, "Server status");
    });
    if (results[0]?.status === "fulfilled")
      snapshot.activity = results[0].value;
    if (results[1]?.status === "fulfilled")
      snapshot.dashboard = results[1].value;
    if (results[2]?.status === "fulfilled") snapshot.storage = results[2].value;
    if (results[3]?.status === "fulfilled")
      snapshot.remoteAccess = results[3].value;
    if (results[4]?.status === "fulfilled") snapshot.jobs = results[4].value;
    if (Object.keys(failures).length > 0) snapshot.failures = failures;
    return snapshot;
  }

  runConnectivityCheck(signal: AbortSignal): Promise<RemoteAccessStatus> {
    return this.client.request<RemoteAccessStatus>(
      "/api/remote-access/test-direct",
      { method: "POST", signal },
    );
  }

  async stopPlayback(session: PlaybackSession, signal: AbortSignal): Promise<void> {
    try {
      const positionSeconds = Math.max(0, session.positionSeconds);
      await this.client.stopPlayback(session.id, {
        disposition: "stopped",
        positionSeconds,
        durationSeconds: Math.max(0, session.media.durationSeconds ?? positionSeconds),
      }, { signal });
    } catch (reason) {
      if (!(reason instanceof ApiError) || reason.status !== 404) throw reason;
    }
  }

  remoteAccess(signal: AbortSignal): Promise<RemoteAccessStatus> {
    return this.client.remoteAccessStatus({ signal });
  }

  updateRemoteAccess(
    input: RemoteAccessSettingsPatch,
    signal: AbortSignal,
  ): Promise<RemoteAccessStatus> {
    return this.client.request<RemoteAccessStatus>(
      "/api/remote-access/settings",
      { method: "PATCH", body: input, signal },
    );
  }

  startRemoteAccessClaim(signal: AbortSignal): Promise<RemoteAccessStatus> {
    return this.client.request<RemoteAccessStatus>(
      "/api/remote-access/claim/start",
      { method: "POST", signal },
    );
  }

  cancelRemoteAccessClaim(signal: AbortSignal): Promise<RemoteAccessStatus> {
    return this.client.request<RemoteAccessStatus>(
      "/api/remote-access/claim/cancel",
      { method: "POST", signal },
    );
  }

  async unclaimRemoteAccess(signal: AbortSignal): Promise<RemoteAccessStatus> {
    const current = await this.client.remoteAccessStatus({ signal });
    const serverId = current.settings.serverId.trim();
    if (serverId) {
      try {
        await this.hosted.request<{ ok: boolean }>(
          `/api/account/servers/${encodeURIComponent(serverId)}`,
          { method: "DELETE", signal },
        );
      } catch (reason) {
        // A prior attempt may have committed Hosted deletion before its
        // response was lost. Missing is therefore terminal success; local
        // unclaim must still run so the server cannot be stranded.
        if (!(reason instanceof ApiError) || reason.status !== 404)
          throw reason;
      }
    }
    return this.client.request<RemoteAccessStatus>(
      "/api/remote-access/unclaim",
      { method: "POST", signal },
    );
  }

  renewRemoteAccessCertificate(
    signal: AbortSignal,
  ): Promise<RemoteAccessStatus> {
    return this.client.request<RemoteAccessStatus>(
      "/api/remote-access/certificates/renew",
      { method: "POST", signal },
    );
  }

  async settingsOperations(
    scope: SettingsOperationalScope,
    signal: AbortSignal,
  ): Promise<SettingsOperationalSnapshot> {
    const required = (...scopes: SettingsOperationalScope[]) =>
      scopes.includes(scope);
    const empty = <T>(value: T): Promise<T> => Promise.resolve(value);
    type Panel = keyof NonNullable<SettingsOperationalSnapshot['failures']>;
    type SettledPanel<T> = { panel: Panel; value?: T; error?: string };
    const settle = async <T>(panel: Panel, promise: Promise<T>): Promise<SettledPanel<T>> => {
      try {
        return { panel, value: await promise };
      } catch (reason) {
        abortIfRequested(signal);
        return { panel, error: reasonMessage(reason, "Settings") };
      }
    };
    const usersPromise = required("people")
      ? this.client.users({ signal })
      : empty({ items: [] as User[], total: 0 });
    // A runtime-published server identity is already account-fenced authority;
    // it must not wait for a failing Server read before Hosted invitations can
    // load. Bundled connections without that identity still derive eligibility
    // from the canonical Server owner row before resolving remote-access state.
    const invitesPromise = required("people")
      ? (this.authoritativeServerId ? (async () => {
          const serverId = await this.remoteAccessServerID(signal);
          const invitePage = await this.hosted.request<{
            items: PorticoInvite[];
          }>(
            `/api/account/servers/${encodeURIComponent(serverId)}/invites?limit=100`,
            { signal },
          );
          return invitePage.items;
        })() : usersPromise.then(async (users) => {
          if (!users.items.some((user) => user.role === "owner" && user.authOrigin === "portico")) return [] as PorticoInvite[];
          const serverId = await this.remoteAccessServerID(signal);
          const invitePage = await this.hosted.request<{ items: PorticoInvite[] }>(
            `/api/account/servers/${encodeURIComponent(serverId)}/invites?limit=100`,
            { signal },
          );
          return invitePage.items;
        }))
      : empty([] as PorticoInvite[]);
    const panels = await Promise.all([
      settle('libraries', required("media", "people")
        ? this.client.libraries({ signal })
        : empty({ items: [], total: 0 })),
      settle('users', usersPromise),
      settle('devices', required("people")
        ? this.client.devices({ signal })
        : empty({ items: [], total: 0 })),
      settle('apiKeys', required("people")
        ? this.client.apiKeys({ signal })
        : empty({ items: [], total: 0 })),
      settle('tasks', required("maintenance")
        ? this.client.scheduledTasks({ signal })
        : empty({ items: [], total: 0 })),
      settle('backups', required("maintenance")
        ? this.client.backups({ signal })
        : empty({ items: [], total: 0 })),
      settle('sessions', required("people")
        ? this.client.accountSessions({ signal })
        : empty({ items: [], total: 0 })),
      settle('release', required("diagnostics", "help")
        ? this.client.systemRelease({ signal })
        : empty({} as SettingsOperationalSnapshot["release"])),
      settle('diagnostics', required("diagnostics")
        ? this.client.systemDiagnostics({ signal })
        : empty({} as SettingsOperationalSnapshot["diagnostics"])),
      settle('capabilities', required("diagnostics", "people")
        ? this.client.request<SettingsOperationalSnapshot["capabilities"]>(
            "/api/system/capabilities",
            { signal },
          )
        : empty({} as SettingsOperationalSnapshot["capabilities"])),
      settle('storage', required("maintenance")
        ? this.client.systemStorage(undefined, { signal })
        : empty({} as SettingsOperationalSnapshot["storage"])),
    ]);
    abortIfRequested(signal);
    const byPanel = new Map(panels.map((panel) => [panel.panel, panel]));
    const failureMap: SettingsOperationalSnapshot['failures'] = {};
    for (const panel of panels) if (panel.error) failureMap[panel.panel] = panel.error;
    const page = <T,>(panel: Panel, fallback: T): T => byPanel.get(panel)?.value as T ?? fallback;
    const libraries = page('libraries', { items: [] as Library[], total: 0 });
    const users = page('users', { items: [] as User[], total: 0 });
    const devices = page('devices', { items: [] as Device[], total: 0 });
    const apiKeys = page('apiKeys', { items: [] as APIKey[], total: 0 });
    const tasks = page('tasks', { items: [] as ScheduledTask[], total: 0 });
    const backups = page('backups', { items: [] as BackupInfo[], total: 0 });
    const sessions = page('sessions', { items: [] as AccountSession[], total: 0 });
    const release = page('release', {} as SettingsOperationalSnapshot['release']);
    const diagnostics = page('diagnostics', {} as SettingsOperationalSnapshot['diagnostics']);
    const capabilities = page('capabilities', {} as SettingsOperationalSnapshot['capabilities']);
    const storage = page('storage', {} as SettingsOperationalSnapshot['storage']);
    let porticoInvites: PorticoInvite[] = [];
    try {
      porticoInvites = await invitesPromise;
    } catch (reason) {
      abortIfRequested(signal);
      failureMap.porticoInvites = reasonMessage(
        reason,
        "Portico Account invitations",
      );
    }
    return {
      libraries: libraries.items,
      users: users.items,
      devices: devices.items,
      apiKeys: apiKeys.items,
      tasks: tasks.items,
      backups: backups.items,
      sessions: sessions.items,
      release,
      diagnostics,
      capabilities,
      storage,
      porticoInvites,
      ...(Object.keys(failureMap).length > 0 ? { failures: failureMap } : {}),
    };
  }

  async liveTVSources(signal: AbortSignal): Promise<LiveTVSource[]> {
    const response = await this.client.request<ListResponse<LiveTVSource>>(
      "/api/live-tv/sources",
      { signal },
    );
    return response.items;
  }

  createLiveTVSource(
    input: LiveTVSourceRequest,
    testBeforeSaving: boolean,
    signal: AbortSignal,
  ): Promise<LiveTVSource> {
    return this.client.request<LiveTVSource>(
      testBeforeSaving
        ? "/api/live-tv/sources/test-add"
        : "/api/live-tv/sources",
      {
        method: "POST",
        body: input,
        signal,
      },
    );
  }

  updateLiveTVSource(
    id: string,
    input: LiveTVSourceRequest,
    signal: AbortSignal,
  ): Promise<LiveTVSource> {
    return this.client.request<LiveTVSource>(
      `/api/live-tv/sources/${encodeURIComponent(id)}`,
      { method: "PATCH", body: input, signal },
    );
  }

  refreshLiveTVSource(id: string, signal: AbortSignal): Promise<LiveTVSource> {
    return this.client.request<LiveTVSource>(
      `/api/live-tv/sources/${encodeURIComponent(id)}/refresh`,
      { method: "POST", signal },
    );
  }

  async deleteLiveTVSource(id: string, signal: AbortSignal): Promise<void> {
    await this.client.request<{ ok: boolean }>(
      `/api/live-tv/sources/${encodeURIComponent(id)}`,
      { method: "DELETE", signal },
    );
  }

  dvrStatus(
    sourceId: string | undefined,
    signal: AbortSignal,
  ): Promise<DVRStatusSnapshot> {
    return this.client.adminDvrOperationalStatus(sourceId, { signal });
  }

  libraryChannels(
    signal: AbortSignal,
  ): Promise<AdminLibraryChannelListResponse> {
    return this.client.request("/api/admin/library-channels?limit=250", {
      signal,
    });
  }

  libraryChannel(
    id: string,
    signal: AbortSignal,
  ): Promise<LibraryChannelAggregate> {
    return this.client.request(
      `/api/admin/library-channels/${encodeURIComponent(id)}`,
      { signal },
    );
  }

  createLibraryChannel(
    input: LibraryChannelConfigurationRequest,
    signal: AbortSignal,
  ): Promise<LibraryChannelAggregate> {
    return this.client.request("/api/admin/library-channels", {
      method: "POST",
      body: input,
      signal,
    });
  }

  updateLibraryChannel(
    id: string,
    input: LibraryChannelConfigurationRequest,
    signal: AbortSignal,
  ): Promise<LibraryChannelAggregate> {
    return this.client.request(
      `/api/admin/library-channels/${encodeURIComponent(id)}`,
      { method: "PUT", body: input, signal },
    );
  }

  async deleteLibraryChannel(
    id: string,
    expectedRevision: number,
    signal: AbortSignal,
  ): Promise<void> {
    await this.client.request(
      `/api/admin/library-channels/${encodeURIComponent(id)}?expectedRevision=${encodeURIComponent(String(expectedRevision))}`,
      { method: "DELETE", signal },
    );
  }

  libraryChannelTemplates(
    signal: AbortSignal,
  ): Promise<LibraryChannelTemplatesResponse> {
    return this.client.request("/api/admin/library-channels/templates", {
      signal,
    });
  }

  restoreLibraryChannelDefaults(
    input: LibraryChannelRestoreDefaultsRequest,
    signal: AbortSignal,
  ): Promise<LibraryChannelRestoreDefaultsResponse> {
    return this.client.request("/api/admin/library-channels/restore-defaults", {
      method: "POST",
      body: input,
      signal,
    });
  }

  regenerateLibraryChannel(
    id: string,
    signal: AbortSignal,
  ): Promise<LibraryChannelGeneration> {
    return this.client.request(
      `/api/admin/library-channels/${encodeURIComponent(id)}/regenerate`,
      { method: "POST", signal },
    );
  }

  async uploadLibraryChannelLogo(
    file: File,
    signal: AbortSignal,
  ): Promise<LibraryChannelLogoAsset> {
    abortIfRequested(signal);
    const form = new FormData();
    form.set("file", file);
    return this.client.uploadAdminLibraryChannelLogo(form);
  }

  transcodeCapacity(signal: AbortSignal): Promise<TranscodeCapacityReport> {
    return this.client.transcodeCapacity({ signal });
  }

  systemStorage(signal: AbortSignal): Promise<SystemStorageReport> {
    return this.client.systemStorage(undefined, { signal });
  }

  cleanupStorage(signal: AbortSignal): Promise<SystemStorageCleanupResponse> {
    return this.client.request<SystemStorageCleanupResponse>(
      "/api/system/storage/cleanup",
      { method: "POST", signal },
    );
  }

  dlnaStatus(signal: AbortSignal): Promise<DLNAStatus> {
    return this.client.dlnaStatus({ signal });
  }

  createLibrary(
    input: LibraryMutationInput,
    signal: AbortSignal,
  ): Promise<Library> {
    return this.client.request<Library>("/api/libraries", {
      method: "POST",
      body: input,
      signal,
    });
  }

  updateLibrary(
    id: string,
    input: LibraryMutationInput,
    signal: AbortSignal,
  ): Promise<Library> {
    return this.client.request<Library>(
      `/api/libraries/${encodeURIComponent(id)}`,
      { method: "PATCH", body: input, signal },
    );
  }

  async deleteLibrary(id: string, signal: AbortSignal): Promise<void> {
    await this.client.request<{ ok: boolean }>(
      `/api/libraries/${encodeURIComponent(id)}`,
      { method: "DELETE", signal },
    );
  }

  async remoteStorageSources(
    id: string,
    signal: AbortSignal,
  ): Promise<RemoteStorageSource[]> {
    return (await this.client.remoteStorageSources(id, { signal })).items;
  }

  createRemoteStorageSource(
    id: string,
    input: RemoteStorageSourceRequest,
    signal: AbortSignal,
  ): Promise<RemoteStorageSource> {
    return this.client.createRemoteStorageSource(id, input, { signal });
  }

  async deleteRemoteStorageSource(
    id: string,
    sourceId: string,
    signal: AbortSignal,
  ): Promise<void> {
    await this.client.deleteRemoteStorageSource(id, sourceId, { signal });
  }

  updateRemoteStorageSourceAnalysisMode(
    id: string,
    sourceId: string,
    analysisMode: RemoteStorageAnalysisMode,
    signal: AbortSignal,
  ): Promise<RemoteStorageSource> {
    return this.client.updateRemoteStorageSourceAnalysisMode(
      id,
      sourceId,
      { analysisMode },
      { signal },
    );
  }

  inventoryRemoteStorageSource(
    id: string,
    sourceId: string,
    signal: AbortSignal,
  ): Promise<Job> {
    return this.client.inventoryRemoteStorageSource(id, sourceId, { signal });
  }

  libraryScanOperations(
    id: string,
    signal: AbortSignal,
  ): Promise<LibraryScanOperationsResponse> {
    return this.client.request<LibraryScanOperationsResponse>(
      `/api/libraries/${encodeURIComponent(id)}/scan-operations`,
      { signal },
    );
  }

  libraryScanReview(
    id: string,
    cursor: string | undefined,
    signal: AbortSignal,
  ): Promise<LibraryScanReviewResponse> {
    const query = new URLSearchParams({ limit: "50" });
    if (cursor) query.set("cursor", cursor);
    return this.client.request<LibraryScanReviewResponse>(
      `/api/libraries/${encodeURIComponent(id)}/scan-review?${query}`,
      { signal },
    );
  }

  updateLibraryStorageClassification(
    libraryId: string,
    sourceId: string,
    classification: "local" | "network" | "fuse" | "unknown",
    signal: AbortSignal,
  ): Promise<LibraryStorageSource> {
    return this.client.request<LibraryStorageSource>(
      `/api/libraries/${encodeURIComponent(libraryId)}/storage-sources/${encodeURIComponent(sourceId)}`,
      { method: "PATCH", body: { classification }, signal },
    );
  }

  resolveIdentityReconciliationReview(
    reviewId: string,
    resolution: "keep_separate" | "merge_into_candidate",
    selectedCandidateId: string | undefined,
    signal: AbortSignal,
  ): Promise<IdentityReconciliationReview> {
    return this.client.request<IdentityReconciliationReview>(
      `/api/identity-reconciliation/reviews/${encodeURIComponent(reviewId)}/resolve`,
      {
        method: "POST",
        body: {
          resolution,
          ...(selectedCandidateId ? { selectedCandidateId } : {}),
        },
        signal,
      },
    );
  }

  scanLibrary(
    id: string,
    signal: AbortSignal,
    mode: LibraryScanMode = "reconcile",
    confirmedRunId?: string,
  ): Promise<Job> {
    return this.client.request<Job>(
      `/api/libraries/${encodeURIComponent(id)}/scan`,
      {
        method: "POST",
        body: { mode, ...(confirmedRunId ? { confirmedRunId } : {}) },
        signal,
      },
    );
  }

  cancelLibraryScan(libraryId: string, signal: AbortSignal): Promise<Job> {
    return this.client.request<Job>(
      `/api/libraries/${encodeURIComponent(libraryId)}/scan/cancel`,
      { method: "POST", signal },
    );
  }

  retryLibraryScan(
    libraryId: string,
    runId: string | undefined,
    signal: AbortSignal,
  ): Promise<Job> {
    return this.client.request<Job>(
      `/api/libraries/${encodeURIComponent(libraryId)}/scan/retry`,
      { method: "POST", body: runId ? { runId } : {}, signal },
    );
  }

  createUser(input: UserCreateRequest, signal: AbortSignal): Promise<User> {
    return this.client.request<User>("/api/users", {
      method: "POST",
      body: input,
      signal,
    });
  }

  async createPorticoMemberInvite(
    input: PorticoMemberInviteInput,
    signal: AbortSignal,
  ): Promise<{ inviteUrl?: string }> {
    const serverId = await this.remoteAccessServerID(signal);
    return this.hosted.request<{ inviteUrl?: string }>(
      `/api/account/servers/${encodeURIComponent(serverId)}/invites`,
      { method: "POST", body: input, signal },
    );
  }

  async resendPorticoMemberInvite(
    inviteId: string,
    signal: AbortSignal,
  ): Promise<PorticoInvite> {
    const serverId = await this.remoteAccessServerID(signal);
    return this.hosted.request<PorticoInvite>(
      `/api/account/servers/${encodeURIComponent(serverId)}/invites/${encodeURIComponent(inviteId)}/resend`,
      { method: "POST", signal },
    );
  }

  async revokePorticoMemberInvite(
    inviteId: string,
    signal: AbortSignal,
  ): Promise<void> {
    const serverId = await this.remoteAccessServerID(signal);
    try {
      await this.hosted.request(
        `/api/account/servers/${encodeURIComponent(serverId)}/invites/${encodeURIComponent(inviteId)}`,
        { method: "DELETE", signal },
      );
    } catch (reason) {
      if (!(reason instanceof HostedTerminalMutationCommittedError)) throw reason;
    }
  }

  async updateUser(
    user: User,
    input: UserPatchRequest,
    signal: AbortSignal,
  ): Promise<User> {
    if (user.authOrigin === "portico") {
      const membershipId = user.porticoMembershipId?.trim();
      if (!membershipId)
        throw new Error(
          "This Portico Account member is missing its Hosted membership identity. Refresh server access before editing it.",
        );
      const serverId = await this.remoteAccessServerID(signal);
      await this.hosted.request(
        `/api/account/servers/${encodeURIComponent(serverId)}/members/${encodeURIComponent(membershipId)}`,
        {
          method: "PATCH",
          body: {
            permissionTemplate: {
              permissions: input.permissions,
              maxContentRating: input.maxContentRating,
            },
          },
          signal,
        },
      );
      // Hosted owns generic grants only. Library access is this server's
      // local assignment and must never cross the Hosted boundary.
      await this.client.request<User>(
        `/api/users/${encodeURIComponent(user.id)}`,
        { method: "PATCH", body: { libraryIds: input.libraryIds }, signal },
      );
      await this.refreshRemoteAccessPolicy(signal);
      return { ...user, ...input } as User;
    }
    return this.client.request<User>(
      `/api/users/${encodeURIComponent(user.id)}`,
      { method: "PATCH", body: input, signal },
    );
  }

  async deleteUser(user: User, signal: AbortSignal): Promise<void> {
    if (user.authOrigin === "portico") {
      const membershipId = user.porticoMembershipId?.trim();
      if (!membershipId)
        throw new Error(
          "This Portico Account member is missing its Hosted membership identity. Refresh server access before removing it.",
        );
      const serverId = await this.remoteAccessServerID(signal);
      await this.hosted.request<{ ok: boolean }>(
        `/api/account/servers/${encodeURIComponent(serverId)}/members/${encodeURIComponent(membershipId)}`,
        { method: "DELETE", signal },
      );
      await this.refreshRemoteAccessPolicy(signal);
      return;
    }
    await this.client.request<{ ok: boolean }>(
      `/api/users/${encodeURIComponent(user.id)}`,
      { method: "DELETE", signal },
    );
  }

  private async remoteAccessServerID(signal: AbortSignal): Promise<string> {
    if (this.authoritativeServerId) return this.authoritativeServerId;
    const status = await this.client.remoteAccessStatus({ signal });
    const serverId = status.settings.serverId.trim();
    if (!serverId)
      throw new Error(
        "Connect this server to Portico before managing Portico Account members.",
      );
    return serverId;
  }

  private async refreshRemoteAccessPolicy(signal: AbortSignal): Promise<void> {
    await this.client.request<RemoteAccessStatus>(
      "/api/remote-access/policy-sync",
      { method: "POST", signal },
    );
  }

  updateDevice(
    id: string,
    input: Parameters<SettingsDataSource["updateDevice"]>[1],
    signal: AbortSignal,
  ): Promise<Device> {
    return this.client.request<Device>(
      `/api/devices/${encodeURIComponent(id)}`,
      { method: "PATCH", body: input, signal },
    );
  }

  async revokeDevice(id: string, signal: AbortSignal): Promise<void> {
    await this.client.request<{ ok: boolean }>(
      `/api/devices/${encodeURIComponent(id)}`,
      { method: "DELETE", signal },
    );
  }

  createAPIKey(
    input: { name: string; scopes: string[] },
    signal: AbortSignal,
  ): Promise<APIKeyCreateResponse> {
    return this.client.request<APIKeyCreateResponse>("/api/auth/api-keys", {
      method: "POST",
      body: input,
      signal,
    });
  }

  async revokeAPIKey(id: string, signal: AbortSignal): Promise<void> {
    await this.client.request<APIKey>(
      `/api/auth/api-keys/${encodeURIComponent(id)}`,
      { method: "DELETE", signal },
    );
  }

  updateScheduledTask(
    id: string,
    input: ScheduledTaskUpdateRequest,
    signal: AbortSignal,
  ): Promise<ScheduledTask> {
    return this.client.request<ScheduledTask>(
      `/api/tasks/${encodeURIComponent(id)}`,
      { method: "PATCH", body: input, signal },
    );
  }

  runScheduledTask(
    id: string,
    signal: AbortSignal,
  ): Promise<ScheduledTaskRunResponse> {
    return this.client.request<ScheduledTaskRunResponse>(
      `/api/tasks/${encodeURIComponent(id)}/run`,
      { method: "POST", signal },
    );
  }

  createBackup(signal: AbortSignal): Promise<BackupInfo> {
    return this.client.request<BackupInfo>("/api/backups", {
      method: "POST",
      signal,
    });
  }

  restoreBackup(
    name: string,
    stepUp: RestoreStepUp,
    confirmation: string,
    signal: AbortSignal,
  ): Promise<RestoreWorkflowResponse> {
    return this.restoreAuthorization(stepUp, signal).then((hostedAuthorization) =>
      this.client.restoreBackup(
        name,
        hostedAuthorization
          ? { confirmation, hostedAuthorization }
          : { confirmation, password: stepUp.password ?? "" },
        { signal },
      ),
    );
  }

  restoreUploadedDatabase(
    file: File,
    stepUp: RestoreStepUp,
    confirmation: string,
    signal: AbortSignal,
  ): Promise<RestoreWorkflowResponse> {
    return this.restoreAuthorization(stepUp, signal).then((hostedAuthorization) =>
      this.client.restoreUploadedDatabase(
        hostedAuthorization
          ? { file, confirmation, hostedAuthorization }
          : { file, confirmation, password: stepUp.password ?? "" },
        { signal },
      ),
    );
  }

  private async restoreAuthorization(stepUp: RestoreStepUp, signal: AbortSignal) {
    if (stepUp.origin !== "portico") return undefined;
    const [context, serverId] = await Promise.all([
      this.client.restoreAuthorizationContext({ signal }),
      this.remoteAccessServerID(signal),
    ]);
    return this.hosted.createServerRestoreAuthorization(
      serverId,
      {
        restoreSecurityEpoch: context.restoreSecurityEpoch,
        password: stepUp.password || undefined,
        mfaCode: stepUp.mfaCode || undefined,
        recoveryCode: stepUp.recoveryCode || undefined,
      },
      { signal },
    );
  }

  restoreStatus(
    operationId: string,
    statusToken: string,
    signal: AbortSignal,
  ): Promise<RestoreWorkflowResponse> {
    return this.client.request<RestoreWorkflowResponse>(
      `/api/backups/restore/${encodeURIComponent(operationId)}`,
      {
        signal,
        headers: { "X-Portico-Restore-Status": statusToken },
      },
    );
  }

  logs(
    input: { limit?: number; cursor?: string },
    signal: AbortSignal,
  ): Promise<ListResponse<LogEvent>> {
    return this.client.logs({ ...input, init: { signal } });
  }

  async signedInDevices(
    origin: AccountOrigin,
    signal: AbortSignal,
  ): Promise<AccountSignedInDevice[]> {
    if (origin === "portico") {
      abortIfRequested(signal);
      const response = await this.hosted.devices(
        { limit: 100, count: "exact" },
        { signal },
      );
      abortIfRequested(signal);
      return response.items
        .filter((device) => !device.revokedAt)
        .map((device) => ({
          id: device.id,
          authority: "portico" as const,
          name: device.name || device.platform || "Signed-in device",
          platform: device.platform,
          current: false,
          canRevoke: true,
          lastSeenAt: device.lastSeenAt,
        }));
    }
    const response = await this.client.accountSessions({ signal });
    return response.items.map((session) => ({
      id: session.id,
      authority: "local" as const,
      name: session.deviceName || session.platform || "Signed-in device",
      app: session.app,
      platform: session.platform,
      current: session.current,
      canRevoke: session.canRevoke,
      trusted: session.trusted,
      createdAt: session.createdAt,
      lastSeenAt: session.lastSeenAt,
      expiresAt: session.expiresAt,
      clientIp: session.clientIp,
    }));
  }

  async updateAccountIdentity(
    origin: AccountOrigin,
    input: { displayName: string; email: string },
    signal: AbortSignal,
  ): Promise<AccountIdentitySnapshot> {
    if (origin === "portico") {
      const result = await this.hosted.request<{ user: PorticoAccountUser }>(
        "/api/account/me",
        {
          method: "PATCH",
          body: { username: input.displayName, email: input.email },
          signal,
        },
      );
      return this.syncHostedIdentity(result.user, signal);
    }
    const user = await this.client.request<User>("/api/account/profile", {
      method: "PATCH",
      body: input,
      signal,
    });
    return {
      displayName: user.displayName,
      email: user.email,
      profileImageUrl: user.profileImageUrl,
    };
  }

  async uploadAccountImage(
    origin: AccountOrigin,
    file: File,
    signal: AbortSignal,
  ): Promise<AccountIdentitySnapshot> {
    abortIfRequested(signal);
    const form = new FormData();
    form.append("file", file);
    if (origin === "portico") {
      const result = await this.hosted.uploadAccountImage(form, { signal });
      abortIfRequested(signal);
      if (!result.user)
        throw new Error(
          "Portico Account did not return the updated profile image.",
        );
      return this.syncHostedIdentity(result.user, signal);
    }
    const user = await this.client.uploadProfileImage(form);
    abortIfRequested(signal);
    return {
      displayName: user.displayName,
      email: user.email,
      profileImageUrl: user.profileImageUrl,
    };
  }

  async deleteAccountImage(
    origin: AccountOrigin,
    signal: AbortSignal,
  ): Promise<AccountIdentitySnapshot> {
    abortIfRequested(signal);
    if (origin === "portico") {
      const result = await this.hosted.deleteAccountImage({ signal });
      abortIfRequested(signal);
      if (!result.user)
        throw new Error("Portico Account did not return the updated profile.");
      return this.syncHostedIdentity(result.user, signal);
    }
    const user = await this.client.deleteProfileImage();
    abortIfRequested(signal);
    return {
      displayName: user.displayName,
      email: user.email,
      profileImageUrl: user.profileImageUrl,
    };
  }

  accountImageUrl(value: string): string {
    if (
      !value ||
      /^https?:\/\//i.test(value) ||
      value.startsWith("data:") ||
      value.startsWith("blob:")
    )
      return value;
    if (value.startsWith("/api/account/me/image"))
      return this.hosted.hostedApiUrl(value);
    return this.client.imageResourceUrl(value, { rendition: "small" });
  }

  async changeLocalPassword(
    input: { currentPassword: string; newPassword: string },
    signal: AbortSignal,
  ): Promise<void> {
    await this.client.changeAccountPassword(input, { signal });
  }

  async changePorticoPassword(
    input: { currentPassword: string; newPassword: string },
    signal: AbortSignal,
  ): Promise<void> {
    await this.hosted.request<{ ok: boolean }>("/api/account/me/password", {
      method: "POST",
      body: input,
      signal,
    });
  }

  async deletePorticoAccount(
    input: { password: string; mfaCode?: string; recoveryCode?: string },
    signal: AbortSignal,
  ): Promise<void> {
    abortIfRequested(signal);
    await this.hosted.deleteAccount(input, { signal });
    abortIfRequested(signal);
  }

  async porticoMFAStatus(signal: AbortSignal): Promise<AccountMFAStatus> {
    return normalizeMFAStatus(
      await this.hosted.request<PorticoMFAStatus>("/api/auth/mfa/status", {
        signal,
      }),
    );
  }

  async startPorticoMFA(
    password: string,
    signal: AbortSignal,
  ): Promise<AccountMFASetup> {
    const response = await this.hosted.request<Record<string, unknown>>(
      "/api/auth/mfa/setup",
      { method: "POST", body: { password }, signal },
    );
    const secret = typeof response.secret === "string" ? response.secret : "";
    const otpauthUrl =
      typeof response.otpauthUrl === "string" ? response.otpauthUrl : "";
    const enrollmentToken =
      typeof response.enrollmentToken === "string"
        ? response.enrollmentToken
        : "";
    if (!secret || !otpauthUrl || !enrollmentToken)
      throw new Error(
        "Portico Account security returned an incomplete authenticator setup.",
      );
    return { enrollmentToken, secret, otpauthUrl };
  }

  async enablePorticoMFA(
    input: { code: string; enrollmentToken: string },
    signal: AbortSignal,
  ): Promise<AccountMFAEnableResult> {
    const response = await this.hosted.request<Record<string, unknown>>(
      "/api/auth/mfa/enable",
      { method: "POST", body: input, signal },
    );
    return {
      enabled: response.enabled === true,
      recoveryCodes: Array.isArray(response.recoveryCodes)
        ? response.recoveryCodes.filter(
            (value): value is string => typeof value === "string",
          )
        : [],
    };
  }

  async rotatePorticoMFARecoveryCodes(
    code: string,
    signal: AbortSignal,
  ): Promise<AccountMFAEnableResult> {
    const response = await this.hosted.rotateMFARecoveryCodes(
      { code },
      { signal },
    );
    return {
      enabled: response.enabled === true,
      recoveryCodes: Array.isArray(response.recoveryCodes)
        ? response.recoveryCodes.filter(
            (value): value is string => typeof value === "string",
          )
        : [],
    };
  }

  async disablePorticoMFA(
    input: { password: string; code: string },
    signal: AbortSignal,
  ): Promise<void> {
    await this.hosted.request<{ ok: boolean }>("/api/auth/mfa/disable", {
      method: "POST",
      body: input,
      signal,
    });
  }

  async revokeSignedInDevice(
    origin: AccountOrigin,
    id: string,
    signal: AbortSignal,
  ): Promise<void> {
    if (origin === "portico") {
      abortIfRequested(signal);
      await this.hosted.revokeDevice(id, { signal });
      abortIfRequested(signal);
      return;
    }
    await this.client.request<{ ok: boolean }>(
      `/api/account/sessions/${encodeURIComponent(id)}`,
      { method: "DELETE", signal },
    );
  }

  async clearWatchHistory(signal: AbortSignal): Promise<void> {
    await this.client.request<{ ok: boolean; clearedAt: string }>(
      "/api/account/watch-history",
      { method: "DELETE", signal },
    );
  }

  async signOut(signal: AbortSignal): Promise<void> {
    await this.client.request<{ ok: boolean }>("/api/auth/logout", {
      method: "POST",
      signal,
    });
  }
}
