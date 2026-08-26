import { createContext, useContext } from 'react';
import type { HostedDeviceAuthorizationDecisionResponse, HostedDeviceAuthorizationPreviewResponse, HostedTVSetupGrantResponse, HostedTVSetupPreviewResponse, ProductMessageId, ViewerScope } from '@porticomediaserver/client-core';
import type { PorticoDataSource, Viewer } from '../data/models';
import type { WebViewerRuntime } from '../data/viewerRuntime';
import type { HostedServerSummary, RuntimeConfig, RuntimeState } from './runtimeMachine';

export type RuntimeContextValue = {
  config: RuntimeConfig;
  state: RuntimeState;
  source?: PorticoDataSource;
  initialViewer?: Viewer;
  /** Cosmetic only; populated only after the account publication fence passes. */
  restoredPresentation?: { accountId: string; displayName: string };
  expectedViewerScope?: ViewerScope;
  viewerRuntime: WebViewerRuntime;
  connectionWarning?: ProductMessageId;
  hostedRetryCohort: string;
  hostedAvailabilityFailureStartedAt?: number;
  dismissConnectionWarning: () => void;
  busy: boolean;
  mfaRequired: boolean;
  hasPasswordResetIntent: boolean;
  ssoOnboardingToken?: string;
  hasServerClaimIntent: boolean;
  hasDeviceAuthorizationIntent: boolean;
  deviceAuthorizationProvider?: 'google' | 'apple';
  deviceAuthorizationCode?: string;
  nativeDeviceAuthorizationReturn: boolean;
  serverClaimName?: string;
  localLoginServerName?: string;
  retry: () => void;
  tryNearbyConnection: () => Promise<void>;
  recoverActiveRoute: () => Promise<void>;
  continueWithHostedAccount: () => Promise<void>;
  selectServer: (server: HostedServerSummary) => Promise<void>;
  selectProfile: (profileId: string, pin?: string) => Promise<void>;
  beginProfileSelection: () => Promise<void>;
  reselectServer: () => Promise<void>;
  disconnectServer: () => Promise<void>;
  hostedLogin: (credentials: { login: string; password: string; mfaCode?: string; recoveryCode?: string }) => Promise<void>;
  hostedRegister: (details: { email: string; username: string; password: string }) => Promise<void>;
  previewSSOOnboarding: (onboardingToken: string, signal?: AbortSignal) => Promise<SSOOnboardingPreview>;
  completeSSOOnboarding: (details: { onboardingToken: string; username: string; contactEmail?: string }) => Promise<void>;
  hostedLogout: () => Promise<void>;
  refreshMemberships: () => Promise<void>;
  claimServer: (claimCode: string) => Promise<void>;
  acceptInvite: (inviteId: string) => Promise<void>;
  previewTVSetup: (code: string, signal?: AbortSignal) => Promise<HostedTVSetupPreviewResponse>;
  authorizeTVSetup: (preview: HostedTVSetupPreviewResponse, serverId: string, signal?: AbortSignal) => Promise<HostedTVSetupGrantResponse>;
  previewGenericDeviceAuthorization: (code: string, signal?: AbortSignal) => Promise<HostedDeviceAuthorizationPreviewResponse>;
  decideGenericDeviceAuthorization: (code: string, decision: 'approve' | 'deny', signal?: AbortSignal) => Promise<HostedDeviceAuthorizationDecisionResponse>;
  requestPasswordReset: (email: string) => Promise<void>;
  completePasswordReset: (password: string) => Promise<void>;
};

export type SSOOnboardingPreview = {
  provider: 'google' | 'apple';
  providerEmail?: string;
  privateEmail: boolean;
  verifiedContactEmailRequired: boolean;
  suggestedUsername?: string;
};

export const RuntimeContext = createContext<RuntimeContextValue | null>(null);

export function useRuntime() {
  const runtime = useContext(RuntimeContext);
  if (!runtime) throw new Error('Runtime hooks must be used inside RuntimeProvider.');
  return runtime;
}

export function useOptionalRuntime() {
  return useContext(RuntimeContext);
}
