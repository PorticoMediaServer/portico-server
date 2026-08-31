import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import {
	assertViewerIdentity,
	isTerminalServerAuthorizationFailure,
	normalizeViewerScope,
	PORTICO_QUERY_RETAIN_TIME_MS,
	PORTICO_QUERY_STALE_TIME_MS,
	productMessage,
	resolveProductProblem,
	sameViewerScope,
	shouldRetryPorticoQuery,
	ViewerRuntimeTeardownError,
	viewerQueryKey,
	type LibraryChannelGuide,
	type LibraryChannelsGuide,
	type LibraryChannelListResponse,
	type ProductMessageId,
	type AppEvent,
	type ProfileTransitionReason,
	type ViewerScope,
} from '@porticomediaserver/client-core';
import { QueryClient, QueryClientProvider, useQuery, useQueryClient } from '@tanstack/react-query';
import type {
		BrowserAccountRemoval,
		BrowserAccountSummary,
		BrowserAccountsState,
  LocalProfileLoginChallenge,
  HomeResult,
  HomeRow,
  LiveTVGuideInput,
  LiveTVGuideResult,
  ActionableLiveTVChannel,
  ActionableLiveTVSource,
  DVRResult,
  MediaItem,
  PersonDetail,
  PorticoDataSource,
  SavedResourceDetail,
  SavedResourceItemsInput,
  SavedResourceItemsMutation,
  SavedEditableResourceKind,
  SavedResourceKind,
  SavedResourceSummary,
  SearchPageResult,
  SearchPageInput,
  SearchResult,
  Viewer,
} from './models';
import { HttpPorticoDataSource, LocalProfileSelectionRequiredError } from './httpSource';
import { scopedDataSource, WebViewerRuntime } from './viewerRuntime';
import { clearArtworkFailureCache, tagsMayRefreshArtwork } from './artworkFailureCache';
import {
	ambientCookieRestoreStatus,
	bindAmbientCookieMutationToViewer,
	claimAmbientCookieMutation,
	clearAmbientCookieAfterVerifiedAuthentication,
	ownsAmbientCookieMutation,
	releaseAmbientCookieMutationReservation,
	reserveAmbientCookieMutation,
	type AmbientCookieExpectedIdentity,
	type AmbientCookieMutationKind,
	type AmbientCookieQuarantineMarker,
} from '../runtime/ambientCookieQuarantine';
import { withAmbientCookieMutationLock } from '../runtime/accountPublicationFence';
import { PORTICO_ROUTE_DIAGNOSTIC_EVENT, recordRouteDataState, type RouteDiagnosticUpload } from '../runtime/routeDiagnostics';

type QueryState<T> =
  | { status: 'loading'; data?: undefined; error?: undefined; stale: false; isFetching: boolean; lastSuccessAt?: undefined }
  | { status: 'success'; data: T; error?: Error; stale: boolean; isFetching: boolean; lastSuccessAt?: number }
  | { status: 'error'; data?: undefined; error: Error };

const DataSourceContext = createContext<PorticoDataSource | null>(null);
const ViewerRuntimeContext = createContext<WebViewerRuntime | null>(null);
const QueryRuntimeReadyContext = createContext(false);

function createWebQueryClient() {
	return new QueryClient({
		defaultOptions: {
			mutations: { gcTime: 5 * 60_000, retry: false },
			queries: {
				gcTime: PORTICO_QUERY_RETAIN_TIME_MS,
				refetchOnReconnect: true,
				refetchOnWindowFocus: false,
				retry: shouldRetryPorticoQuery,
				retryDelay: (attempt) => Math.min(250 * (2 ** attempt), 1_000),
				staleTime: PORTICO_QUERY_STALE_TIME_MS,
			},
		},
	});
}

type LiveRevisionSubscription = { tags: ReadonlySet<string>; listener: () => void };

class LiveDataRevisionStore {
	private revisions = new Map<string, number>();
	private sequence = 0;
	private subscriptions = new Set<LiveRevisionSubscription>();

	publish(tags: readonly string[]) {
		const unique = new Set(tags);
		if (!unique.size) return;
		const sequence = ++this.sequence;
		for (const tag of unique) this.revisions.set(tag, sequence);
		for (const subscription of this.subscriptions) {
			if (unique.has('*') || subscription.tags.has('*') || [...unique].some((tag) => subscription.tags.has(tag))) subscription.listener();
		}
	}

	revision(tags: readonly string[]) {
		return Math.max(this.revisions.get('*') ?? 0, ...tags.map((tag) => this.revisions.get(tag) ?? 0));
	}

	subscribe(tags: readonly string[], listener: () => void) {
		const subscription = { tags: new Set(tags), listener };
		this.subscriptions.add(subscription);
		return () => this.subscriptions.delete(subscription);
	}
}

const emptyLiveDataRevisionStore = new LiveDataRevisionStore();
const LiveDataRevisionContext = createContext<LiveDataRevisionStore>(emptyLiveDataRevisionStore);

const viewerIdentityEventTags = new Set(['account', 'profiles', 'users']);

function mayChangeViewerIdentity(tags: readonly string[]): boolean {
	return tags.includes('*') || tags.some((tag) => viewerIdentityEventTags.has(tag));
}

function sameViewerIdentity(left: ViewerScope, right: ViewerScope): boolean {
	return left.authority === right.authority
		&& left.accountId === right.accountId
		&& left.serverId === right.serverId
		&& left.profileId === right.profileId;
}

function verifiedViewerScope(viewer: Viewer): ViewerScope | undefined {
	if (!viewer.authenticated) return undefined;
	if (!viewer.viewerScope) throw new Error('This server returned an authenticated session without a complete viewing profile.');
	return normalizeViewerScope(viewer.viewerScope);
}

function withoutVerifiedViewer(viewer: Viewer | undefined): Viewer | undefined {
	return viewer ? { ...viewer, authenticated: false, user: undefined, viewerScope: undefined } : viewer;
}

type ExpectedReplacementIdentity = Pick<ViewerScope, 'authority' | 'accountId'> & Partial<Pick<ViewerScope, 'serverId' | 'profileId'>>;

type CanonicalIdentityError = Error & {
	messageId: ProductMessageId;
	code?: string;
	status?: number;
	details?: Readonly<Record<string, unknown>>;
	detail?: string;
	requestId?: string;
	retryAfter?: string;
	retryAt?: string;
	retryAfterMs?: number;
	retryable?: boolean;
	ambiguous?: boolean;
};

function canonicalIdentityError(reason: unknown, fallback: ProductMessageId = 'problem.request-failed'): CanonicalIdentityError {
	const candidate = reason && typeof reason === 'object'
		? reason as { code?: unknown; messageId?: unknown; status?: unknown; detail?: unknown; details?: unknown; requestId?: unknown; retryAfter?: unknown; retryAt?: unknown; retryAfterMs?: unknown; retryable?: unknown; ambiguous?: unknown }
		: undefined;
	const resolved = candidate ? resolveProductProblem({
		...(typeof candidate.code === 'string' ? { code: candidate.code } : {}),
		...(typeof candidate.messageId === 'string' ? { messageId: candidate.messageId } : {}),
		...(typeof candidate.status === 'number' ? { status: candidate.status } : {}),
		...(candidate.details && typeof candidate.details === 'object' ? { details: candidate.details as Readonly<Record<string, unknown>> } : {}),
	}) : productMessage(fallback);
	const presentation = resolved.id === 'problem.request-failed' && fallback !== 'problem.request-failed'
		? productMessage(fallback)
		: resolved;
	const error = new Error(presentation.body ?? presentation.text ?? presentation.title ?? 'Portico could not complete this request.');
	error.name = 'PorticoIdentityError';
	(error as Error & { cause?: unknown }).cause = reason;
	return Object.assign(error, {
		messageId: presentation.id,
		...(typeof candidate?.code === 'string' ? { code: candidate.code } : {}),
		...(typeof candidate?.status === 'number' ? { status: candidate.status } : {}),
		...(typeof candidate?.detail === 'string' ? { detail: candidate.detail } : {}),
		...(candidate?.details && typeof candidate.details === 'object' ? { details: candidate.details as Readonly<Record<string, unknown>> } : {}),
		...(typeof candidate?.requestId === 'string' ? { requestId: candidate.requestId } : {}),
		...(typeof candidate?.retryAfter === 'string' ? { retryAfter: candidate.retryAfter } : {}),
		...(typeof candidate?.retryAt === 'string' ? { retryAt: candidate.retryAt } : {}),
		...(typeof candidate?.retryAfterMs === 'number' ? { retryAfterMs: candidate.retryAfterMs } : {}),
		...(typeof candidate?.retryable === 'boolean' ? { retryable: candidate.retryable } : {}),
		...(typeof candidate?.ambiguous === 'boolean' ? { ambiguous: candidate.ambiguous } : {}),
	});
}

class ReplacementViewerIdentityError extends Error {
	readonly messageId = 'problem.request-failed' as const;

	constructor() {
		super(productMessage('problem.request-failed').body);
		this.name = 'ReplacementViewerIdentityError';
	}
}

function assertReplacementIdentity(actual: ViewerScope | undefined, expected: ExpectedReplacementIdentity): asserts actual is ViewerScope {
	if (!actual
		|| actual.authority !== expected.authority
		|| actual.accountId !== expected.accountId
		|| (expected.serverId !== undefined && actual.serverId !== expected.serverId)
		|| (expected.profileId !== undefined && actual.profileId !== expected.profileId)) {
		throw new ReplacementViewerIdentityError();
	}
}

function requiredReplacementScope(viewer: Viewer): ViewerScope {
	try {
		const scope = verifiedViewerScope(viewer);
		if (scope) return scope;
	} catch { /* Replace malformed server detail with one non-presentational identity failure. */ }
	throw new ReplacementViewerIdentityError();
}

function browserAccountExpectation(account: BrowserAccountSummary, currentScope: ViewerScope | undefined): ExpectedReplacementIdentity {
	return {
		authority: account.authOrigin === 'portico' ? 'hosted' : 'local',
		accountId: account.id,
		...(currentScope?.serverId ? { serverId: currentScope.serverId } : {}),
	};
}

type AuthContextValue = {
  status: 'loading' | 'ready' | 'error';
  viewer?: Viewer;
  error?: Error;
  busy: boolean;
	browserAccounts: {
		status: 'loading' | 'ready' | 'error';
		data: BrowserAccountsState;
		error?: Error;
	};
	sessionRevision: number;
	viewerScopeKey: string;
	addingAccount: boolean;
	localProfileLogin?: LocalProfileLoginChallenge;
	login: (credentials: { login: string; password: string; rememberOnBrowser?: boolean }) => Promise<void>;
	selectLocalProfile: (profileId: string, pin?: string) => Promise<void>;
	cancelLocalProfileLogin: () => void;
	setup: (details: { serverName: string; username: string; email: string; displayName: string; password: string; rememberOnBrowser?: boolean }) => Promise<void>;
  startPorticoSetup: (serverName: string) => Promise<{ claimUrl: string; expiresAt?: string }>;
  porticoSetupStatus: () => Promise<{ setupRequired: boolean; claimStatus: string; porticoConnected: boolean }>;
  logout: () => Promise<void>;
	switchBrowserAccount: (accountId: string) => Promise<void>;
		switchLocalProfile: (input: { login: string; password: string; profileId: string; pin?: string }) => Promise<void>;
		switchAuthenticatedLocalProfile: (profileId: string, pin?: string) => Promise<void>;
		switchHostedProfile: (input: { profileId: string; pin?: string }) => Promise<void>;
	updateAutomaticSignIn: (automaticSignIn: boolean) => Promise<void>;
	removeBrowserAccount: (accountId: string) => Promise<BrowserAccountRemoval>;
	signOutAllBrowserAccounts: () => Promise<void>;
	beginAddAccount: () => void;
	cancelAddAccount: () => void;
	retryBrowserAccounts: () => void;
	registerSessionTeardown: (teardown: () => Promise<void> | void) => () => void;
	registerRuntimeTeardown: (kind: 'playback' | 'realtime' | 'artwork' | 'overlay' | 'focus' | 'profile-local', teardown: () => Promise<void> | void) => () => void;
  updateProfile: (profile: { displayName: string; email: string }) => Promise<void>;
  refresh: () => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

const emptyBrowserAccounts: BrowserAccountsState = {
	accounts: [],
	automaticSignIn: true,
	selectionRequired: false,
	canAddAccount: false,
};

export function DataProvider({ children, source, initialViewer, expectedViewerScope, browserAccountsEnabled = true, localSessionQuarantineEnabled = false, viewerRuntime }: { children: ReactNode; source?: PorticoDataSource; initialViewer?: Viewer; expectedViewerScope?: ViewerScope; browserAccountsEnabled?: boolean; localSessionQuarantineEnabled?: boolean; viewerRuntime?: WebViewerRuntime }) {
  const value = useMemo(() => source ?? new HttpPorticoDataSource(), [source]);
	// Hosted RuntimeProvider supplies one process-lifetime runtime so replacing a
	// PorticoDataSource cannot discard the generation fence or bypass awaited
	// teardown. Standalone/bundled tests retain a provider-owned runtime.
	const ownedRuntime = useMemo(() => new WebViewerRuntime(), []);
	const runtime = viewerRuntime ?? ownedRuntime;
	// Keep serving the already-verified source while a replacement performs its
	// fresh identity check and awaited viewer transition. Publishing the new
	// source directly from props would create a render-sized window in which B
	// could run under A's scope.
	const [activeValue, setActiveValue] = useState(value);
	const subscribeGeneration = useCallback((listener: () => void) => runtime.subscribeGeneration(listener), [runtime]);
	const generationSnapshot = useCallback(() => runtime.currentGeneration(), [runtime]);
	const scopeGeneration = useSyncExternalStore(subscribeGeneration, generationSnapshot, generationSnapshot);
	const scopedValue = useMemo(() => scopedDataSource(activeValue, runtime), [activeValue, runtime, scopeGeneration]);
	useEffect(() => {
		if (typeof window === 'undefined' || !scopedValue.uploadClientDiagnostics) return;
		const upload = scopedValue.uploadClientDiagnostics.bind(scopedValue);
		const listener = (event: Event) => {
			const detail = (event as CustomEvent<RouteDiagnosticUpload>).detail;
			if (!detail?.entries?.length) return;
			const controller = new AbortController();
			void upload(detail, controller.signal).catch(() => undefined);
		};
		window.addEventListener(PORTICO_ROUTE_DIAGNOSTIC_EVENT, listener);
		return () => window.removeEventListener(PORTICO_ROUTE_DIAGNOSTIC_EVENT, listener);
	}, [scopedValue]);
	// Route selection lives below logical server data. Keep one process-lifetime
	// query client: viewer transitions synchronously fence keys and await the
	// registered cache teardown before the new identity can publish. Replacing
	// the QueryClient itself during that transition creates a render/effect race
	// in which realtime invalidation can target the retired instance.
	const activeQueryScopeKey = runtime.activeScopeKey();
	const queryClient = useMemo(() => createWebQueryClient(), []);
	const [queryRuntimeReadyScopeKey, setQueryRuntimeReadyScopeKey] = useState('');
	useEffect(() => {
		const unregister = runtime.register('query', async () => {
			await queryClient.cancelQueries();
			queryClient.getMutationCache().clear();
			queryClient.getQueryCache().clear();
		});
		return () => { unregister(); };
	}, [queryClient, runtime]);
  // initialViewer is deliberately only a boot hint. A fresh /me response must
  // supply the complete scope before authenticated product UI can mount.
  const [auth, setAuth] = useState<Pick<AuthContextValue, 'status' | 'viewer' | 'error'>>({ status: 'loading', viewer: initialViewer });
	const [browserAccounts, setBrowserAccounts] = useState<AuthContextValue['browserAccounts']>(browserAccountsEnabled
		? { status: 'loading', data: emptyBrowserAccounts }
		: { status: 'ready', data: emptyBrowserAccounts });
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
	const [browserRevision, setBrowserRevision] = useState(0);
	const [sessionRevision, setSessionRevision] = useState(0);
	const liveDataRevisions = useMemo(() => new LiveDataRevisionStore(), []);
	const [addingAccount, setAddingAccount] = useState(false);
	const [localProfileLogin, setLocalProfileLogin] = useState<LocalProfileLoginChallenge>();
	const localProfileGeneration = useRef(0);
	type LocalProfileOperation = { generation: number; controller: AbortController; commitStarted: boolean };
	type LocalProfileSecurityFailure = { error: Error; scope: AmbientCookieExpectedIdentity };
	const activeLocalProfileOperation = useRef<LocalProfileOperation | undefined>(undefined);
	const localProfileSecurityFailure = useRef<LocalProfileSecurityFailure | undefined>(undefined);
	const automaticSwitchAttempt = useRef('');
	const suppressAutomaticSwitch = useRef(false);
	const liveViewerReconciliation = useRef<{ generation: number; controller?: AbortController }>({ generation: 0 });

  useEffect(() => {
    const controller = new AbortController();
    // Preserve non-authoritative capability hints while the fresh identity
    // request runs. Product UI remains blocked by `status: loading`; only the
    // final viewer scope is authoritative.
    setAuth((current) => ({ status: 'loading', viewer: current.viewer }));
	const ambientRestore = localSessionQuarantineEnabled ? ambientCookieRestoreStatus() : undefined;
	if (ambientRestore && (!ambientRestore.trustedForRestore || ambientRestore.quarantined)) {
		suppressAutomaticSwitch.current = true;
		setAuth((current) => ({
			status: 'ready',
			viewer: withoutVerifiedViewer(current.viewer),
			error: canonicalIdentityError(undefined, 'auth.sign-out-storage-warning'),
		}));
		return () => controller.abort();
	}
    value.viewer(controller.signal).then(async (viewer) => {
		if (controller.signal.aborted) return;
		const nextScope = verifiedViewerScope(viewer);
		if (expectedViewerScope) {
			if (!nextScope) throw new Error('The final server identity did not contain the expected viewing profile.');
			assertViewerIdentity(nextScope, expectedViewerScope);
		}
		const currentScope = runtime.activeScope();
		if (currentScope && nextScope && !sameViewerScope(currentScope, nextScope)) await runtime.transition(nextScope, 'authorization-changed');
		else if (currentScope && !nextScope) await runtime.transition(undefined, 'profile-revoked');
		else if (!currentScope && nextScope) runtime.activateInitial(nextScope);
		if (!controller.signal.aborted) {
				setActiveValue(value);
			setAuth((current) => ({ status: 'ready', viewer: { ...viewer, authCapabilities: viewer.authCapabilities ?? current.viewer?.authCapabilities } }));
		}
	}).catch(async (reason: unknown) => {
		if (controller.signal.aborted) return;
		if (isTerminalServerAuthorizationFailure(reason) && runtime.activeScope()) {
			try { await runtime.transition(undefined, 'profile-revoked'); } catch { /* the security fence still clears the viewer scope */ }
				suppressAutomaticSwitch.current = true;
			setAuth((current) => ({ status: 'ready', viewer: withoutVerifiedViewer(current.viewer), error: canonicalIdentityError(reason, 'auth.session-expired') }));
			return;
		}
		if (expectedViewerScope) {
			try { await runtime.transition(undefined, 'profile-revoked'); } catch { runtime.failClosed(); }
		}
		// A transport failure or 5xx response does not prove that an already
		// verified ambient-cookie session was revoked. Keep the last published
		// viewer and its generation live so a temporary server outage cannot turn
		// a usable Bundled Web surface into a misleading sign-in/profile failure.
		// Replacement bootstraps (`expectedViewerScope`) still fail closed because
		// their candidate identity has never been verified in this process.
		if (!expectedViewerScope && runtime.activeScope()) {
			setAuth((current) => ({ status: 'ready', viewer: current.viewer, error: canonicalIdentityError(reason) }));
			return;
		}
		setAuth({ status: 'error', error: canonicalIdentityError(reason) });
	});
    return () => controller.abort();
  }, [expectedViewerScope, localSessionQuarantineEnabled, revision, runtime, value]);

	const reconcileLiveViewer = useCallback(async () => {
		liveViewerReconciliation.current.controller?.abort();
		const controller = new AbortController();
		const generation = liveViewerReconciliation.current.generation + 1;
		liveViewerReconciliation.current = { generation, controller };
		const ownsPublication = () => !controller.signal.aborted
			&& liveViewerReconciliation.current.generation === generation;
		try {
			const viewer = await value.viewer(controller.signal);
			if (!ownsPublication()) return;
			const currentScope = runtime.activeScope();
			const nextScope = verifiedViewerScope(viewer);
			if (currentScope && nextScope && !sameViewerIdentity(currentScope, nextScope)) {
				await runtime.transition(undefined, 'profile-revoked');
				if (ownsPublication()) {
					suppressAutomaticSwitch.current = true;
					setAuth((current) => ({
						status: 'ready',
						viewer: withoutVerifiedViewer(current.viewer),
						error: canonicalIdentityError(undefined, 'auth.session-expired'),
					}));
				}
				return;
			}
			if (currentScope && !nextScope) {
				await runtime.transition(undefined, 'profile-revoked');
				if (ownsPublication()) {
					suppressAutomaticSwitch.current = true;
					setAuth((current) => ({
						status: 'ready',
						viewer: { ...viewer, authCapabilities: viewer.authCapabilities ?? current.viewer?.authCapabilities },
						error: canonicalIdentityError(undefined, 'auth.session-expired'),
					}));
				}
				return;
			}
			if (!currentScope || !nextScope) return;
			if (!sameViewerScope(currentScope, nextScope)) {
				await runtime.transition(nextScope, 'authorization-changed');
			}
			if (ownsPublication()) {
				setActiveValue(value);
				setAuth((current) => ({
					status: 'ready',
					viewer: { ...viewer, authCapabilities: viewer.authCapabilities ?? current.viewer?.authCapabilities },
				}));
			}
		} catch (reason) {
			if (!ownsPublication()) return;
			if (isTerminalServerAuthorizationFailure(reason) && runtime.activeScope()) {
				try { await runtime.transition(undefined, 'profile-revoked'); } catch { runtime.failClosed(); }
				if (ownsPublication()) {
					suppressAutomaticSwitch.current = true;
					setAuth((current) => ({
						status: 'ready',
						viewer: withoutVerifiedViewer(current.viewer),
						error: canonicalIdentityError(reason, 'auth.session-expired'),
					}));
				}
				return;
			}
			// A failed background identity check is not proof that the active
			// viewer was revoked. Preserve the verified product surface and let
			// the realtime transport retry instead of replacing it with an error page.
			setAuth((current) => ({ ...current, status: 'ready', error: canonicalIdentityError(reason) }));
		}
	}, [runtime, value]);

	useEffect(() => () => {
		liveViewerReconciliation.current.controller?.abort();
		liveViewerReconciliation.current.generation += 1;
	}, []);

	useEffect(() => {
		if (!browserAccountsEnabled) {
			setBrowserAccounts({ status: 'ready', data: emptyBrowserAccounts });
			return;
		}
		if (auth.status !== 'ready' || !auth.viewer || auth.viewer.setupRequired) return;
		const controller = new AbortController();
		setBrowserAccounts((current) => ({ status: 'loading', data: current.data }));
		value.browserAccounts(controller.signal).then(
			(data) => !controller.signal.aborted && setBrowserAccounts({ status: 'ready', data }),
			(reason: unknown) => !controller.signal.aborted && setBrowserAccounts((current) => ({
				status: 'error',
				data: current.data,
					error: canonicalIdentityError(reason),
			})),
		);
		return () => controller.abort();
	}, [auth.status, auth.viewer?.setupRequired, browserAccountsEnabled, browserRevision, value]);

	const applyReplacementViewer = useCallback(async (
		signal: AbortSignal,
		reason: ProfileTransitionReason = 'server-switch',
		expectedIdentity?: ExpectedReplacementIdentity,
		commitPublication?: (scope: ViewerScope, publish: () => void) => void,
	) => {
			const viewer = await value.viewer(signal);
			const nextScope = requiredReplacementScope(viewer);
			if (expectedIdentity) assertReplacementIdentity(nextScope, expectedIdentity);
			const currentScope = runtime.activeScope();
			if (currentScope) await runtime.transition(nextScope, reason);
			else runtime.activateInitial(nextScope);
			// The caller's final ownership check and every React identity publication
			// are one synchronous commit after the last await. Quarantine is released
			// only after publish() has committed all state updates.
			const publish = () => {
				setAuth((current) => ({ status: 'ready', viewer: { ...viewer, authCapabilities: viewer.authCapabilities ?? current.viewer?.authCapabilities } }));
				setAddingAccount(false);
				setSessionRevision((current) => current + 1);
				setBrowserRevision((current) => current + 1);
			};
			if (commitPublication) commitPublication(nextScope, publish);
			else publish();
	}, [runtime, value]);

	const fenceCurrentViewer = useCallback(async (reason: ProfileTransitionReason) => {
		if (runtime.activeScope()) {
			try { await runtime.transition(undefined, reason); }
				finally { /* Runtime generation publication rebuilds every scoped binding. */ }
		}
	}, [runtime]);
	const fenceSignedOutViewer = useCallback(async () => {
		try {
			await fenceCurrentViewer('sign-out');
			return undefined;
		} catch (reason) {
			return reason instanceof Error ? reason : new Error('Some viewing-profile activity could not be closed cleanly.');
		}
	}, [fenceCurrentViewer]);
	const failClosedOnUnsafeTransition = useCallback((reason: unknown, sessionWasFenced = false) => {
		if (reason instanceof ReplacementViewerIdentityError || reason instanceof ViewerRuntimeTeardownError || sessionWasFenced) {
			runtime.failClosed();
			suppressAutomaticSwitch.current = true;
			const error = canonicalIdentityError(reason);
			setAuth((current) => ({ status: 'error', viewer: withoutVerifiedViewer(current.viewer), error }));
			return;
		}
	}, [runtime]);

	const assertBundledCookieMutationOwner = useCallback((marker: AmbientCookieQuarantineMarker | undefined): void => {
		if (localSessionQuarantineEnabled && (!marker || !ownsAmbientCookieMutation(marker))) {
			throw canonicalIdentityError(undefined, 'auth.sign-out-storage-warning');
		}
	}, [localSessionQuarantineEnabled]);
	const reserveBundledCookieMutation = useCallback((
		kind: AmbientCookieMutationKind,
		intent: Parameters<typeof reserveAmbientCookieMutation>[1],
	): AmbientCookieQuarantineMarker | undefined => {
		if (!localSessionQuarantineEnabled) return undefined;
		const marker = reserveAmbientCookieMutation(kind, intent);
		if (!marker) throw canonicalIdentityError(undefined, 'auth.sign-out-storage-warning');
		return marker;
	}, [localSessionQuarantineEnabled]);
	const withAmbientCookieMutation = useCallback(async <T,>(
		kind: AmbientCookieMutationKind,
		intent: Parameters<typeof reserveAmbientCookieMutation>[1],
		operation: (marker: AmbientCookieQuarantineMarker | undefined) => Promise<T>,
	): Promise<T> => {
		// Publish the intent before queueing. A later context can supersede this
		// marker while an older Set-Cookie or final /me is still in flight; every
		// older bind/clear/publication then fails its ownership check.
		const marker = reserveBundledCookieMutation(kind, intent);
		// Bundled authentication mutates one origin-wide HttpOnly cookie. A hook-
		// local promise tail cannot linearize another tab or another provider root.
		// Browsers without the required same-origin lock fail before any mutation.
		return localSessionQuarantineEnabled
			? withAmbientCookieMutationLock(async () => {
				if (!marker || !claimAmbientCookieMutation(marker)) {
					if (marker) releaseAmbientCookieMutationReservation(marker);
					throw canonicalIdentityError(undefined, 'auth.sign-out-storage-warning');
				}
				try { return await operation(marker); }
				finally { releaseAmbientCookieMutationReservation(marker); }
			})
			: operation(marker);
	}, [localSessionQuarantineEnabled, reserveBundledCookieMutation]);
	const bindBundledCookieMutation = useCallback((
		marker: AmbientCookieQuarantineMarker | undefined,
		scope: ViewerScope,
	): AmbientCookieQuarantineMarker | undefined => {
		if (!localSessionQuarantineEnabled) return undefined;
		if (!marker) throw canonicalIdentityError(undefined, 'auth.sign-out-storage-warning');
		const exact = bindAmbientCookieMutationToViewer(marker, scope);
		if (!exact) throw canonicalIdentityError(undefined, 'auth.sign-out-storage-warning');
		return exact;
	}, [localSessionQuarantineEnabled]);
	const clearBundledCookieMutation = useCallback((
		marker: AmbientCookieQuarantineMarker | undefined,
		scope: ViewerScope,
	): void => {
		if (localSessionQuarantineEnabled
			&& (!marker || !clearAmbientCookieAfterVerifiedAuthentication(marker, scope))) {
			throw canonicalIdentityError(undefined, 'auth.sign-out-storage-warning');
		}
	}, [localSessionQuarantineEnabled]);

	const switchToBrowserAccount = useCallback(async (account: BrowserAccountSummary, signal: AbortSignal, currentScope = runtime.activeScope()) => {
		const expected = browserAccountExpectation(account, currentScope);
		return withAmbientCookieMutation('browser-account-switch', {
			state: 'authenticated',
			expected,
		}, async (marker) => {
			try {
				if (currentScope) await fenceCurrentViewer('server-switch');
				const candidateViewer = await value.switchBrowserAccount(account.id, signal);
				const candidateScope = requiredReplacementScope(candidateViewer);
				assertReplacementIdentity(candidateScope, expected);
				const exactMarker = bindBundledCookieMutation(marker, candidateScope);
				await applyReplacementViewer(signal, 'server-switch', {
					...expected,
					serverId: expected.serverId ?? candidateScope.serverId,
					profileId: candidateScope.profileId,
				}, (finalScope, publish) => {
					assertBundledCookieMutationOwner(exactMarker);
					publish();
					clearBundledCookieMutation(exactMarker, finalScope);
				});
			} catch (reason) {
				if (reason instanceof LocalProfileSelectionRequiredError) {
					const challenge = reason.challenge;
					if (challenge.directory.authority !== 'local'
						|| challenge.directory.accountId !== account.id
						|| (expected.serverId && challenge.directory.serverId !== expected.serverId)
						|| !challenge.installationId
						|| Date.parse(challenge.expiresAt) <= Date.now()) {
						throw new ReplacementViewerIdentityError();
					}
					suppressAutomaticSwitch.current = true;
					setAuth((current) => ({ status: 'ready', viewer: withoutVerifiedViewer(current.viewer) }));
					setBrowserAccounts((current) => ({
						...current,
						data: { ...current.data, selectionRequired: true },
					}));
					setLocalProfileLogin(challenge);
					return;
				}
				if (!localSessionQuarantineEnabled) throw reason;
				runtime.failClosed();
				suppressAutomaticSwitch.current = true;
				const unsafe = canonicalIdentityError(reason, 'auth.sign-out-storage-warning');
				setAuth((current) => ({
					status: 'error',
					viewer: withoutVerifiedViewer(current.viewer),
					error: unsafe,
				}));
				throw unsafe;
			}
		});
	}, [
		applyReplacementViewer,
		assertBundledCookieMutationOwner,
		bindBundledCookieMutation,
		clearBundledCookieMutation,
		fenceCurrentViewer,
		localSessionQuarantineEnabled,
		runtime,
		value,
		withAmbientCookieMutation,
	]);

	useEffect(() => {
		if (!browserAccountsEnabled || browserAccounts.status !== 'ready' || addingAccount || suppressAutomaticSwitch.current) return;
		const viewer = auth.viewer;
		const accounts = browserAccounts.data.accounts;
		if (!viewer || viewer.authenticated || viewer.setupRequired || !browserAccounts.data.automaticSignIn || browserAccounts.data.selectionRequired || accounts.length === 0) return;
		const account = accounts[0];
		const attemptKey = `${account.id}:${account.lastUsedAt}`;
		if (automaticSwitchAttempt.current === attemptKey) return;
		automaticSwitchAttempt.current = attemptKey;
		const controller = new AbortController();
		const sessionWasFenced = Boolean(runtime.activeScope());
		setBusy(true);
		switchToBrowserAccount(account, controller.signal).then(
			() => undefined,
			(reason: unknown) => {
				if (controller.signal.aborted) return;
				failClosedOnUnsafeTransition(reason, sessionWasFenced);
				setBrowserAccounts((current) => ({
					status: 'error',
					data: { ...current.data, selectionRequired: true },
					error: canonicalIdentityError(reason),
				}));
			},
		).finally(() => !controller.signal.aborted && setBusy(false));
		return () => controller.abort();
	}, [addingAccount, auth.viewer, browserAccounts, browserAccountsEnabled, failClosedOnUnsafeTransition, runtime, switchToBrowserAccount]);

  const runAuthMutation = async (mutation: (signal: AbortSignal) => Promise<Viewer | void>, replacesSession = false) => {
    const controller = new AbortController();
	const sessionWasFenced = replacesSession && Boolean(runtime.activeScope());
    setBusy(true);
    try {
			if (replacesSession) await fenceCurrentViewer('server-switch');
	      const result = await mutation(controller.signal);
			if (result && replacesSession) {
				const candidateScope = requiredReplacementScope(result);
				await applyReplacementViewer(controller.signal, 'server-switch', candidateScope);
			}
		else if (result) setAuth((current) => ({ status: 'ready', viewer: { ...result, authCapabilities: result.authCapabilities ?? current.viewer?.authCapabilities } }));
      else setRevision((current) => current + 1);
	    } catch (reason) {
	      const error = canonicalIdentityError(reason);
			if (reason instanceof ReplacementViewerIdentityError || reason instanceof ViewerRuntimeTeardownError || sessionWasFenced) failClosedOnUnsafeTransition(reason, sessionWasFenced);
			else setAuth((current) => ({ ...current, error }));
      throw error;
    } finally {
      setBusy(false);
    }
  };

	const beginLocalProfileOperation = () => {
		if (!activeLocalProfileOperation.current?.commitStarted) activeLocalProfileOperation.current?.controller.abort();
		const operation = {
			generation: ++localProfileGeneration.current,
			controller: new AbortController(),
			commitStarted: false,
		};
		activeLocalProfileOperation.current = operation;
		return operation;
	};
	const assertLocalProfileOperation = (operation: LocalProfileOperation) => {
		if (operation.controller.signal.aborted || operation.generation !== localProfileGeneration.current) {
			throw new DOMException('A newer local profile choice replaced this one.', 'AbortError');
		}
	};
	const compensateLocalProfileMutation = async (
		reason: unknown,
		scope: AmbientCookieExpectedIdentity,
		ownedCleanupMarker?: AmbientCookieQuarantineMarker,
		supersededMarker?: AmbientCookieQuarantineMarker,
	): Promise<void> => {
		// The browser-session endpoint mutates an ambient HttpOnly cookie. Publish
		// and read-verify the restart barrier synchronously before the first cleanup
		// await, even when the current process can still attempt compensation.
		let quarantineError: unknown;
		let cleanupMarker = ownedCleanupMarker;
		try {
			if (!cleanupMarker && supersededMarker) {
				const current = ambientCookieRestoreStatus();
				if (!current.trustedForRestore || !current.quarantined) {
					throw new Error('The superseded ambient-cookie mutation did not retain its restart quarantine.');
				}
			} else if (!cleanupMarker) {
				cleanupMarker = reserveBundledCookieMutation('logout', { state: 'signed-out' });
				if (!cleanupMarker || !claimAmbientCookieMutation(cleanupMarker)) {
					throw new Error('Portico could not publish the superseded session cleanup barrier.');
				}
			}
		}
		catch (error) { quarantineError = error; }
		const cleanupController = new AbortController();
		let logoutError: unknown;
		try { await value.logout(cleanupController.signal); } catch (error) { logoutError = error; }
		let verificationError: unknown;
		try {
			const finalViewer = await value.viewer(cleanupController.signal);
			if (finalViewer.authenticated) throw new Error('The superseded local browser session remained authenticated.');
			if (cleanupMarker) assertBundledCookieMutationOwner(cleanupMarker);
			else {
				const current = ambientCookieRestoreStatus();
				if (!current.trustedForRestore || !current.quarantined) {
					throw new Error('A newer ambient-cookie mutation cleared quarantine before compensation completed.');
				}
			}
		} catch (error) {
			verificationError = error;
		}
		if (quarantineError || verificationError) {
			if (!cleanupMarker) {
				try {
					cleanupMarker = reserveBundledCookieMutation('logout', { state: 'signed-out' });
					if (!cleanupMarker || !claimAmbientCookieMutation(cleanupMarker)) {
						throw new Error('Portico could not publish the failed cleanup barrier.');
					}
				} catch (error) {
					quarantineError = quarantineError ?? error;
				}
			}
			const unsafe = new AggregateError(
				[
					reason,
					...(quarantineError ? [quarantineError] : []),
					...(logoutError ? [logoutError] : []),
					...(verificationError ? [verificationError] : []),
				],
				'Portico could not prove that a superseded local browser session was removed.',
			);
			localProfileSecurityFailure.current = { error: unsafe, scope };
			runtime.failClosed();
			suppressAutomaticSwitch.current = true;
			setAuth((current) => ({
				status: 'error',
				viewer: withoutVerifiedViewer(current.viewer),
				error: canonicalIdentityError(unsafe, 'auth.sign-out-storage-warning'),
			}));
			throw unsafe;
		}
		// A latched publication failure is recoverable only after the same commit
		// lane has verified that the browser is no longer authenticated.
		localProfileSecurityFailure.current = undefined;
		runtime.failClosed();
		suppressAutomaticSwitch.current = true;
		setAuth((current) => ({status: 'ready', viewer: withoutVerifiedViewer(current.viewer)}));
	};
	const finishLocalProfileLogin = async (
		challenge: LocalProfileLoginChallenge,
		profileId: string,
		pin: string | undefined,
		operation: LocalProfileOperation,
	) => {
		let sessionWasFenced = false;
		let mutationAttempted = false;
		let mutationCompensated = false;
		try {
			const selection = await value.verifyLocalProfileSelection(challenge, profileId, pin, operation.controller.signal);
			assertLocalProfileOperation(operation);
			const expected = {
				authority: 'local' as const,
				accountId: challenge.directory.accountId,
				serverId: challenge.directory.serverId,
				profileId,
			};
			if (selection.grant.authority !== expected.authority
				|| selection.grant.accountId !== expected.accountId
				|| selection.grant.serverId !== expected.serverId
				|| selection.grant.profileId !== expected.profileId
				|| Date.parse(selection.grant.expiresAt) <= Date.now()) {
				throw new ReplacementViewerIdentityError();
			}
			if (localProfileSecurityFailure.current) {
				const failure = localProfileSecurityFailure.current;
				await withAmbientCookieMutation('logout', {state: 'signed-out'}, (cleanupMarker) => (
					compensateLocalProfileMutation(failure.error, failure.scope, cleanupMarker)
				));
			}
			await withAmbientCookieMutation('profile-session', {
				state: 'authenticated',
				expected,
			}, async (cookieMarker) => {
				try {
					if (localProfileSecurityFailure.current) throw localProfileSecurityFailure.current.error;
					assertLocalProfileOperation(operation);
					operation.commitStarted = true;
					// From this point the cookie-mutating request is intentionally not
					// aborted by a newer choice. The newer publisher waits for this lane,
					// so a late Set-Cookie cannot land after the winning session.
					const commitController = new AbortController();
					if (runtime.activeScope()) {
						await fenceCurrentViewer('profile-switch');
						sessionWasFenced = true;
					}
					assertLocalProfileOperation(operation);
					mutationAttempted = true;
					const candidateViewer = await value.publishLocalProfileSession(selection, commitController.signal);
					if (operation.generation !== localProfileGeneration.current) {
						mutationCompensated = true;
						await compensateLocalProfileMutation(new DOMException('A newer local profile choice replaced this one.', 'AbortError'), expected, undefined, cookieMarker);
						throw new DOMException('A newer local profile choice replaced this one.', 'AbortError');
					}
					const candidateScope = requiredReplacementScope(candidateViewer);
					assertReplacementIdentity(candidateScope, expected);
					const exactMarker = bindBundledCookieMutation(cookieMarker, candidateScope);
					await applyReplacementViewer(commitController.signal, 'profile-switch', expected, (scope, publish) => {
						assertLocalProfileOperation(operation);
						assertBundledCookieMutationOwner(exactMarker);
						publish();
						clearBundledCookieMutation(exactMarker, scope);
					});
					assertLocalProfileOperation(operation);
					setLocalProfileLogin(undefined);
				} catch (reason) {
					if (mutationAttempted && !mutationCompensated) {
						mutationCompensated = true;
						await compensateLocalProfileMutation(reason, expected, undefined, cookieMarker);
					}
					throw reason;
				}
			});
		} catch (reason) {
			if (operation.generation !== localProfileGeneration.current || operation.controller.signal.aborted) {
				throw new DOMException('A newer local profile choice replaced this one.', 'AbortError');
			}
			const messageId = (reason as { messageId?: unknown } | null)?.messageId;
			if (messageId === 'auth.sign-out-storage-warning' || localProfileSecurityFailure.current) {
				runtime.failClosed();
				suppressAutomaticSwitch.current = true;
				const unsafe = canonicalIdentityError(reason, 'auth.sign-out-storage-warning');
				setAuth((current) => ({ status: 'error', viewer: withoutVerifiedViewer(current.viewer), error: unsafe }));
				throw unsafe;
			}
			if (sessionWasFenced || mutationAttempted || reason instanceof ViewerRuntimeTeardownError) {
				failClosedOnUnsafeTransition(reason, sessionWasFenced);
			}
			throw canonicalIdentityError(reason, 'auth.profile-selection-failed');
		}
	};
	const loginWithLocalProfiles = async (credentials: { login: string; password: string; rememberOnBrowser?: boolean }) => {
		const operation = beginLocalProfileOperation();
		setBusy(true);
		try {
			const challenge = await value.beginLocalProfileLogin(credentials, operation.controller.signal);
			assertLocalProfileOperation(operation);
			if (challenge.directory.authority !== 'local' || Date.parse(challenge.expiresAt) <= Date.now()) {
				throw new ReplacementViewerIdentityError();
			}
			const profiles = [...challenge.directory.profiles].sort((left, right) => left.sortOrder - right.sortOrder);
			if (profiles.length === 0) throw canonicalIdentityError(undefined, 'auth.profile-not-available');
			// Beginning profile selection only creates a short-lived challenge; it
			// does not mutate the browser cookie. Preserve the current verified
			// viewer until the user actually commits a replacement profile session.
			setLocalProfileLogin(challenge);
			if (profiles.length === 1 && !profiles[0].hasPIN) {
				await finishLocalProfileLogin(challenge, profiles[0].id, undefined, operation);
			}
		} catch (reason) {
			if (reason instanceof DOMException && reason.name === 'AbortError') throw reason;
			throw canonicalIdentityError(reason, 'auth.profile-selection-failed');
		} finally {
			if (operation.generation === localProfileGeneration.current) setBusy(false);
		}
	};
	const selectLocalProfile = async (profileId: string, pin?: string) => {
		const challenge = localProfileLogin;
		if (!challenge) throw canonicalIdentityError(undefined, 'auth.profile-selection-required');
		const operation = beginLocalProfileOperation();
		setBusy(true);
		try {
			await finishLocalProfileLogin(challenge, profileId, pin, operation);
		} finally {
			if (operation.generation === localProfileGeneration.current) setBusy(false);
		}
	};
	const switchLocalProfile = async (input: { login: string; password: string; profileId: string; pin?: string }) => {
		const operation = beginLocalProfileOperation();
		setBusy(true);
		try {
			const challenge = await value.beginLocalProfileLogin({
				login: input.login,
				password: input.password,
				rememberOnBrowser: true,
			}, operation.controller.signal);
			assertLocalProfileOperation(operation);
			if (!challenge.directory.profiles.some((profile) => profile.id === input.profileId)) {
				throw canonicalIdentityError(undefined, 'auth.profile-not-available');
			}
			await finishLocalProfileLogin(challenge, input.profileId, input.pin, operation);
		} finally {
			if (operation.generation === localProfileGeneration.current) setBusy(false);
		}
	};
	const switchAuthenticatedLocalProfile = async (profileId: string, pin?: string) => {
		const previous = runtime.activeScope();
		if (!previous || previous.authority !== 'local') {
			throw canonicalIdentityError(undefined, 'auth.profile-selection-required');
		}
		const expected = { ...previous, profileId };
		const operation = beginLocalProfileOperation();
		let mutationAttempted = false;
		let sessionWasFenced = false;
		setBusy(true);
		try {
			await withAmbientCookieMutation('profile-session', { state: 'authenticated', expected }, async (cookieMarker) => {
				try {
					assertLocalProfileOperation(operation);
					operation.commitStarted = true;
					await fenceCurrentViewer('profile-switch');
					sessionWasFenced = true;
					mutationAttempted = true;
					const commitController = new AbortController();
					const candidateViewer = await value.switchAuthenticatedLocalProfile(profileId, pin, commitController.signal);
					const candidateScope = requiredReplacementScope(candidateViewer);
					assertReplacementIdentity(candidateScope, expected);
					const exactMarker = bindBundledCookieMutation(cookieMarker, candidateScope);
					await applyReplacementViewer(commitController.signal, 'profile-switch', expected, (scope, publish) => {
						assertBundledCookieMutationOwner(exactMarker);
						publish();
						clearBundledCookieMutation(exactMarker, scope);
					});
				} catch (reason) {
					if (mutationAttempted) await compensateLocalProfileMutation(reason, expected, undefined, cookieMarker);
					throw reason;
				}
			});
		} catch (reason) {
			if (sessionWasFenced || mutationAttempted || reason instanceof ViewerRuntimeTeardownError) {
				failClosedOnUnsafeTransition(reason, sessionWasFenced);
			}
			throw canonicalIdentityError(reason, 'auth.profile-selection-failed');
		} finally {
			if (operation.generation === localProfileGeneration.current) setBusy(false);
		}
	};

	useEffect(() => {
		if (auth.status !== 'ready' || !auth.viewer?.authenticated || !auth.viewer.viewerScope) return;
		const sync = runtime.viewerSync();
		if (!sync) return;
		const resource = sync.registerResource({
			key: 'web-live-data-cache',
			tags: ['*'],
			priority: 'interactive',
			refresh: async (batch) => {
				const tags = batch.tags.has('runtime:reconcile') ? ['*'] : [...batch.tags];
				// Keep the small revision store for the remaining non-query consumers,
				// but let TanStack own query invalidation directly. This prevents a
				// React render from becoming an accidental prerequisite for refresh.
				liveDataRevisions.publish(tags);
				await queryClient.invalidateQueries({
					predicate: (query) => {
						if (tags.includes('*')) return true;
						const liveTags = query.meta?.liveTags;
						return Array.isArray(liveTags) && liveTags.some((tag) => tag === '*' || (typeof tag === 'string' && tags.includes(tag)));
					},
					refetchType: 'active',
				});
			},
		});
		const onEvent = (event: AppEvent) => {
			if (tagsMayRefreshArtwork(event.tags)) clearArtworkFailureCache();
			if (mayChangeViewerIdentity(event.tags)) {
				// Identity and authorization boundaries are never delayed behind
				// ordinary UI invalidation coalescing.
				sync.invalidate(event.tags, 'immediate');
				void reconcileLiveViewer();
			} else {
				sync.invalidate(event.tags, 'coalesced');
			}
		};
		const onReset = async () => {
			sync.invalidate(['runtime:reconcile'], 'immediate', true);
			await reconcileLiveViewer();
		};
		const subscription = sync.leaseSubscription({
			key: 'application',
			// Keep the lightweight multiplexed stream alive during playback: it
			// also carries authorization and playback-state invalidations. The
			// coordinator still defers resources registered as background.
			priority: 'interactive',
			start: (signal) => scopedValue.watchApplicationEvents(onEvent, onReset, signal as AbortSignal),
		});
		// Data queries begin only after the invalidation consumer is installed.
		// This closes the bootstrap gap where a fast first response could publish
		// before the application event stream was listening.
		setQueryRuntimeReadyScopeKey(runtime.activeScopeKey());
		const publishRuntimeState = () => {
			runtime.setRuntimeState({
				foreground: document.visibilityState === 'visible',
				online: navigator.onLine,
			});
		};
		publishRuntimeState();
		document.addEventListener('visibilitychange', publishRuntimeState);
		window.addEventListener('online', publishRuntimeState);
		window.addEventListener('offline', publishRuntimeState);
		return () => {
			subscription.release();
			resource.release();
			document.removeEventListener('visibilitychange', publishRuntimeState);
			window.removeEventListener('online', publishRuntimeState);
			window.removeEventListener('offline', publishRuntimeState);
		};
	}, [auth.status, auth.viewer?.authenticated, liveDataRevisions, queryClient, reconcileLiveViewer, runtime, scopeGeneration, scopedValue]);

	useEffect(() => {
		runtime.setSyncLifecycleHandler(() => { void reconcileLiveViewer(); });
		return () => runtime.setSyncLifecycleHandler(undefined);
	}, [reconcileLiveViewer, runtime]);

	const authValue: AuthContextValue = {
    ...auth,
    busy,
	browserAccounts,
	sessionRevision,
	viewerScopeKey: runtime.activeScopeKey(),
	addingAccount,
	localProfileLogin,
	login: loginWithLocalProfiles,
	selectLocalProfile,
	cancelLocalProfileLogin: () => {
		activeLocalProfileOperation.current?.controller.abort();
		localProfileGeneration.current += 1;
		activeLocalProfileOperation.current = undefined;
		setLocalProfileLogin(undefined);
		setBusy(false);
	},
	setup: (details) => runAuthMutation((signal) => value.setup(details, signal), true),
    startPorticoSetup: async (serverName) => {
      const controller = new AbortController();
      setBusy(true);
      try {
        return await value.startPorticoSetup(serverName, controller.signal);
	  } catch (reason) {
		throw canonicalIdentityError(reason);
      } finally {
        setBusy(false);
      }
    },
    porticoSetupStatus: async () => {
      const controller = new AbortController();
      try {
        const status = await value.porticoSetupStatus(controller.signal);
        return {
          setupRequired: status.setupRequired,
          claimStatus: status.remoteAccess.settings.claimStatus,
          porticoConnected: status.remoteAccess.porticoConnected,
        };
      } catch (reason) {
        throw canonicalIdentityError(reason);
      }
    },
	logout: async () => {
		const controller = new AbortController();
		if (!activeLocalProfileOperation.current?.commitStarted) activeLocalProfileOperation.current?.controller.abort();
		localProfileGeneration.current += 1;
		activeLocalProfileOperation.current = undefined;
		setLocalProfileLogin(undefined);
		setBusy(true);
		try {
			const { cleanupError, serverError } = await withAmbientCookieMutation('logout', { state: 'signed-out' }, async (marker) => {
				const cleanupError = await fenceSignedOutViewer();
				let serverError: unknown;
				try { await value.logout(controller.signal); } catch (reason) { serverError = reason; }
				assertBundledCookieMutationOwner(marker);
				return { cleanupError, serverError };
			});
			suppressAutomaticSwitch.current = true;
			setAuth((current) => ({ status: 'ready', viewer: withoutVerifiedViewer(current.viewer) }));
			setSessionRevision((current) => current + 1);
			setBrowserRevision((current) => current + 1);
			if (serverError || cleanupError) {
				throw canonicalIdentityError(
					serverError ?? cleanupError,
					localSessionQuarantineEnabled
						? 'auth.sign-out-storage-warning'
						: 'problem.request-failed',
				);
			}
		} finally { setBusy(false); }
	},
		switchBrowserAccount: async (accountId) => {
			const controller = new AbortController();
			const currentScope = runtime.activeScope();
			const sessionWasFenced = Boolean(currentScope);
			const selectedAccount = browserAccounts.data.accounts.find((account) => account.id === accountId);
			setBusy(true);
			try {
				if (!selectedAccount) throw new ReplacementViewerIdentityError();
				suppressAutomaticSwitch.current = false;
				await switchToBrowserAccount(selectedAccount, controller.signal, currentScope);
		} catch (reason) {
			const error = canonicalIdentityError(reason);
			failClosedOnUnsafeTransition(reason, sessionWasFenced);
			setBrowserAccounts((current) => ({ ...current, error }));
			throw error;
		} finally { setBusy(false); }
	},
		switchLocalProfile,
		switchAuthenticatedLocalProfile,
			switchHostedProfile: async (input) => {
				const controller = new AbortController();
				const previousScope = runtime.activeScope();
				setBusy(true);
				try {
					// Hosted RuntimeProvider owns this replacement as one staged
					// server-credential/runtime transaction. Do not fence the current
					// viewer here: doing so erased the rollback target before the staged
					// connector had a chance to restore it after a PIN or network failure.
					const viewer = await value.switchHostedProfile(input, controller.signal);
					const candidateScope = requiredReplacementScope(viewer);
					const publishedScope = runtime.activeScope();
					if (!publishedScope || !sameViewerScope(publishedScope, candidateScope)) {
						throw new ReplacementViewerIdentityError();
					}
					setAuth((current) => ({ status: 'ready', viewer: { ...viewer, authCapabilities: viewer.authCapabilities ?? current.viewer?.authCapabilities } }));
					setAddingAccount(false);
					setSessionRevision((current) => current + 1);
					setBrowserRevision((current) => current + 1);
				} catch (reason) {
					const error = canonicalIdentityError(reason, 'auth.profile-selection-failed');
					const restoredScope = runtime.activeScope();
					if (previousScope && restoredScope && sameViewerScope(previousScope, restoredScope) && !runtime.isTransitioning()) {
						setAuth((current) => ({ ...current, status: 'ready', error }));
					} else {
						failClosedOnUnsafeTransition(reason, true);
					}
					throw error;
				} finally {
					setBusy(false);
				}
			},
	updateAutomaticSignIn: async (automaticSignIn) => {
		const controller = new AbortController();
		setBusy(true);
		try {
			await value.updateBrowserAccountPreferences(automaticSignIn, controller.signal);
			setBrowserAccounts((current) => ({ ...current, data: { ...current.data, automaticSignIn } }));
		} catch (reason) {
			throw canonicalIdentityError(reason);
		} finally { setBusy(false); }
	},
	removeBrowserAccount: async (accountId) => {
		const controller = new AbortController();
		const currentScope = runtime.activeScope();
		const removingActive = currentScope?.accountId === accountId;
		setBusy(true);
		try {
			const intent = removingActive || !currentScope
				? { state: 'signed-out' as const }
				: { state: 'authenticated' as const, expected: currentScope };
			const { result, cleanupError } = await withAmbientCookieMutation('browser-account-remove', intent, async (marker) => {
				const cleanupError = removingActive ? await fenceSignedOutViewer() : undefined;
				const result = await value.removeBrowserAccount(accountId, controller.signal);
				if (!removingActive && currentScope) {
					if (result.activeAccountRemoved) {
						throw canonicalIdentityError(undefined, 'auth.sign-out-storage-warning');
					}
					const finalViewer = await value.viewer(controller.signal);
					const finalScope = requiredReplacementScope(finalViewer);
					assertViewerIdentity(finalScope, currentScope);
					const exact = bindBundledCookieMutation(marker, finalScope);
					clearBundledCookieMutation(exact, finalScope);
				} else assertBundledCookieMutationOwner(marker);
				return { result, cleanupError };
			});
			if (result.activeAccountRemoved || removingActive) {
				suppressAutomaticSwitch.current = true;
				setAuth((current) => ({ status: 'ready', viewer: withoutVerifiedViewer(current.viewer) }));
				setSessionRevision((current) => current + 1);
			}
			setBrowserRevision((current) => current + 1);
			if (cleanupError) throw cleanupError;
			return result;
		} catch (reason) {
			if (removingActive) setAuth((current) => ({ status: 'ready', viewer: withoutVerifiedViewer(current.viewer) }));
			if (localSessionQuarantineEnabled) {
				runtime.failClosed();
				suppressAutomaticSwitch.current = true;
				const unsafe = canonicalIdentityError(reason, 'auth.sign-out-storage-warning');
				setAuth((current) => ({status: 'error', viewer: withoutVerifiedViewer(current.viewer), error: unsafe}));
				throw unsafe;
			}
			throw canonicalIdentityError(reason);
		} finally { setBusy(false); }
	},
	signOutAllBrowserAccounts: async () => {
		const controller = new AbortController();
		setBusy(true);
		try {
			const { cleanupError, serverError } = await withAmbientCookieMutation('browser-account-sign-out-all', { state: 'signed-out' }, async (marker) => {
				const cleanupError = await fenceSignedOutViewer();
				let serverError: unknown;
				try { await value.signOutAllBrowserAccounts(controller.signal); } catch (reason) { serverError = reason; }
				assertBundledCookieMutationOwner(marker);
				return { cleanupError, serverError };
			});
			setAuth((current) => ({ status: 'ready', viewer: withoutVerifiedViewer(current.viewer) }));
			setBrowserAccounts({ status: 'ready', data: emptyBrowserAccounts });
			setSessionRevision((current) => current + 1);
			if (serverError || cleanupError) {
				throw canonicalIdentityError(
					serverError ?? cleanupError,
					localSessionQuarantineEnabled
						? 'auth.sign-out-storage-warning'
						: 'problem.request-failed',
				);
			}
		} finally { setBusy(false); }
	},
	beginAddAccount: () => setAddingAccount(true),
	cancelAddAccount: () => setAddingAccount(false),
	retryBrowserAccounts: () => {
		automaticSwitchAttempt.current = '';
		suppressAutomaticSwitch.current = false;
		setBrowserRevision((current) => current + 1);
	},
	registerSessionTeardown: (teardown) => {
		return runtime.register('playback', teardown);
	},
	registerRuntimeTeardown: (kind, teardown) => runtime.register(kind, teardown),
    updateProfile: (profile) => runAuthMutation((signal) => value.updateProfile(profile, signal)),
    refresh: () => setRevision((current) => current + 1),
  };

  const queryRuntimeReady = Boolean(activeQueryScopeKey) && queryRuntimeReadyScopeKey === activeQueryScopeKey;
  return <ViewerRuntimeContext.Provider value={runtime}><DataSourceContext.Provider value={scopedValue}><LiveDataRevisionContext.Provider value={liveDataRevisions}><AuthContext.Provider value={authValue}><QueryClientProvider client={queryClient}><QueryRuntimeReadyContext.Provider value={queryRuntimeReady}>{children}</QueryRuntimeReadyContext.Provider></QueryClientProvider></AuthContext.Provider></LiveDataRevisionContext.Provider></DataSourceContext.Provider></ViewerRuntimeContext.Provider>;
}

export function usePorticoDataSource() {
  const source = useContext(DataSourceContext);
  if (!source) throw new Error('Portico data hooks must be used inside DataProvider.');
  return source;
}

export function useAuthSession() {
  const auth = useContext(AuthContext);
  if (!auth) throw new Error('Authentication hooks must be used inside DataProvider.');
  return auth;
}

export function useOptionalAuthSession() {
  return useContext(AuthContext);
}

export function useViewerRuntime() {
	const runtime = useContext(ViewerRuntimeContext);
	if (!runtime) throw new Error('Viewer runtime hooks must be used inside DataProvider.');
	return runtime;
}

export function useOptionalViewerRuntime() {
	return useContext(ViewerRuntimeContext);
}

/**
 * Returns a monotonic revision for the requested server-owned data domains.
 * Consumers can quietly refresh retained state without remounting their UI.
 */
export function useLiveDataRevision(tags: readonly string[]): number {
  const revisions = useContext(LiveDataRevisionContext);
  const tagKey = tags.join('\u0000');
  const stableTags = useMemo(() => [...tags], [tagKey]);
  const subscribe = useCallback((listener: () => void) => revisions.subscribe(stableTags, listener), [revisions, stableTags]);
  const snapshot = useCallback(() => revisions.revision(stableTags), [revisions, stableTags]);
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

type SourceQueryOptions<T> = {
	enabled?: boolean;
	initialData?: T;
};

function isAuthoritativeNotFound(reason: unknown): reason is Error {
	if (!(reason instanceof Error)) return false;
	const problem = reason as Error & { status?: unknown };
	return problem.status === 404;
}

function useSourceQuery<T>(key: string, load: (source: PorticoDataSource, signal: AbortSignal) => Promise<T>, liveTags: readonly string[] = [], refreshRevision = 0, options: SourceQueryOptions<T> = {}): QueryState<T> {
  const source = usePorticoDataSource();
	const runtime = useViewerRuntime();
	const queryClient = useQueryClient();
	const queryRuntimeReady = useContext(QueryRuntimeReadyContext);
	const scope = runtime.activeScope();
	const liveTagKey = liveTags.join('\u0000');
	const stableLiveTags = useMemo(() => [...liveTags], [liveTagKey]);
	const resource = key.split(':', 1)[0] || 'query';
	const queryKey = useMemo(() => scope
		? viewerQueryKey(scope, resource, { identity: key })
		: ['portico', 'unscoped', resource, key] as const, [key, resource, scope]);
	const queryIdentity = `${runtime.activeScopeKey()}\u0000${key}`;
	const previousRefreshRevision = useRef(refreshRevision);
	const previousInitialData = useRef(options.initialData);
	const authoritativeNotFound = useRef<{ identity: string; error: Error } | undefined>(undefined);
	const query = useQuery({
		enabled: Boolean(scope) && queryRuntimeReady && options.enabled !== false,
		initialData: options.initialData,
		meta: { liveTags: stableLiveTags },
		queryFn: ({ signal }) => load(source, signal),
		queryKey,
	});
	const explicitRefresh = previousRefreshRevision.current !== refreshRevision;
	if (authoritativeNotFound.current?.identity !== queryIdentity || explicitRefresh) {
		authoritativeNotFound.current = undefined;
	}
	if (query.data !== undefined) authoritativeNotFound.current = undefined;
	else if (!explicitRefresh && isAuthoritativeNotFound(query.error)) authoritativeNotFound.current = { identity: queryIdentity, error: query.error };
	useEffect(() => {
		recordRouteDataState(key, query.error ? 'error' : query.data !== undefined ? 'success' : 'loading', query.error);
	}, [key, query.data, query.error]);

	useEffect(() => {
		if (options.initialData === undefined || previousInitialData.current === options.initialData) return;
		previousInitialData.current = options.initialData;
		queryClient.setQueryData(queryKey, options.initialData);
	}, [options.initialData, queryClient, queryKey]);

  useEffect(() => {
		const refreshChanged = previousRefreshRevision.current !== refreshRevision;
		previousRefreshRevision.current = refreshRevision;
		if (refreshChanged) void queryClient.invalidateQueries({ exact: true, queryKey });
	}, [queryClient, queryKey, refreshRevision]);

	return useMemo<QueryState<T>>(() => {
		if (authoritativeNotFound.current) return { status: 'error', error: authoritativeNotFound.current.error };
		const error = query.error instanceof Error ? query.error : query.error ? new Error('Portico request failed.') : undefined;
		const lastSuccessAt = query.dataUpdatedAt > 0 ? query.dataUpdatedAt : undefined;
		if (query.data !== undefined) return {
			status: 'success',
			data: query.data,
			error,
			// A background refresh is ordinary activity, not a user-visible stale
			// condition. Preserve the separate isFetching signal for subtle progress
			// UI and reserve stale/error projection for aged or failed data.
			stale: Boolean(error) || query.isStale,
			isFetching: query.isFetching,
			lastSuccessAt,
		};
		if (query.error) return { status: 'error', error: query.error instanceof Error ? query.error : new Error('Portico request failed.') };
		return { status: 'loading', stale: false, isFetching: query.isFetching };
	}, [query.data, query.dataUpdatedAt, query.error, query.isFetching, query.isStale]);
}

/**
 * Viewer-fenced escape hatch for feature-specific server contracts. New Web
 * surfaces should use this instead of rebuilding request state, retry, and
 * stale-data behavior in component-local effects.
 */
export function usePorticoQuery<T>(
	key: string,
	load: (source: PorticoDataSource, signal: AbortSignal) => Promise<T>,
	liveTags: readonly string[] = [],
	refreshRevision = 0,
	options: SourceQueryOptions<T> = {},
): QueryState<T> {
	return useSourceQuery(key, load, liveTags, refreshRevision, options);
}

export function useHome(reloadKey = 0): QueryState<HomeResult> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.home(signal), []);
  return useSourceQuery('home', load, ['home', 'libraries', 'library-items', 'media', 'metadata', 'playback-progress', 'media-state', 'playlists'], reloadKey);
}

export function useSearchContract(reloadKey = 0) {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.searchContract(signal), []);
  return useSourceQuery('search-contract', load, [], reloadKey);
}

export function useProductContract(reloadKey = 0) {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.productContract(signal), []);
  return useSourceQuery('product-contract', load, [], reloadKey);
}

export function useHomeRow(id: string, cursor?: string, reloadKey = 0, options: SourceQueryOptions<HomeRow> = {}): QueryState<HomeRow> {
  const normalizedId = id.trim();
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => normalizedId
    ? source.homeRow(normalizedId, cursor, signal)
    : Promise.reject(new Error('No Home row was selected.')), [normalizedId, cursor]);
  return useSourceQuery(`home-row:${normalizedId}:${cursor ?? ''}`, load, ['home', 'libraries', 'library-items', 'media', 'metadata', 'playback-progress', 'media-state', 'playlists'], reloadKey, options);
}

export function useLibraries(reloadKey = 0) {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.libraries(signal), []);
  return useSourceQuery('libraries', load, ['libraries'], reloadKey);
}

export function useQuickSearch(query: string): QueryState<SearchResult[]> {
  const normalized = query.trim();
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => normalized ? source.search(normalized, signal) : Promise.resolve([]), [normalized]);
  return useSourceQuery(`search:${normalized}`, load, ['search', 'media', 'metadata', 'library-items']);
}

export function useSearchPage(input: string | SearchPageInput): QueryState<SearchPageResult> {
  const request = typeof input === 'string' ? { query: input.trim() } : { ...input, query: input.query.trim() };
  const key = JSON.stringify(request);
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => request.query
    ? source.searchPage(request, signal)
    : Promise.resolve<SearchPageResult>({ query: '', sort: 'relevance', direction: 'desc', groups: [] }), [key]);
  return useSourceQuery(`search-page:${key}`, load, ['search', 'media', 'metadata', 'library-items']);
}

export function useMediaDetail(id: string | undefined, reloadKey = 0): QueryState<MediaItem> {
  const mediaId = id?.trim() ?? '';
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => mediaId
    ? source.media(mediaId, signal)
    : Promise.reject(new Error('No media item was selected.')), [mediaId]);
  return useSourceQuery(`media:${mediaId}`, load, ['media', 'metadata', 'library-items', 'playback-progress', 'media-state'], reloadKey);
}

export function usePersonDetail(id: string | undefined, reloadKey = 0): QueryState<PersonDetail> {
  const personId = id?.trim() ?? '';
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => personId
    ? source.person(personId, signal)
    : Promise.reject(new Error('No person was selected.')), [personId]);
  return useSourceQuery(`person:${personId}`, load, ['media', 'metadata', 'library-items'], reloadKey);
}

export function useWatchlist(reloadKey = 0): QueryState<MediaItem[]> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.watchlist(signal), []);
  return useSourceQuery('watchlist', load, ['media-state', 'playback-progress', 'media'], reloadKey);
}

export function useFavorites(reloadKey = 0): QueryState<MediaItem[]> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.favorites(signal), []);
  return useSourceQuery('favorites', load, ['media-state', 'playback-progress', 'media'], reloadKey);
}

export function useSavedResources(kind: SavedResourceKind, reloadKey = 0): QueryState<SavedResourceSummary[]> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.savedResources(kind, signal), [kind]);
  return useSourceQuery(`saved-resources:${kind}`, load, ['playlists', 'collections', 'saved', 'media-state', 'media', 'library-items'], reloadKey);
}

export function useSavedResource<K extends SavedResourceKind>(kind: K, id: string | undefined, input: SavedResourceItemsInput = {}, reloadKey = 0): QueryState<SavedResourceDetail<K>> {
  const resourceId = id?.trim() ?? '';
  const cursor = input.cursor;
  const limit = input.limit;
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => resourceId
    ? source.savedResource(kind, resourceId, { cursor, limit }, signal)
    : Promise.reject(new Error('No saved resource was selected.')), [kind, resourceId, cursor, limit]);
  return useSourceQuery<SavedResourceDetail<K>>(`saved-resource:${kind}:${resourceId}:${cursor ?? 'first'}:${limit ?? 'default'}`, load, ['playlists', 'collections', 'saved', 'media-state', 'media', 'library-items'], reloadKey);
}

export function useLiveTVSources(reloadKey = 0): QueryState<ActionableLiveTVSource[]> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.liveTVSources(signal), []);
  return useSourceQuery('live-tv-sources', load, ['live-tv'], reloadKey);
}

export function useLiveTVGuide(sourceId: string, input: LiveTVGuideInput, reloadKey = 0): QueryState<LiveTVGuideResult> {
  const key = `${sourceId}:${JSON.stringify(input)}`;
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => sourceId
    ? source.liveTVGuide(sourceId, input, signal)
    : Promise.reject(new Error('No Live TV source is selected.')), [key]);
  return useSourceQuery(`live-tv-guide:${key}`, load, ['live-tv', 'dvr'], reloadKey);
}

export function useLiveTVChannels(sourceId: string, reloadKey = 0): QueryState<ActionableLiveTVChannel[]> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => sourceId
    ? source.liveTVChannels(sourceId, signal)
    : Promise.reject(new Error('No Live TV source is selected.')), [sourceId]);
  return useSourceQuery(`live-tv-channels:${sourceId}`, load, ['live-tv'], reloadKey);
}

export function useDVR(reloadKey = 0): QueryState<DVRResult> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.dvr(signal), []);
  return useSourceQuery('dvr', load, ['dvr', 'live-tv'], reloadKey);
}

export function useLibraryChannels(reloadKey = 0): QueryState<LibraryChannelListResponse> {
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.libraryChannels(signal), []);
  return useSourceQuery('library-channels', load, ['library-channels', 'live-tv'], reloadKey);
}

export function useLibraryChannelGuide(channelId: string, input: { from?: string; to?: string; limit?: number }, reloadKey = 0): QueryState<LibraryChannelGuide> {
  const key = `${channelId}:${JSON.stringify(input)}`;
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => channelId
    ? source.libraryChannelGuide(channelId, input, signal)
    : Promise.reject(new Error('No Library Channel is selected.')), [key]);
  return useSourceQuery(`library-channel-guide:${key}`, load, ['library-channels', 'live-tv'], reloadKey);
}

export function useLibraryChannelsGuide(input: { from?: string; to?: string; limit?: number }, reloadKey = 0): QueryState<LibraryChannelsGuide> {
  const key = JSON.stringify(input);
  const load = useMemo(() => (source: PorticoDataSource, signal: AbortSignal) => source.libraryChannelsGuide(input, signal), [key]);
  return useSourceQuery(`library-channels-guide:${key}`, load, ['library-channels', 'live-tv'], reloadKey);
}

export function useLibraryChannelMutations() {
  const source = usePorticoDataSource();
  return useMemo(() => ({
    tune: (channelId: string) => source.startLibraryChannelPlayback(channelId, new AbortController().signal),
  }), [source]);
}

export function useLiveTVMutations() {
  const source = usePorticoDataSource();
  return useMemo(() => ({
    favoriteChannel: (id: string, favorite: boolean) => source.updateLiveTVChannel(id, { favorite }, new AbortController().signal),
    recordProgram: (input: { sourceId: string; channelId?: string; programId?: string; title: string; startsAt: string; endsAt: string }) => source.createDVRRecording(input, new AbortController().signal),
    recordSeries: (input: { sourceId: string; channelId?: string; programId?: string; title: string; matchType: string }) => source.createDVRRule(input, new AbortController().signal),
    updateRule: (id: string, input: Parameters<PorticoDataSource['updateDVRRule']>[1]) => source.updateDVRRule(id, input, new AbortController().signal),
    deleteRecording: (id: string) => source.deleteDVRRecording(id, new AbortController().signal),
    deleteRule: (id: string) => source.deleteDVRRule(id, new AbortController().signal),
  }), [source]);
}

export function useMediaMutations() {
  const source = usePorticoDataSource();
  return useMemo(() => ({
    setWatchlist: (id: string, watchlisted: boolean) => source.setWatchlist(id, watchlisted, new AbortController().signal),
    setFavorite: (id: string, favorite: boolean) => source.setFavorite(id, favorite, new AbortController().signal),
    setReaction: (id: string, reaction: '' | 'like' | 'dislike') => source.setReaction(id, reaction, new AbortController().signal),
    setRating: (id: string, rating: number) => source.setRating(id, rating, new AbortController().signal),
    setWatched: (id: string, watched: boolean) => source.setWatched(id, watched, new AbortController().signal),
    updateMetadata: (ids: string[], patch: Parameters<PorticoDataSource['updateMediaMetadata']>[1]) => source.updateMediaMetadata(ids, patch, new AbortController().signal),
    uploadArtwork: (id: string, type: string, file: File, expectedRevision: number) => source.uploadMediaImage(id, type, file, expectedRevision, new AbortController().signal),
    deleteArtwork: (id: string, imageId: string, expectedRevision: number) => source.deleteMediaImage(id, imageId, expectedRevision, new AbortController().signal),
    setPreferredArtwork: (id: string, imageId: string, expectedRevision: number) => source.setPreferredMediaImage(id, imageId, expectedRevision, new AbortController().signal),
    reorderArtwork: (id: string, imageIds: string[], expectedRevision: number) => source.reorderMediaImages(id, imageIds, expectedRevision, new AbortController().signal),
    uploadSubtitle: (id: string, file: File, language: string, label: string) => source.uploadSubtitle(id, file, language, label, new AbortController().signal),
    updateSubtitle: (id: string, streamId: string, offsetMs: number) => source.updateSubtitle(id, streamId, offsetMs, new AbortController().signal),
    deleteSubtitle: (id: string, streamId: string) => source.deleteSubtitle(id, streamId, new AbortController().signal),
    uploadLyrics: (id: string, file: File, language: string) => source.uploadLyrics(id, file, language, new AbortController().signal),
    fetchLyrics: (id: string) => source.fetchLyrics(id, new AbortController().signal),
    searchLyrics: (id: string, query: string) => source.searchLyrics(id, query, new AbortController().signal),
    applyLyrics: (id: string, candidate: Parameters<PorticoDataSource['applyLyrics']>[1]) => source.applyLyrics(id, candidate, new AbortController().signal),
    deleteLyrics: (id: string, lyricId: string) => source.deleteLyrics(id, lyricId, new AbortController().signal),
    searchMatches: (id: string, query: string) => source.searchMediaMatches(id, query, new AbortController().signal),
    applyMatch: (id: string, candidate: Parameters<PorticoDataSource['applyMediaMatch']>[1], expectedRevision: number) => source.applyMediaMatch(id, candidate, expectedRevision, new AbortController().signal),
    deleteMedia: (id: string, input: Parameters<PorticoDataSource['deleteMedia']>[1]) => source.deleteMedia(id, input, new AbortController().signal),
  }), [source]);
}

export function useMediaOperations() {
  const source = usePorticoDataSource();
  return useMemo(() => ({
    queueJob: (id: string, type: Parameters<PorticoDataSource['queueMediaJob']>[1], options: Parameters<PorticoDataSource['queueMediaJob']>[2]) => source.queueMediaJob(id, type, options, new AbortController().signal),
    downloadOptions: (id: string) => source.mediaDownloadOptions(id, new AbortController().signal),
    downloadPreparations: () => source.downloadPreparations(new AbortController().signal),
    updateDownloadPreparation: (id: string, action: 'pause' | 'resume' | 'cancel' | 'retry' | 'remove') => source.updateDownloadPreparation(id, action, new AbortController().signal),
    downloadPreparationURL: (id: string) => source.downloadPreparationURL(id, new AbortController().signal),
    createOptimizedVersion: (id: string, profile: string) => source.createOptimizedVersion(id, profile, new AbortController().signal),
    deleteOptimizedVersion: (id: string, profile: string) => source.deleteOptimizedVersion(id, profile, new AbortController().signal),
    createDownloadURL: (id: string, profile: string) => source.createMediaDownloadURL(id, profile, new AbortController().signal),
  }), [source]);
}

export function useSavedMutations() {
  const source = usePorticoDataSource();
  return useMemo(() => ({
    create: (kind: SavedResourceKind, input: Parameters<PorticoDataSource['createSavedResource']>[1]) => source.createSavedResource(kind, input, new AbortController().signal),
    update: (kind: SavedResourceKind, id: string, input: Parameters<PorticoDataSource['updateSavedResource']>[2]) => source.updateSavedResource(kind, id, input, new AbortController().signal),
    delete: (kind: SavedResourceKind, id: string) => source.deleteSavedResource(kind, id, new AbortController().signal),
    mutateItems: <K extends SavedEditableResourceKind>(kind: K, id: string, mutation: SavedResourceItemsMutation<K>) => source.mutateSavedResourceItems(kind, id, mutation, new AbortController().signal),
  }), [source]);
}
