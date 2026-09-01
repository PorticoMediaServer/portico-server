import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useEffect, useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, productMessage, type AppEvent } from '@porticomediaserver/client-core';
import { DataProvider, useAuthSession, useHome, useMediaDetail, usePorticoDataSource } from './DataProvider';
import { FixturePorticoDataSource } from './fixtureSource';
import type { HomeResult, LocalProfileLoginChallenge, LocalProfileSelection, MediaItem, Viewer } from './models';
import { WebViewerRuntime } from './viewerRuntime';
import { markLocalSessionSignedOut } from '../runtime/localSessionQuarantine';
import { ambientCookieRestoreStatus } from '../runtime/ambientCookieQuarantine';

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function viewer(profileId: string, authorizationRevision: string): Viewer {
  return {
    authenticated: true,
    setupRequired: false,
    serverName: 'Scope Test Server',
    viewerScope: {
      authority: 'hosted',
      accountId: 'one-household',
      serverId: 'server-one',
      profileId,
      authorizationRevision,
    },
    user: {
      id: 'one-household',
      displayName: profileId,
      email: 'household@example.test',
      role: 'user',
      permissions: { playMedia: true },
      preferences: { sidebarOrder: [] },
    },
  };
}

function localViewer(profileId: string, authorizationRevision: string): Viewer {
  const result = viewer(profileId, authorizationRevision);
  result.viewerScope = {
    authority: 'local',
    accountId: 'local-account',
    serverId: 'local-server',
    profileId,
    authorizationRevision,
  };
  result.user!.id = 'local-account';
  return result;
}

function accountViewer(accountId: string, profileId: string, authorizationRevision: string): Viewer {
  const result = viewer(profileId, authorizationRevision);
  result.viewerScope!.accountId = accountId;
  result.user!.id = accountId;
  return result;
}

function selectableAccounts(source: SessionSource, targetId: string) {
  source.browserAccountState = {
    accounts: [
      {id: 'one-household', displayName: 'Current', profileImageUrl: undefined, authOrigin: 'portico', authProvider: 'portico', lastUsedAt: '2026-07-14T10:00:00.000Z'},
      {id: targetId, displayName: targetId, profileImageUrl: undefined, authOrigin: 'portico', authProvider: 'portico', lastUsedAt: '2026-07-14T11:00:00.000Z'},
    ],
    activeAccountId: 'one-household',
    automaticSignIn: false,
    selectionRequired: true,
    canAddAccount: true,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

class SessionSource extends FixturePorticoDataSource {
  finalViewer: Viewer;
  viewerResult?: Promise<Viewer>;
  viewerError?: Error;
  switchViewer?: Viewer;
  browserAccountState: Awaited<ReturnType<FixturePorticoDataSource['browserAccounts']>> = {
    accounts: [{ id: 'one-household', displayName: 'Adult', profileImageUrl: undefined, authOrigin: 'portico', authProvider: 'portico', lastUsedAt: '2026-07-14T10:00:00.000Z' }],
    activeAccountId: 'one-household',
    automaticSignIn: true,
    selectionRequired: false,
    canAddAccount: true,
  };
  logoutCalled = false;
	hostedProfileSwitchError?: Error;

  constructor(initial: Viewer) {
    super(initial);
    this.finalViewer = initial;
  }

  override async viewer(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (this.viewerError) throw this.viewerError;
    return structuredClone(this.viewerResult ? await this.viewerResult : this.finalViewer);
  }

  override async browserAccounts(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return structuredClone(this.browserAccountState);
  }

  override async switchBrowserAccount(_accountId: string, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return structuredClone(this.switchViewer ?? this.finalViewer);
  }

	override async switchHostedProfile(_input: { profileId: string; pin?: string }, signal: AbortSignal) {
		if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
		if (this.hostedProfileSwitchError) throw this.hostedProfileSwitchError;
		return structuredClone(this.switchViewer ?? this.finalViewer);
	}

  override async logout(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    this.logoutCalled = true;
  }
}

class LocalProfileSource extends SessionSource {
  publishCalls = 0;
  selection?: LocalProfileSelection;
  readonly challenge: LocalProfileLoginChallenge = {
    accountAuthenticationToken: 'local-proof',
    expiresAt: new Date(Date.now() + 60_000).toISOString(),
    installationId: 'installation-local',
    rememberOnBrowser: true,
    directory: {
      authority: 'local',
      accountId: 'local-account',
      serverId: 'local-server',
      profilesAllowed: true,
      profiles: [{
        id: 'local-profile',
        name: 'Local profile',
        isPrimary: true,
        isAccountAdmin: true,
        hasPIN: true,
        pinRevision: 2,
        sortOrder: 0,
        policy: { version: 'v1', maximumAgeRating: null, allowUnrated: true, blockedLabels: [], allowDownloads: true, allowLiveTV: true, allowDvr: true, allowWatchWithFriends: true, allowFeedback: true },
      }],
    },
  };

  override async beginLocalProfileLogin(_credentials: { login: string; password: string }, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return structuredClone(this.challenge);
  }

  override async verifyLocalProfileSelection(_challenge: LocalProfileLoginChallenge, _profileId: string, _pin: string | undefined, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!this.selection) throw new Error('Test selection was not configured.');
    return structuredClone(this.selection);
  }

  override async publishLocalProfileSession(_selection: LocalProfileSelection, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    this.publishCalls += 1;
    return this.finalViewer;
  }
}

class LiveHomeSource extends SessionSource {
  homeCalls = 0;
  watchCalls = 0;
  nextHome?: Promise<HomeResult>;
  private appEvent?: (event: AppEvent) => void;
  private appReset?: () => void | Promise<void>;

  override async home(_signal: AbortSignal) {
    this.homeCalls += 1;
    return this.nextHome ? this.nextHome : home(`Home ${this.homeCalls}`);
  }

  override async watchApplicationEvents(onEvent: (event: AppEvent) => void, onReset: () => void | Promise<void>, signal: AbortSignal) {
	this.watchCalls += 1;
    this.appEvent = onEvent;
    this.appReset = onReset;
    if (signal.aborted) return;
    await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve(), { once: true }));
  }

  publish(tags: string[], scope: { resource?: string; resourceId?: string; type?: string } = {}) {
    this.appEvent?.({
		id: Date.now(),
		type: scope.type ?? 'data.changed',
		tags,
		resource: scope.resource ?? '',
		resourceId: scope.resourceId ?? '',
		fields: {},
		createdAt: new Date().toISOString(),
	});
  }

  reset() {
    return this.appReset?.();
  }
}

function Probe({
  failingTeardown = false,
  switchAccountId = 'one-household',
  onPublication,
}: {
  failingTeardown?: boolean;
  switchAccountId?: string;
  onPublication?: (accountId: string) => void;
}) {
  const auth = useAuthSession();
	const publication = auth.status === 'ready' && auth.viewer?.authenticated ? auth.viewer.viewerScope?.accountId ?? 'missing-scope' : 'blocked';
	const structuredError = auth.error as (Error & { requestId?: string; retryAfterMs?: number }) | undefined;
  useEffect(() => failingTeardown ? auth.registerRuntimeTeardown('realtime', async () => {
    throw new Error('realtime close failed');
  }) : undefined, [auth.registerRuntimeTeardown, failingTeardown]);
  useEffect(() => { onPublication?.(publication); }, [onPublication, publication]);
  return <>
    <output aria-label="status">{auth.status}</output>
    <output aria-label="profile">{auth.viewer?.viewerScope?.profileId ?? 'none'}</output>
    <output aria-label="scope-key">{auth.viewerScopeKey}</output>
    <output aria-label="browser-accounts-status">{auth.browserAccounts.status}</output>
    <output aria-label="product-publication">{publication}</output>
		<output aria-label="identity-error">{auth.error?.message ?? 'none'}</output>
		<output aria-label="identity-request-id">{structuredError?.requestId ?? 'none'}</output>
		<output aria-label="identity-retry-ms">{structuredError?.retryAfterMs ?? 'none'}</output>
    <button type="button" onClick={() => void auth.switchBrowserAccount(switchAccountId).catch(() => undefined)}>Switch</button>
    <button type="button" onClick={() => void auth.switchHostedProfile({ profileId: 'child', pin: '1234' }).catch(() => undefined)}>Switch hosted profile</button>
    <button type="button" onClick={auth.refresh}>Refresh viewer</button>
    <button type="button" onClick={() => void auth.logout().catch(() => undefined)}>Sign out</button>
    <button type="button" onClick={() => void auth.removeBrowserAccount('one-household').catch(() => undefined)}>Remove active</button>
    <button type="button" onClick={() => void auth.signOutAllBrowserAccounts().catch(() => undefined)}>Sign out all</button>
  </>;
}

function LocalProfileProbe() {
  const auth = useAuthSession();
  return <>
	<output aria-label="local-status">{auth.status}</output>
	<output aria-label="local-error">{auth.error?.message ?? 'none'}</output>
    <output aria-label="profile">{auth.viewer?.viewerScope?.profileId ?? 'none'}</output>
    <output aria-label="authorization-revision">{auth.viewer?.viewerScope?.authorizationRevision ?? 'none'}</output>
    <output aria-label="local-challenge">{auth.localProfileLogin?.directory.accountId ?? 'none'}</output>
    <button type="button" onClick={() => void auth.login({ login: 'owner', password: 'password' }).catch(() => undefined)}>Begin local</button>
    <button type="button" onClick={() => void auth.selectLocalProfile('local-profile', '1234').catch(() => undefined)}>Open local</button>
    <button type="button" onClick={auth.cancelLocalProfileLogin}>Cancel local</button>
  </>;
}

function ReplacementHome() {
  const home = useHome();
  return <>
    <output aria-label="home-title">{home.status === 'success' ? home.data.rows[0]?.title ?? 'empty' : home.status}</output>
    <output aria-label="home-stale">{home.status === 'success' && home.stale ? 'stale' : 'fresh'}</output>
    <output aria-label="home-error">{home.status === 'success' ? home.error?.message ?? 'none' : 'none'}</output>
    <output aria-label="home-last-success">{home.status === 'success' && home.lastSuccessAt ? 'set' : 'none'}</output>
  </>;
}

function MediaDetailState() {
	const [reloadKey, setReloadKey] = useState(0);
	const detail = useMediaDetail('missing-media', reloadKey);
	return <>
		<output aria-label="media-detail-state">{detail.status}</output>
		<output aria-label="media-detail-result">{detail.status === 'success' ? detail.data.title : detail.status === 'error' ? detail.error.message : 'loading'}</output>
		<button type="button" onClick={() => setReloadKey((current) => current + 1)}>Retry media</button>
	</>;
}

function ScopedMediaStates() {
	const first = useMediaDetail('media-first');
	const second = useMediaDetail('media-second');
	const homeState = useHome();
	return <>
		<output aria-label="first-media">{first.status === 'success' ? first.data.title : first.status}</output>
		<output aria-label="second-media">{second.status === 'success' ? second.data.title : second.status}</output>
		<output aria-label="scoped-home">{homeState.status === 'success' ? homeState.data.rows[0]?.title : homeState.status}</output>
	</>;
}

function ReplacementProbe() {
  const auth = useAuthSession();
  return <>
    <output aria-label="profile">{auth.viewer?.viewerScope?.profileId ?? 'none'}</output>
    {auth.status === 'ready' && auth.viewer?.authenticated && auth.viewerScopeKey ? <ReplacementHome /> : <output aria-label="home-title">waiting</output>}
  </>;
}

function MutationCapture({ capture }: { capture: (run: () => Promise<Viewer>) => void }) {
  const source = usePorticoDataSource();
  useEffect(() => capture(() => source.updateProfile({ displayName: 'Owner', email: 'owner@example.test' }, new AbortController().signal)), [capture, source]);
  return <output aria-label="mutation-ready">ready</output>;
}

function home(title: string): HomeResult {
  return { pivots: [], rows: [{ id: title.toLowerCase().replaceAll(' ', '-'), title, type: 'media', items: [], hasMore: false }] };
}

describe('DataProvider viewer scope integration', () => {
	it('reuses the logical application subscription across a provider remount', async () => {
		const source = new LiveHomeSource(viewer('adult', 'policy-live'));
		const runtime = new WebViewerRuntime();
		const firstMount = render(<DataProvider source={source} viewerRuntime={runtime}><Probe /></DataProvider>);
		await waitFor(() => expect(source.watchCalls).toBe(1));
		firstMount.unmount();
		render(<DataProvider source={source} viewerRuntime={runtime}><Probe /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('ready'));
		expect(source.watchCalls).toBe(1);
	});

	it('quietly refetches matching live data without replacing successful content with a loading state', async () => {
		const active = viewer('adult', 'policy-live');
		const source = new LiveHomeSource(active);
		render(<DataProvider source={source}><ReplacementHome /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1'));
		const pending = deferred<HomeResult>();
		source.nextHome = pending.promise;
		act(() => source.publish(['playback-progress']));
		expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1');
		act(() => pending.resolve(home('Home 2')));
		await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 2'));
		expect(source.homeCalls).toBe(2);
	});

	it('scopes exact media metadata events to the matching detail query', async () => {
		class ScopedMediaSource extends LiveHomeSource {
			mediaCalls = new Map<string, number>();
			override async media(id: string) {
				const calls = (this.mediaCalls.get(id) ?? 0) + 1;
				this.mediaCalls.set(id, calls);
				return {
					id, title: `${id} ${calls}`, subtitle: '', year: 2026, entityKind: 'movie' as const,
					poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [],
				};
			}
		}
		const source = new ScopedMediaSource(viewer('adult', 'policy-live'));
		render(<DataProvider source={source}><ScopedMediaStates /></DataProvider>);
		await waitFor(() => {
			expect(screen.getByLabelText('first-media')).toHaveTextContent('media-first 1');
			expect(screen.getByLabelText('second-media')).toHaveTextContent('media-second 1');
			expect(screen.getByLabelText('scoped-home')).toHaveTextContent('Home 1');
		});

		act(() => source.publish(['media', 'metadata', 'library-items'], { resource: 'media', resourceId: 'media-first' }));
		await waitFor(() => expect(screen.getByLabelText('first-media')).toHaveTextContent('media-first 2'));
		expect(source.mediaCalls.get('media-second')).toBe(1);
		expect(source.homeCalls).toBe(1);
	});

	it('keeps broad catalog invalidations authoritative for all matching details', async () => {
		class ScopedMediaSource extends LiveHomeSource {
			mediaCalls = new Map<string, number>();
			override async media(id: string) {
				const calls = (this.mediaCalls.get(id) ?? 0) + 1;
				this.mediaCalls.set(id, calls);
				return {
					id, title: `${id} ${calls}`, subtitle: '', year: 2026, entityKind: 'movie' as const,
					poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [],
				};
			}
		}
		const source = new ScopedMediaSource(viewer('adult', 'policy-live'));
		render(<DataProvider source={source}><ScopedMediaStates /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('second-media')).toHaveTextContent('media-second 1'));

		act(() => source.publish(['media', 'library-items'], { resource: 'library', resourceId: 'library-movies' }));
		await waitFor(() => {
			expect(screen.getByLabelText('first-media')).toHaveTextContent('media-first 2');
			expect(screen.getByLabelText('second-media')).toHaveTextContent('media-second 2');
			expect(screen.getByLabelText('scoped-home')).toHaveTextContent('Home 2');
		});
	});

	it('defers live refetches while the browser tab is hidden and catches up when visible', async () => {
		const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
		const source = new LiveHomeSource(viewer('adult', 'policy-live'));
		render(<DataProvider source={source}><ReplacementHome /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1'));

		visibility.mockReturnValue('hidden');
		act(() => {
			document.dispatchEvent(new Event('visibilitychange'));
			source.publish(['home']);
		});
		await new Promise((resolve) => setTimeout(resolve, 150));
		expect(source.homeCalls).toBe(1);
		expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1');

		visibility.mockReturnValue('visible');
		act(() => document.dispatchEvent(new Event('visibilitychange')));
		await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 2'));
		expect(source.homeCalls).toBe(2);
	});

	it('propagates a Saved change into matching product data without blanking the current scope', async () => {
		const source = new LiveHomeSource(viewer('adult', 'policy-live'));
		render(<DataProvider source={source}><ReplacementHome /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1'));
		const pending = deferred<HomeResult>();
		source.nextHome = pending.promise;

		act(() => source.publish(['playlists']));
		expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1');
		expect(screen.getByLabelText('home-stale')).toHaveTextContent('fresh');

		act(() => pending.resolve(home('Saved propagated')));
		await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Saved propagated'));
		expect(source.homeCalls).toBe(2);
	});

	it('retains successful content when a background live refresh fails', async () => {
		const active = viewer('adult', 'policy-live');
		const source = new LiveHomeSource(active);
		render(<DataProvider source={source}><ReplacementHome /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1'));
		const pending = deferred<HomeResult>();
		source.nextHome = pending.promise;
		act(() => source.publish(['playback-progress']));
		expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1');
		await waitFor(() => expect(source.homeCalls).toBe(2));
		act(() => pending.reject(new Error('transient route failure')));
		await waitFor(() => {
			expect(screen.getByLabelText('home-title')).toHaveTextContent('Home 1');
			expect(screen.getByLabelText('home-stale')).toHaveTextContent('stale');
			expect(screen.getByLabelText('home-error')).toHaveTextContent('transient route failure');
			expect(screen.getByLabelText('home-last-success')).toHaveTextContent('set');
		});
	});

	it('keeps an authoritative not-found visible during background invalidation until refreshed data succeeds', async () => {
		const pending = deferred<MediaItem>();
		class MissingMediaSource extends LiveHomeSource {
			mediaCalls = 0;
			override async media() {
				this.mediaCalls += 1;
				if (this.mediaCalls === 1) throw new ApiError(404, 'media_not_found', 'Item not found');
				return pending.promise;
			}
		}
		const source = new MissingMediaSource(viewer('adult', 'policy-live'));
		render(<DataProvider source={source}><MediaDetailState /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('media-detail-state')).toHaveTextContent('error'));

		act(() => source.publish(['media']));
		await waitFor(() => expect(source.mediaCalls).toBe(2));
		expect(screen.getByLabelText('media-detail-state')).toHaveTextContent('error');
		expect(screen.getByLabelText('media-detail-result')).toHaveTextContent('Item not found');

		act(() => pending.resolve({
			id: 'missing-media', title: 'Restored media', subtitle: '', year: 2026, entityKind: 'movie',
			poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [],
		}));
		await waitFor(() => expect(screen.getByLabelText('media-detail-result')).toHaveTextContent('Restored media'));
	});

	it('reconciles the authoritative viewer after an identity-sensitive application event', async () => {
		const active = viewer('adult', 'policy-1');
		const source = new LiveHomeSource(active);
		const runtime = new WebViewerRuntime();
		render(<DataProvider source={source} viewerRuntime={runtime}><Probe /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));

		source.finalViewer = {
			...viewer('adult', 'policy-2'),
			user: {
				...viewer('adult', 'policy-2').user!,
				displayName: 'Adult updated',
				permissions: { playMedia: false },
			},
		};
		act(() => source.publish(['profiles']));

		await waitFor(() => expect(runtime.activeScope()?.authorizationRevision).toBe('policy-2'));
		expect(screen.getByLabelText('status')).toHaveTextContent('ready');
		expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
	});

	it('reconciles the authoritative viewer after a realtime reset without blanking product UI', async () => {
		const active = viewer('adult', 'policy-1');
		const source = new LiveHomeSource(active);
		const runtime = new WebViewerRuntime();
		render(<DataProvider source={source} viewerRuntime={runtime}><Probe /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));

		source.finalViewer = viewer('adult', 'policy-2');
		await act(async () => { await source.reset(); });

		await waitFor(() => expect(runtime.activeScope()?.authorizationRevision).toBe('policy-2'));
		expect(screen.getByLabelText('status')).toHaveTextContent('ready');
	});

  it('treats initialViewer as a hint and mounts no authenticated scope before fresh /me completes', async () => {
    const hint = viewer('stale-hint', 'old-policy');
    const fresh = viewer('adult', 'policy-2');
    const pending = deferred<Viewer>();
    const source = new SessionSource(fresh);
    source.viewerResult = pending.promise;
    render(<DataProvider source={source} initialViewer={hint}><Probe /></DataProvider>);
    expect(screen.getByLabelText('status')).toHaveTextContent('loading');
    expect(screen.getByLabelText('scope-key')).toBeEmptyDOMElement();
    act(() => pending.resolve(fresh));
    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('ready'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
    expect(screen.getByLabelText('scope-key')).not.toBeEmptyDOMElement();
  });

  it('does not let a bundled Local Auth tombstone block an unrelated Hosted-authority provider', async () => {
    expect(markLocalSessionSignedOut({
      authority: 'local', accountId: 'local-account', serverId: 'local-server', profileId: 'local-profile', authorizationRevision: 'policy-local',
    })).toBe(true);
    const hosted = viewer('adult', 'policy-hosted');
    const source = new SessionSource(hosted);
    render(<DataProvider source={source} initialViewer={hosted}><Probe /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('ready'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
    window.localStorage.clear();
  });

	it('keeps the verified viewer active when a Hosted profile replacement fails before publication', async () => {
		const adult = viewer('adult', 'policy-1');
		const source = new SessionSource(adult);
		source.hostedProfileSwitchError = new ApiError(429, 'rate_limited', 'Hosted profile request timed out.', undefined, {
			messageId: 'problem.rate-limited',
			requestId: 'profile-request-1',
			retryAfterMs: 5000,
		});
		const runtime = new WebViewerRuntime();
		render(<DataProvider source={source} viewerRuntime={runtime}><Probe /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));

		fireEvent.click(screen.getByRole('button', { name: 'Switch hosted profile' }));

		await waitFor(() => expect(screen.getByLabelText('identity-error')).toHaveTextContent(productMessage('problem.rate-limited').body!));
		expect(screen.getByLabelText('status')).toHaveTextContent('ready');
		expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
		expect(screen.getByLabelText('product-publication')).toHaveTextContent('one-household');
		expect(runtime.activeScope()?.profileId).toBe('adult');
		expect(screen.getByLabelText('identity-request-id')).toHaveTextContent('profile-request-1');
		expect(screen.getByLabelText('identity-retry-ms')).toHaveTextContent('5000');
	});

	it('keeps a verified bundled viewer active when a refresh fails transiently', async () => {
		const adult = viewer('adult', 'policy-1');
		const source = new SessionSource(adult);
		const runtime = new WebViewerRuntime();
		render(<DataProvider source={source} viewerRuntime={runtime}><Probe /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));

		source.viewerError = new TypeError('The server connection was interrupted.');
		fireEvent.click(screen.getByRole('button', { name: 'Refresh viewer' }));

		await waitFor(() => expect(screen.getByLabelText('identity-error')).toHaveTextContent(productMessage('problem.request-failed').body!));
		expect(screen.getByLabelText('status')).toHaveTextContent('ready');
		expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
		expect(screen.getByLabelText('product-publication')).toHaveTextContent('one-household');
		expect(runtime.activeScope()?.profileId).toBe('adult');
	});

	it('does not erase a verified viewer for an endpoint-generic 401 without an explicit revocation code', async () => {
		const adult = viewer('adult', 'policy-1');
		const source = new SessionSource(adult);
		const runtime = new WebViewerRuntime();
		render(<DataProvider source={source} viewerRuntime={runtime}><Probe /></DataProvider>);
		await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));

		source.viewerError = new ApiError(401, 'unauthorized', 'This endpoint could not authorize the request.');
		fireEvent.click(screen.getByRole('button', { name: 'Refresh viewer' }));

		await waitFor(() => expect(screen.getByLabelText('identity-error')).toHaveTextContent(productMessage('problem.request-failed').body!));
		expect(screen.getByLabelText('status')).toHaveTextContent('ready');
		expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
		expect(runtime.activeScope()?.profileId).toBe('adult');
	});

  it('blocks replacement activation and shows a fail-closed state when profile-switch teardown fails', async () => {
    const adult = viewer('adult', 'policy-1');
    const child = viewer('child', 'policy-2');
    const source = new SessionSource(adult);
    render(<DataProvider source={source}><Probe failingTeardown /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));
    source.finalViewer = child;
    fireEvent.click(screen.getByRole('button', { name: 'Switch' }));
    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('error'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('none');
    expect(screen.getByLabelText('product-publication')).toHaveTextContent('blocked');
  });

  it('still clears viewer UI and calls server logout when teardown reports a failure', async () => {
    const source = new SessionSource(viewer('adult', 'policy-1'));
    render(<DataProvider source={source}><Probe failingTeardown /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));
    fireEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('none'));
    expect(screen.getByLabelText('status')).toHaveTextContent('ready');
    expect(source.logoutCalled).toBe(true);
  });

  it('rejects a Hosted bundled cookie after restart when logout failed', async () => {
    class HostedLogoutFailureSource extends SessionSource {
      markerPrecededMutation = false;
      override async logout() {
        this.markerPrecededMutation = ambientCookieRestoreStatus().quarantined;
        this.logoutCalled = true;
        throw new TypeError('Hosted profile cookie logout failed.');
      }
    }
    const hosted = viewer('adult', 'policy-1');
    const source = new HostedLogoutFailureSource(hosted);
    const first = render(
      <DataProvider source={source} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}>
        <Probe />
      </DataProvider>,
    );
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));
    fireEvent.click(screen.getByRole('button', {name: 'Sign out'}));
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('none'));
    expect(source.markerPrecededMutation).toBe(true);
    expect(ambientCookieRestoreStatus()).toMatchObject({
      trustedForRestore: true,
      quarantined: true,
      marker: {mutationKind: 'logout', intent: {state: 'signed-out'}},
    });

    first.unmount();
    const restart = new SessionSource(hosted);
    const viewerRead = vi.spyOn(restart, 'viewer');
    render(
      <DataProvider source={restart} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}>
        <Probe />
      </DataProvider>,
    );
    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('ready'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('none');
    expect(screen.getByLabelText('identity-error')).toHaveTextContent(productMessage('auth.sign-out-storage-warning').body!);
    expect(viewerRead).not.toHaveBeenCalled();
  });

  it('retains restart quarantine when browser-account switch lands a cookie but final identity drifts', async () => {
    const initial = viewer('adult', 'policy-1');
    const candidate = viewer('second-profile', 'policy-2');
    candidate.viewerScope!.accountId = 'account-two';
    candidate.user!.id = 'account-two';
    const wrongFinal = viewer('attacker-profile', 'policy-attacker');
    wrongFinal.viewerScope!.accountId = 'attacker-account';
    wrongFinal.user!.id = 'attacker-account';
    class LateSwitchIdentityFailure extends SessionSource {
      switched = false;
      markerPrecededMutation = false;
      override async switchBrowserAccount(_accountId: string) {
        this.markerPrecededMutation = ambientCookieRestoreStatus().quarantined;
        this.switched = true;
        return structuredClone(candidate);
      }
      override async viewer(signal: AbortSignal) {
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        return structuredClone(this.switched ? wrongFinal : initial);
      }
    }
    const source = new LateSwitchIdentityFailure(initial);
    source.browserAccountState = {
      accounts: [
        {id: 'one-household', displayName: 'Adult', profileImageUrl: undefined, authOrigin: 'portico', authProvider: 'portico', lastUsedAt: '2026-07-14T10:00:00.000Z'},
        {id: 'account-two', displayName: 'Second', profileImageUrl: undefined, authOrigin: 'portico', authProvider: 'portico', lastUsedAt: '2026-07-14T11:00:00.000Z'},
      ],
      activeAccountId: 'one-household',
      automaticSignIn: false,
      selectionRequired: true,
      canAddAccount: true,
    };
    const first = render(
      <DataProvider source={source} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}>
        <Probe switchAccountId="account-two" />
      </DataProvider>,
    );
    await waitFor(() => expect(screen.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));
    fireEvent.click(screen.getByRole('button', {name: 'Switch'}));
    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('error'));
    expect(source.markerPrecededMutation).toBe(true);
    expect(ambientCookieRestoreStatus()).toMatchObject({
      quarantined: true,
      marker: {mutationKind: 'browser-account-switch'},
    });

    first.unmount();
    const restart = new SessionSource(candidate);
    const viewerRead = vi.spyOn(restart, 'viewer');
    render(<DataProvider source={restart} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><Probe /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('ready'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('none');
    expect(viewerRead).not.toHaveBeenCalled();
  });

  it('serializes two bundled contexts and rejects an older late Set-Cookie before the newer identity publishes', async () => {
    const initial = viewer('adult', 'policy-1');
    const olderCandidate = accountViewer('account-two', 'older-profile', 'policy-2');
    const winner = accountViewer('account-three', 'winner-profile', 'policy-3');
    const lateCookie = deferred<Viewer>();
    let olderMutationCalls = 0;
    let winnerMutationCalls = 0;
    class OlderContextSource extends SessionSource {
      override async switchBrowserAccount() {
        olderMutationCalls += 1;
        return structuredClone(await lateCookie.promise);
      }
    }
    class WinnerContextSource extends SessionSource {
      override async switchBrowserAccount() {
        winnerMutationCalls += 1;
        this.finalViewer = winner;
        return structuredClone(winner);
      }
    }
    const olderSource = new OlderContextSource(initial);
    const winnerSource = new WinnerContextSource(initial);
    selectableAccounts(olderSource, 'account-two');
    selectableAccounts(winnerSource, 'account-three');
    const older = render(<DataProvider source={olderSource} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><Probe switchAccountId="account-two" /></DataProvider>);
    const newer = render(<DataProvider source={winnerSource} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><Probe switchAccountId="account-three" /></DataProvider>);
    const olderUI = within(older.container);
    const newerUI = within(newer.container);
    await waitFor(() => expect(olderUI.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));
    await waitFor(() => expect(newerUI.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));

    fireEvent.click(olderUI.getByRole('button', {name: 'Switch'}));
    await waitFor(() => expect(olderMutationCalls).toBe(1));
    fireEvent.click(newerUI.getByRole('button', {name: 'Switch'}));
    await Promise.resolve();
    expect(ambientCookieRestoreStatus().quarantined).toBe(true);

    act(() => lateCookie.resolve(olderCandidate));
    await waitFor(() => expect(olderUI.getByLabelText('product-publication')).toHaveTextContent('blocked'));
    await waitFor(() => expect(newerUI.getByLabelText('product-publication')).toHaveTextContent('account-three'));
    expect(olderUI.getByLabelText('product-publication')).not.toHaveTextContent('account-two');
    expect(winnerMutationCalls).toBe(1);
    expect(ambientCookieRestoreStatus()).toEqual({trustedForRestore: true, quarantined: false});
  });

  it('does not let an older late final /me publish or clear the newer context quarantine', async () => {
    const initial = viewer('adult', 'policy-1');
    const olderCandidate = accountViewer('account-two', 'older-profile', 'policy-2');
    const winner = accountViewer('account-three', 'winner-profile', 'policy-3');
    const lateFinalViewer = deferred<Viewer>();
    const finalViewerEntered = deferred<void>();
    const winnerMutationEntered = deferred<void>();
    const releaseWinnerMutation = deferred<void>();
    let switched = false;
    const olderPublicationHistory: string[] = [];
    class OlderContextSource extends SessionSource {
      override async switchBrowserAccount() {
        switched = true;
        return structuredClone(olderCandidate);
      }
      override async viewer(signal: AbortSignal) {
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        if (!switched) return structuredClone(initial);
        finalViewerEntered.resolve();
        return structuredClone(await lateFinalViewer.promise);
      }
    }
    class WinnerContextSource extends SessionSource {
      override async switchBrowserAccount() {
        winnerMutationEntered.resolve();
        await releaseWinnerMutation.promise;
        this.finalViewer = winner;
        return structuredClone(winner);
      }
    }
    const olderSource = new OlderContextSource(initial);
    const winnerSource = new WinnerContextSource(initial);
    selectableAccounts(olderSource, 'account-two');
    selectableAccounts(winnerSource, 'account-three');
    const older = render(<DataProvider source={olderSource} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><Probe switchAccountId="account-two" onPublication={(accountId) => olderPublicationHistory.push(accountId)} /></DataProvider>);
    const newer = render(<DataProvider source={winnerSource} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><Probe switchAccountId="account-three" /></DataProvider>);
    const olderUI = within(older.container);
    const newerUI = within(newer.container);
    await waitFor(() => expect(olderUI.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));
    await waitFor(() => expect(newerUI.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));

    fireEvent.click(olderUI.getByRole('button', {name: 'Switch'}));
    await finalViewerEntered.promise;
    // Resolve final /me, then reserve B synchronously before A's promise
    // continuation can enter the no-await publication commit.
    await act(async () => {
      lateFinalViewer.resolve(olderCandidate);
      fireEvent.click(newerUI.getByRole('button', {name: 'Switch'}));
      await Promise.resolve();
    });
    expect(ambientCookieRestoreStatus().quarantined).toBe(true);

    await waitFor(() => expect(olderUI.getByLabelText('product-publication')).toHaveTextContent('blocked'));
    await winnerMutationEntered.promise;
    expect(ambientCookieRestoreStatus()).toMatchObject({
      quarantined: true,
      marker: {intent: {state: 'authenticated', expected: {accountId: 'account-three'}}},
    });
    act(() => releaseWinnerMutation.resolve());
    await waitFor(() => expect(newerUI.getByLabelText('product-publication')).toHaveTextContent('account-three'));
    expect(olderUI.getByLabelText('product-publication')).not.toHaveTextContent('account-two');
    expect(olderPublicationHistory).not.toContain('account-two');
    expect(ambientCookieRestoreStatus()).toEqual({trustedForRestore: true, quarantined: false});
  });

  it('fails closed before a bundled cookie mutation when the cross-context lock is unavailable', async () => {
    const initial = viewer('adult', 'policy-1');
    const candidate = accountViewer('account-two', 'other-profile', 'policy-2');
    const source = new SessionSource(initial);
    source.switchViewer = candidate;
    selectableAccounts(source, 'account-two');
    const mutation = vi.spyOn(source, 'switchBrowserAccount');
    const rendered = render(<DataProvider source={source} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><Probe switchAccountId="account-two" /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));
    const originalLocks = navigator.locks;
    Object.defineProperty(navigator, 'locks', {configurable: true, value: undefined});
    try {
      fireEvent.click(screen.getByRole('button', {name: 'Switch'}));
      await waitFor(() => expect(screen.getByLabelText('product-publication')).toHaveTextContent('blocked'));
      expect(mutation).not.toHaveBeenCalled();
      expect(ambientCookieRestoreStatus()).toMatchObject({
        quarantined: true,
        marker: {mutationKind: 'browser-account-switch'},
      });
    } finally {
      Object.defineProperty(navigator, 'locks', {configurable: true, value: originalLocks});
      rendered.unmount();
    }
  });

  it.each([
    ['active account removal', 'Remove active', 'browser-account-remove'],
    ['sign out all', 'Sign out all', 'browser-account-sign-out-all'],
  ])('retains a restart fence after %s', async (_label, actionName, mutationKind) => {
    const hosted = viewer('adult', 'policy-1');
    class DestructiveAccountSource extends SessionSource {
      markerPrecededMutation = false;
      override async removeBrowserAccount() {
        this.markerPrecededMutation = ambientCookieRestoreStatus().quarantined;
        this.finalViewer = {authenticated: false, setupRequired: false, serverName: 'Scope Test Server'};
        return {ok: true, activeAccountRemoved: true, vaultRevoked: true};
      }
      override async signOutAllBrowserAccounts() {
        this.markerPrecededMutation = ambientCookieRestoreStatus().quarantined;
        this.finalViewer = {authenticated: false, setupRequired: false, serverName: 'Scope Test Server'};
      }
    }
    const source = new DestructiveAccountSource(hosted);
    const first = render(
      <DataProvider source={source} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}>
        <Probe />
      </DataProvider>,
    );
    await waitFor(() => expect(screen.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));
    fireEvent.click(screen.getByRole('button', {name: actionName}));
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('none'));
    expect(source.markerPrecededMutation).toBe(true);
    expect(ambientCookieRestoreStatus()).toMatchObject({
      quarantined: true,
      marker: {mutationKind, intent: {state: 'signed-out'}},
    });

    first.unmount();
    const restart = new SessionSource(hosted);
    const viewerRead = vi.spyOn(restart, 'viewer');
    render(<DataProvider source={restart} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><Probe /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('ready'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('none');
    expect(viewerRead).not.toHaveBeenCalled();
  });

  it('fails closed when final /me returns a different account than the explicitly tapped browser account', async () => {
    const initial = viewer('adult', 'policy-1');
    const candidate = structuredClone(initial);
    candidate.viewerScope!.accountId = 'account-two';
    candidate.viewerScope!.profileId = 'profile-two';
    candidate.user!.id = 'account-two';
    const wrongFinal = structuredClone(candidate);
    wrongFinal.viewerScope!.accountId = 'attacker-account';
    wrongFinal.viewerScope!.profileId = 'attacker-profile';
    wrongFinal.user!.id = 'attacker-account';
    const source = new SessionSource(initial);
    source.switchViewer = candidate;
    source.browserAccountState = {
      accounts: [
        { id: 'one-household', displayName: 'Adult', profileImageUrl: undefined, authOrigin: 'portico', authProvider: 'portico', lastUsedAt: '2026-07-14T10:00:00.000Z' },
        { id: 'account-two', displayName: 'Second account', profileImageUrl: undefined, authOrigin: 'portico', authProvider: 'portico', lastUsedAt: '2026-07-14T11:00:00.000Z' },
      ],
      activeAccountId: 'one-household',
      automaticSignIn: false,
      selectionRequired: true,
      canAddAccount: true,
    };
    const runtime = new WebViewerRuntime();
    render(<DataProvider source={source} viewerRuntime={runtime}><Probe switchAccountId="account-two" /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('product-publication')).toHaveTextContent('one-household'));
    await waitFor(() => expect(screen.getByLabelText('browser-accounts-status')).toHaveTextContent('ready'));
    source.finalViewer = wrongFinal;

    fireEvent.click(screen.getByRole('button', { name: 'Switch' }));

    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('error'));
    expect(screen.getByLabelText('product-publication')).toHaveTextContent('blocked');
    expect(screen.queryByText('attacker-profile')).not.toBeInTheDocument();
    expect(screen.getByLabelText('identity-error')).toHaveTextContent(productMessage('problem.request-failed').body!);
    expect(screen.queryByText('different viewing identity')).not.toBeInTheDocument();
    expect(runtime.activeScope()).toBeUndefined();
  });

  it('keeps one generation fence across datasource replacement and rejects an old-source result', async () => {
    const runtime = new WebViewerRuntime();
    const oldHome = deferred<HomeResult>();
    const sourceA = new SessionSource(viewer('adult', 'policy-1'));
    vi.spyOn(sourceA, 'home').mockImplementation(async () => oldHome.promise);
    const sourceB = new SessionSource(viewer('child', 'policy-2'));
    vi.spyOn(sourceB, 'home').mockResolvedValue(home('Child home'));

    const view = render(<DataProvider source={sourceA} viewerRuntime={runtime}><ReplacementProbe /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));
    await waitFor(() => expect(sourceA.home).toHaveBeenCalled());

    view.rerender(<DataProvider source={sourceB} viewerRuntime={runtime}><ReplacementProbe /></DataProvider>);
    act(() => oldHome.resolve(home('Adult-only row')));

    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('child'));
    await waitFor(() => expect(screen.getByLabelText('home-title')).toHaveTextContent('Child home'));
    expect(screen.queryByText('Adult-only row')).not.toBeInTheDocument();
  });

  it.each<{ field: string; mutate: (scope: NonNullable<Viewer['viewerScope']>) => void }>([
    { field: 'authority', mutate: (scope) => { scope.authority = 'local'; } },
    { field: 'account', mutate: (scope) => { scope.accountId = 'other-account'; } },
    { field: 'server', mutate: (scope) => { scope.serverId = 'other-server'; } },
    { field: 'profile', mutate: (scope) => { scope.profileId = 'other-profile'; } },
  ])('rejects a final /me response with the wrong $field principal before product publication', async ({ mutate }) => {
    const expectedViewer = viewer('adult', 'policy-1');
    const finalViewer = structuredClone(expectedViewer);
    mutate(finalViewer.viewerScope!);
    const source = new SessionSource(finalViewer);
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(expectedViewer.viewerScope!);

    render(<DataProvider source={source} initialViewer={expectedViewer} expectedViewerScope={expectedViewer.viewerScope} viewerRuntime={runtime}><Probe /></DataProvider>);

    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('error'));
    expect(runtime.activeScope()).toBeUndefined();
  });

  it('accepts an authoritative /me authorization revision advance for the expected principal', async () => {
    const expectedViewer = viewer('adult', 'policy-1');
    const finalViewer = viewer('adult', 'policy-2');
    const source = new SessionSource(finalViewer);
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(expectedViewer.viewerScope!);

    render(<DataProvider source={source} initialViewer={expectedViewer} expectedViewerScope={expectedViewer.viewerScope} viewerRuntime={runtime}><Probe /></DataProvider>);

    await waitFor(() => expect(screen.getByLabelText('status')).toHaveTextContent('ready'));
    expect(runtime.activeScope()?.authorizationRevision).toBe('policy-2');
  });

  it('preserves the current verified session when Local Auth profile selection is opened and cancelled', async () => {
    const existing = viewer('adult', 'policy-1');
    const source = new LocalProfileSource(existing);
    const runtime = new WebViewerRuntime();
    render(<DataProvider source={source} viewerRuntime={runtime}><LocalProfileProbe /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));

    fireEvent.click(screen.getByRole('button', { name: 'Begin local' }));
    await waitFor(() => expect(screen.getByLabelText('local-challenge')).toHaveTextContent('local-account'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
    expect(runtime.activeScope()?.profileId).toBe('adult');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel local' }));
    expect(screen.getByLabelText('local-challenge')).toHaveTextContent('none');
    expect(screen.getByLabelText('profile')).toHaveTextContent('adult');
    expect(runtime.activeScope()?.profileId).toBe('adult');
  });

  it.each<[string, (selection: LocalProfileSelection) => void]>([
    ['expired grant', (selection: LocalProfileSelection) => { selection.grant.expiresAt = new Date(Date.now() - 1_000).toISOString(); }],
    ['identity-mismatched grant', (selection: LocalProfileSelection) => { selection.grant.profileId = 'different-profile'; }],
  ])('rejects a Local Auth %s while preserving an existing viewer and never publishing provisional credentials', async (_label, mutate) => {
    const existing = viewer('adult', 'policy-1');
    const source = new LocalProfileSource(existing);
    source.selection = {
      challenge: source.challenge,
      grant: {
        token: 'local-grant',
        authority: 'local',
        accountId: 'local-account',
        serverId: 'local-server',
        profileId: 'local-profile',
        pinRevision: 2,
        installationId: 'installation-local',
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
      },
    };
    mutate(source.selection);
    const runtime = new WebViewerRuntime();
    render(<DataProvider source={source} viewerRuntime={runtime}><LocalProfileProbe /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));
    fireEvent.click(screen.getByRole('button', { name: 'Begin local' }));
    await waitFor(() => expect(screen.getByLabelText('local-challenge')).toHaveTextContent('local-account'));
    fireEvent.click(screen.getByRole('button', { name: 'Open local' }));

    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('adult'));
    expect(source.publishCalls).toBe(0);
    expect(runtime.activeScope()?.profileId).toBe('adult');
  });

  it('serializes cookie publication and compensates a superseded late Local Auth completion before the newest choice publishes', async () => {
    window.localStorage.clear();
    const releaseFirstPublication = deferred<void>();
    const unauthenticated: Viewer = {authenticated: false, setupRequired: false, serverName: 'Scope Test Server'};
    class RacingLocalProfileSource extends LocalProfileSource {
      activeViewer: Viewer = unauthenticated;
      logoutCalls = 0;
      publicationOrder: string[] = [];

      override async viewer(signal: AbortSignal) {
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        return structuredClone(this.activeViewer);
      }

      override async publishLocalProfileSession(_selection: LocalProfileSelection, signal: AbortSignal) {
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        this.publishCalls += 1;
        const label = this.publishCalls === 1 ? 'A' : 'B';
        if (label === 'A') await releaseFirstPublication.promise;
        this.publicationOrder.push(label);
        this.activeViewer = localViewer('local-profile', `policy-${label}`);
        return structuredClone(this.activeViewer);
      }

      override async logout(_signal: AbortSignal) {
        this.logoutCalls += 1;
        this.activeViewer = unauthenticated;
      }
    }
    const source = new RacingLocalProfileSource(unauthenticated);
    source.selection = {
      challenge: source.challenge,
      grant: {
        token: 'local-grant', authority: 'local', accountId: 'local-account', serverId: 'local-server', profileId: 'local-profile',
        pinRevision: 2, installationId: 'installation-local', expiresAt: new Date(Date.now() + 60_000).toISOString(),
      },
    };

    render(<DataProvider source={source} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}><LocalProfileProbe /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('none'));
    fireEvent.click(screen.getByRole('button', { name: 'Begin local' }));
    await waitFor(() => expect(screen.getByLabelText('local-challenge')).toHaveTextContent('local-account'));
    fireEvent.click(screen.getByRole('button', { name: 'Open local' }));
    await waitFor(() => expect(source.publishCalls).toBe(1));

    fireEvent.click(screen.getByRole('button', { name: 'Open local' }));
    await Promise.resolve();
    expect(source.publishCalls).toBe(1);

    await act(async () => { releaseFirstPublication.resolve(); });
    await waitFor(() => expect(screen.getByLabelText('authorization-revision')).toHaveTextContent('policy-B'));
    expect(source.publicationOrder).toEqual(['A', 'B']);
    expect(source.logoutCalls).toBe(1);
    expect(source.activeViewer.viewerScope?.authorizationRevision).toBe('policy-B');
  });

  it('persists a restart quarantine when superseded cookie compensation cannot prove logout', async () => {
    const releaseFirstPublication = deferred<void>();
    const unauthenticated: Viewer = {authenticated: false, setupRequired: false, serverName: 'Scope Test Server'};
    class UncertainCookieSource extends LocalProfileSource {
      activeViewer: Viewer = unauthenticated;

      override async viewer(signal: AbortSignal) {
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        return structuredClone(this.activeViewer);
      }

      override async publishLocalProfileSession(_selection: LocalProfileSelection, signal: AbortSignal) {
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        this.publishCalls += 1;
        if (this.publishCalls === 1) await releaseFirstPublication.promise;
        this.activeViewer = localViewer('local-profile', `policy-${this.publishCalls}`);
        return structuredClone(this.activeViewer);
      }

      override async logout() {
        throw new TypeError('Logout transport failed before cookie removal could be verified.');
      }
    }
    const source = new UncertainCookieSource(unauthenticated);
    source.selection = {
      challenge: source.challenge,
      grant: {
        token: 'local-grant', authority: 'local', accountId: 'local-account', serverId: 'local-server', profileId: 'local-profile',
        pinRevision: 2, installationId: 'installation-local', expiresAt: new Date(Date.now() + 60_000).toISOString(),
      },
    };

    const firstRender = render(
      <DataProvider source={source} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}>
        <LocalProfileProbe />
      </DataProvider>,
    );
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('none'));
    fireEvent.click(screen.getByRole('button', {name: 'Begin local'}));
    await waitFor(() => expect(screen.getByLabelText('local-challenge')).toHaveTextContent('local-account'));
    fireEvent.click(screen.getByRole('button', {name: 'Open local'}));
    await waitFor(() => expect(source.publishCalls).toBe(1));
    fireEvent.click(screen.getByRole('button', {name: 'Open local'}));
    await act(async () => { releaseFirstPublication.resolve(); });

    await waitFor(() => expect(screen.getByLabelText('local-error')).toHaveTextContent(productMessage('auth.sign-out-storage-warning').body!));
    expect(ambientCookieRestoreStatus()).toMatchObject({
      trustedForRestore: true,
      quarantined: true,
      marker: {mutationKind: 'logout', intent: {state: 'signed-out'}},
    });

    firstRender.unmount();
    const restartSource = new LocalProfileSource(localViewer('local-profile', 'stale-cookie-policy'));
    const restartViewer = vi.spyOn(restartSource, 'viewer');
    render(
      <DataProvider source={restartSource} localSessionQuarantineEnabled viewerRuntime={new WebViewerRuntime()}>
        <LocalProfileProbe />
      </DataProvider>,
    );
    await waitFor(() => expect(screen.getByLabelText('local-status')).toHaveTextContent('ready'));
    expect(screen.getByLabelText('profile')).toHaveTextContent('none');
    expect(screen.getByLabelText('local-error')).toHaveTextContent(productMessage('auth.sign-out-storage-warning').body!);
    expect(restartViewer).not.toHaveBeenCalled();
  });

  it('stays fail-closed with the canonical storage warning when an uncertain cookie mutation cannot publish its durable quarantine', async () => {
    const unauthenticated: Viewer = {authenticated: false, setupRequired: false, serverName: 'Scope Test Server'};
    class UnwritableQuarantineSource extends LocalProfileSource {
      activeViewer: Viewer = unauthenticated;

      override async viewer() {
        return structuredClone(this.activeViewer);
      }

      override async publishLocalProfileSession(_selection: LocalProfileSelection, _signal: AbortSignal): Promise<Viewer> {
        this.publishCalls += 1;
        this.activeViewer = localViewer('local-profile', 'cookie-may-have-landed');
        throw new TypeError('The response ended after its Set-Cookie boundary.');
      }

      override async logout() {
        this.activeViewer = unauthenticated;
      }
    }
    const source = new UnwritableQuarantineSource(unauthenticated);
    source.selection = {
      challenge: source.challenge,
      grant: {
        token: 'local-grant', authority: 'local', accountId: 'local-account', serverId: 'local-server', profileId: 'local-profile',
        pinRevision: 2, installationId: 'installation-local', expiresAt: new Date(Date.now() + 60_000).toISOString(),
      },
    };
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('Storage is unavailable.', 'QuotaExceededError');
    });
    const runtime = new WebViewerRuntime();
    render(
      <DataProvider source={source} localSessionQuarantineEnabled viewerRuntime={runtime}>
        <LocalProfileProbe />
      </DataProvider>,
    );
    await waitFor(() => expect(screen.getByLabelText('profile')).toHaveTextContent('none'));
    fireEvent.click(screen.getByRole('button', {name: 'Begin local'}));
    await waitFor(() => expect(screen.getByLabelText('local-challenge')).toHaveTextContent('local-account'));
    fireEvent.click(screen.getByRole('button', {name: 'Open local'}));

    await waitFor(() => expect(screen.getByLabelText('local-error')).toHaveTextContent(productMessage('auth.sign-out-storage-warning').body!));
    expect(runtime.activeScope()).toBeUndefined();
    expect(screen.getByLabelText('profile')).toHaveTextContent('none');
  });

  it('rebuilds the generation-bound datasource after failed A to B and lets the restored A mutation identify only A', async () => {
    const expectedA = viewer('adult', 'policy-1');
    const candidateB = { ...expectedA.viewerScope!, serverId: 'server-two', profileId: 'child', authorizationRevision: 'policy-2' };
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(expectedA.viewerScope!);
    const sourceA = new SessionSource(expectedA);
    const mutationScopes: string[] = [];
    vi.spyOn(sourceA, 'updateProfile').mockImplementation(async () => {
      mutationScopes.push(runtime.activeScope()?.serverId ?? 'none');
      return expectedA;
    });
    let capturedMutation: (() => Promise<Viewer>) | undefined;
    const capture = (run: () => Promise<Viewer>) => { capturedMutation = run; };
    render(<DataProvider source={sourceA} initialViewer={expectedA} expectedViewerScope={expectedA.viewerScope} viewerRuntime={runtime}><MutationCapture capture={capture} /></DataProvider>);
    await waitFor(() => expect(screen.getByLabelText('mutation-ready')).toHaveTextContent('ready'));
    await waitFor(() => expect(capturedMutation).toBeDefined());
    const staleAAction = capturedMutation!;

    let staged: Awaited<ReturnType<WebViewerRuntime['stage']>>;
    await act(async () => { staged = await runtime.stage(candidateB); });
    await expect(staleAAction()).rejects.toMatchObject({ name: 'AbortError' });
    expect(sourceA.updateProfile).not.toHaveBeenCalled();
    act(() => {
      staged!.fenceRollback('restore-previous');
      staged!.rollback('restore-previous');
    });
    await waitFor(() => expect(screen.getByLabelText('mutation-ready')).toHaveTextContent('ready'));
    await waitFor(() => expect(capturedMutation).not.toBe(staleAAction));

    await expect(capturedMutation!()).resolves.toEqual(expectedA);
    expect(mutationScopes).toEqual(['server-one']);
  });
});
