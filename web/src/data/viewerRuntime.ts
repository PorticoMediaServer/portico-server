import {
	normalizeViewerScope,
	sameViewerScope,
	transitionViewerRuntime,
	ViewerSyncCoordinator,
	type ProfileTransitionReason,
	type ViewerRuntimeAdapter,
	type ViewerScope,
	type ViewerSyncLifecycleEvent,
	viewerCacheKey,
} from '@porticomediaserver/client-core';

type RuntimeTaskKind = 'query' | 'mutation';
type RuntimeTeardownKind = 'query' | 'playback' | 'realtime' | 'artwork' | 'overlay' | 'focus' | 'profile-local';

type RuntimeTask = {
	controller: AbortController;
	generation: number;
	kind: RuntimeTaskKind;
	key: string;
	settled: Promise<void>;
};

type RuntimeAuthorizationGate = <T>(scope: ViewerScope, start: () => Promise<T>) => Promise<T>;

export type WebViewerRuntimeRollbackMode = 'restore-previous' | 'fail-closed';

export type StagedWebViewerRuntime = {
	publish(): void;
	fenceRollback(mode?: WebViewerRuntimeRollbackMode): void;
	rollback(mode?: WebViewerRuntimeRollbackMode): void;
};

function abortError(message = 'The previous viewing profile is no longer active.') {
	return new DOMException(message, 'AbortError');
}

function isAbortSignal(value: unknown): value is AbortSignal {
	return typeof AbortSignal !== 'undefined' && value instanceof AbortSignal;
}

function cacheParameter(value: unknown, ancestors = new WeakSet<object>()): unknown {
	if (value === null || typeof value === 'string' || typeof value === 'boolean') return value;
	if (typeof value === 'number') return Number.isFinite(value) ? value : String(value);
	if (value === undefined) return null;
	if (typeof value !== 'object') return String(value);
	if (isAbortSignal(value)) return null;
	if (value instanceof Blob) return { blob: true, size: value.size, type: value.type, name: value instanceof File ? value.name : undefined };
	if (value instanceof Date) return value.toISOString();
	if (ancestors.has(value)) return '[cycle]';
	ancestors.add(value);
	try {
		if (Array.isArray(value)) return value.map((item) => cacheParameter(item, ancestors));
		return Object.fromEntries(Object.entries(value as Record<string, unknown>)
			.map(([key, item]) => [key, cacheParameter(item, ancestors)])
			.filter(([, item]) => item !== undefined));
	} finally {
		ancestors.delete(value);
	}
}

function combineSignals(left: AbortSignal | undefined, right: AbortSignal): AbortSignal {
	if (!left) return right;
	if (typeof AbortSignal.any === 'function') return AbortSignal.any([left, right]);
	const controller = new AbortController();
	const abort = () => controller.abort();
	if (left.aborted || right.aborted) controller.abort();
	else {
		left.addEventListener('abort', abort, { once: true });
		right.addEventListener('abort', abort, { once: true });
	}
	return controller.signal;
}

const mutationPrefixes = [
	'add', 'apply', 'create', 'delete', 'fetch', 'handoff', 'join', 'leave', 'login', 'logout',
	'mutate', 'prepare', 'queue', 'remove', 'reorder', 'renew', 'set', 'setup', 'signOut', 'start',
	'stop', 'switch', 'touch', 'update', 'upload',
];

function operationKind(name: string): RuntimeTaskKind {
	return mutationPrefixes.some((prefix) => name.startsWith(prefix)) ? 'mutation' : 'query';
}

/**
 * Owns the browser generation fence. No result from an earlier profile may
 * resolve into React state after a transition starts, even when its producer
 * ignores AbortSignal.
 */
export class WebViewerRuntime {
	private scope?: ViewerScope;
	private generation = 0;
	private transitioning = false;
	private readonly tasks = new Set<RuntimeTask>();
	private readonly activationWaiters = new Set<(scope: ViewerScope) => void>();
	private readonly teardowns = new Map<RuntimeTeardownKind, Set<() => Promise<void> | void>>();
	private readonly generationListeners = new Set<() => void>();
	private stagedTransition?: symbol;
	private authorizationGate?: RuntimeAuthorizationGate;
	private viewerSyncCoordinator?: ViewerSyncCoordinator;
	private syncLifecycleHandler?: (event: ViewerSyncLifecycleEvent) => void;

	activeScope() {
		return this.scope;
	}

	isTransitioning() {
		return this.transitioning;
	}

	currentGeneration() {
		return this.generation;
	}

	subscribeGeneration(listener: () => void) {
		this.generationListeners.add(listener);
		return () => this.generationListeners.delete(listener);
	}

	setAuthorizationGate(gate: RuntimeAuthorizationGate | undefined) {
		this.authorizationGate = gate;
	}

	setSyncLifecycleHandler(handler: ((event: ViewerSyncLifecycleEvent) => void) | undefined) {
		this.syncLifecycleHandler = handler;
	}

	viewerSync() {
		return this.viewerSyncCoordinator;
	}

	setRuntimeState(state: { foreground?: boolean; online?: boolean }) {
		this.viewerSyncCoordinator?.setRuntimeState(state);
	}

	setPlaybackContinuityActive(active: boolean) {
		this.viewerSyncCoordinator?.setPlaybackContinuityActive(active);
	}

	private activateViewerSync() {
		if (!this.scope || this.transitioning) return;
		this.viewerSyncCoordinator?.close();
		const generation = this.generation;
		this.viewerSyncCoordinator = new ViewerSyncCoordinator({
			generationFence: { generation, currentGeneration: () => this.generation },
			onLifecycleEvent: (event) => this.syncLifecycleHandler?.(event),
		});
	}

	private closeViewerSync() {
		this.viewerSyncCoordinator?.close();
		this.viewerSyncCoordinator = undefined;
	}

	private advanceGeneration() {
		this.closeViewerSync();
		this.generation += 1;
		for (const listener of this.generationListeners) listener();
	}

	assertGeneration(expectedGeneration: number) {
		if (expectedGeneration !== this.generation) throw abortError();
	}

	/** Installs an immediate write fence while an account-level teardown is prepared. */
	fence() {
		if (this.transitioning) return;
		this.transitioning = true;
		this.advanceGeneration();
		for (const task of this.tasks) task.controller.abort();
	}

	activeScopeKey() {
		const scope = this.scope;
		return scope ? viewerCacheKey({ ...scope, contractRevision: 'web-v1', resource: 'app-root' }) : '';
	}

	activateInitial(scope: ViewerScope) {
		if (this.scope || this.transitioning) throw new Error('A viewing profile is already active.');
		this.scope = normalizeViewerScope(scope);
		this.transitioning = false;
		this.activateViewerSync();
		this.resolveActivationWaiters(this.scope);
	}

	register(kind: RuntimeTeardownKind, teardown: () => Promise<void> | void) {
		const callbacks = this.teardowns.get(kind) ?? new Set();
		callbacks.add(teardown);
		this.teardowns.set(kind, callbacks);
		return () => callbacks.delete(teardown);
	}

	async run<T>(name: string, args: unknown[], operation: (signal: AbortSignal) => Promise<T>, expectedGeneration = this.generation): Promise<T> {
		this.assertGeneration(expectedGeneration);
		// Components may mount while the mandatory fresh /api/auth/me request is
		// still running. Queue their work behind that boundary rather than either
		// executing it unscoped or leaving the feature stuck after a rejected
		// pre-auth request. The component's own signal can still abandon the wait.
		const scope = this.scope && !this.transitioning
			? this.scope
			: await this.awaitActiveScope(args.find(isAbortSignal));
		this.assertGeneration(expectedGeneration);
		const controller = new AbortController();
		const generation = this.generation;
		const parameters = cacheParameter(args) as unknown[];
		const key = viewerCacheKey({
			...scope,
			contractRevision: 'web-v1',
			resource: name,
			parameters: { arguments: parameters },
		});
		let settle: () => void = () => {};
		const settled = new Promise<void>((resolve) => { settle = resolve; });
		const task: RuntimeTask = { controller, generation, kind: operationKind(name), key, settled };
		this.tasks.add(task);
		try {
			const start = () => {
				// The authorization gate may be queued behind a cross-tab publication
				// lock. Re-assert ownership at the exact producer boundary so a task
				// fenced while queued cannot start merely because its producer ignores
				// an already-aborted signal.
				if (controller.signal.aborted || generation !== this.generation || this.transitioning || this.scope !== scope) {
					throw abortError();
				}
				return operation(controller.signal);
			};
			const result = await (this.authorizationGate
				? this.authorizationGate(scope, start)
				: start());
			if (controller.signal.aborted || generation !== this.generation || this.transitioning || this.scope !== scope) throw abortError();
			return result;
		} finally {
			this.tasks.delete(task);
			settle();
		}
	}

	private awaitActiveScope(callerSignal: AbortSignal | undefined): Promise<ViewerScope> {
		if (this.scope && !this.transitioning) return Promise.resolve(this.scope);
		// Waiting is permitted only during the first mandatory identity bootstrap.
		// Once a generation fence has advanced, signed-out, revoked, or frozen old
		// UI must fail closed rather than queue work for a future identity.
		if (this.generation > 0 || this.transitioning) return Promise.reject(abortError('No verified viewing profile is active.'));
		if (callerSignal?.aborted) return Promise.reject(abortError());
		return new Promise<ViewerScope>((resolve, reject) => {
			const activate = (scope: ViewerScope) => {
				callerSignal?.removeEventListener('abort', abort);
				resolve(scope);
			};
			const abort = () => {
				this.activationWaiters.delete(activate);
				reject(abortError());
			};
			this.activationWaiters.add(activate);
			callerSignal?.addEventListener('abort', abort, { once: true });
		});
	}

	private resolveActivationWaiters(scope: ViewerScope) {
		const waiters = [...this.activationWaiters];
		this.activationWaiters.clear();
		for (const waiter of waiters) waiter(scope);
	}

	async transition(to: ViewerScope | undefined, reason: ProfileTransitionReason) {
		if (this.stagedTransition) throw new Error('A server connection is already being staged.');
		const from = this.scope;
		if (!from) {
			if (to) this.activateInitial(to);
			return;
		}
		try {
			await transitionViewerRuntime(this.adapter(), from, to, reason);
		} finally {
			// Sign-out and revocation are security boundaries, not profile swaps.
			// They must never leave an authenticated surface active merely because
			// a producer reported a cleanup failure after the generation fence.
			if (!to || reason === 'sign-out' || reason === 'profile-revoked') {
				this.scope = undefined;
				this.transitioning = false;
			}
		}
	}

	/**
	 * Drains and freezes the active source without publishing the candidate
	 * scope. Core may now publish credentials durably; old source bindings are
	 * fenced by the advanced generation until this transaction is resolved.
	 */
	async stage(to: ViewerScope, reason: ProfileTransitionReason = 'server-switch'): Promise<StagedWebViewerRuntime> {
		if (this.stagedTransition) throw new Error('A server connection is already being staged.');
		const next = normalizeViewerScope(to);
		const previous = this.scope;
		const token = Symbol('staged-viewer-runtime');
		this.stagedTransition = token;
		try {
			if (previous) {
				await transitionViewerRuntime({ ...this.adapter(), activateProfile: undefined }, previous, next, reason);
			} else {
				this.transitioning = true;
				this.advanceGeneration();
			}
		} catch (reason) {
			// A failed teardown cannot safely restore the old product surface.
			this.scope = undefined;
			this.transitioning = false;
			this.stagedTransition = undefined;
			throw reason;
		}
		const transactionGeneration = this.generation;

		let published = false;
		let publishedGeneration: number | undefined;
		let rolledBack = false;
		let rollbackFenced = false;
		let rollbackMode: WebViewerRuntimeRollbackMode = 'restore-previous';
		return {
			publish: () => {
				if (rolledBack || rollbackFenced || published) return;
				if (this.stagedTransition !== token) throw new Error('The staged server connection is no longer current.');
				if (this.generation !== transactionGeneration) throw new Error('The staged server connection generation changed before publication.');
				published = true;
				this.stagedTransition = undefined;
				this.scope = next;
				this.transitioning = false;
				this.advanceGeneration();
				publishedGeneration = this.generation;
				this.activateViewerSync();
				this.resolveActivationWaiters(next);
			},
			fenceRollback: (mode = 'restore-previous') => {
				if (rolledBack) {
					if (mode === 'fail-closed') this.failClosed();
					return;
				}
				if (rollbackFenced) {
					if (mode === 'fail-closed') rollbackMode = 'fail-closed';
					return;
				}
				if (published) {
					if (this.generation !== publishedGeneration || !this.scope || !sameViewerScope(this.scope, next)) {
						throw new Error('The published server connection is no longer current.');
					}
				} else if (this.stagedTransition !== token || this.generation !== transactionGeneration) {
					throw new Error('The staged server connection is no longer current.');
				}
				rollbackFenced = true;
				rollbackMode = mode;
				// Retract B synchronously. React may commit the corresponding visual
				// removal on its next render, but every existing binding is invalidated
				// before Client Core can begin restoring A's credentials.
				this.scope = undefined;
				this.transitioning = true;
				this.advanceGeneration();
				for (const task of this.tasks) task.controller.abort();
			},
			rollback: (mode = 'restore-previous') => {
				if (rolledBack) return;
				if (!rollbackFenced) throw new Error('Rollback completion requires a synchronous runtime fence.');
				if (mode === 'fail-closed') rollbackMode = 'fail-closed';
				const restored = rollbackMode === 'restore-previous' ? previous : undefined;
				rolledBack = true;
				this.stagedTransition = undefined;
				this.scope = restored ? normalizeViewerScope(restored) : undefined;
				this.transitioning = false;
				this.advanceGeneration();
				if (this.scope) this.activateViewerSync();
				if (this.scope) this.resolveActivationWaiters(this.scope);
			},
		};
	}

	/** Last-resort synchronous fence used only when transactional rollback fails. */
	failClosed() {
		this.advanceGeneration();
		for (const task of this.tasks) task.controller.abort();
		this.scope = undefined;
		this.transitioning = false;
		this.stagedTransition = undefined;
	}

	private async invoke(kind: RuntimeTeardownKind) {
		const results = await Promise.allSettled([...(this.teardowns.get(kind) ?? [])].map((callback) => callback()));
		const failure = results.find((result): result is PromiseRejectedResult => result.status === 'rejected');
		if (failure) throw failure.reason;
	}

	private adapter(): ViewerRuntimeAdapter {
		return {
			beginTransition: () => {
				this.transitioning = true;
				this.advanceGeneration();
			},
			cancelRequests: async () => {
				const tasks = [...this.tasks];
				for (const task of tasks) task.controller.abort();
				// Query results are generation-fenced and may be abandoned when a
				// producer ignores AbortSignal. Mutations remain serialized because
				// their side effects must settle before credential publication.
				await Promise.all(tasks.filter((task) => task.kind === 'mutation').map((task) => task.settled));
			},
			stopPlayback: () => this.invoke('playback'),
			closeRealtime: () => this.invoke('realtime'),
			clearOptimisticMutations: async () => {
				const mutations = [...this.tasks].filter((task) => task.kind === 'mutation');
				await Promise.all(mutations.map((task) => task.settled));
			},
			clearQueryCaches: () => this.invoke('query'),
			clearArtworkState: () => this.invoke('artwork'),
			closeOverlays: () => this.invoke('overlay'),
			clearFocusRestoration: () => this.invoke('focus'),
			clearProfileLocalState: () => this.invoke('profile-local'),
			activateProfile: (scope) => {
				this.scope = normalizeViewerScope(scope);
				this.transitioning = false;
				this.activateViewerSync();
				this.resolveActivationWaiters(this.scope);
			},
		};
	}
}

const unscopedMethods = new Set([
	'authCapabilities', 'browserAccounts', 'login', 'logout', 'porticoClient', 'removeBrowserAccount',
	'setup', 'signOutAllBrowserAccounts', 'startPorticoSetup', 'switchBrowserAccount',
	'updateBrowserAccountPreferences', 'viewer',
]);

const transitionTeardownMethods = new Set(['stopPlayback']);
const synchronousFactoryMethods = new Set(['settingsDataSource', 'watchWithFriendsSource']);
const optionalSignalPositions: Record<string, number> = { stopPlayback: 1, touchPlayback: 2 };
function scopedPromiseClient<T extends object>(client: T, runtime: WebViewerRuntime, bindingGeneration: number): T {
	const proxy = new Proxy(client, {
		get(target, property, receiver) {
			const value = Reflect.get(target, property, receiver);
			if (typeof property !== 'string' || typeof value !== 'function') return value;
			if (property.endsWith('Url')) return (...args: unknown[]) => {
				runtime.assertGeneration(bindingGeneration);
				return value.apply(target, args);
			};
			return (...args: unknown[]) => runtime.run(`client.${property}`, args, (runtimeSignal) => {
				const nextArgs = args.map((argument) => {
					if (!argument || typeof argument !== 'object' || Array.isArray(argument) || !('signal' in argument)) return argument;
					const options = argument as Record<string, unknown>;
					return { ...options, signal: combineSignals(isAbortSignal(options.signal) ? options.signal : undefined, runtimeSignal) };
				});
				return Promise.resolve(value.apply(target, nextArgs));
			}, bindingGeneration);
		},
	});
	return proxy;
}

/** Wraps all authenticated datasource promises, including feature-local calls. */
export function scopedDataSource<T extends object>(source: T, runtime: WebViewerRuntime): T {
	const bindingGeneration = runtime.currentGeneration();
	return new Proxy(source, {
		get(target, property, receiver) {
			const value = Reflect.get(target, property, receiver);
			if (typeof property !== 'string' || typeof value !== 'function') return value;
			if (property === 'playbackResourceUrl') return (...args: unknown[]) => {
				runtime.assertGeneration(bindingGeneration);
				return value.apply(target, args);
			};
			if (property === 'porticoClient') return () => {
				runtime.assertGeneration(bindingGeneration);
				return scopedPromiseClient(value.call(target), runtime, bindingGeneration);
			};
			if (synchronousFactoryMethods.has(property)) return (...args: unknown[]) => {
				runtime.assertGeneration(bindingGeneration);
				return value.apply(target, args);
			};
			return (...args: unknown[]) => {
				if (transitionTeardownMethods.has(property) && runtime.isTransitioning()) return value.apply(target, args);
				if (unscopedMethods.has(property)) {
					runtime.assertGeneration(bindingGeneration);
					return value.apply(target, args);
				}
				const discoveredSignalIndex = args.findIndex(isAbortSignal);
				const signalIndex = optionalSignalPositions[property] ?? discoveredSignalIndex;
				const callerSignal = signalIndex >= 0 && isAbortSignal(args[signalIndex]) ? args[signalIndex] as AbortSignal : undefined;
				return runtime.run(property, args, (runtimeSignal) => {
					const nextArgs = [...args];
					const signal = combineSignals(callerSignal, runtimeSignal);
					if (signalIndex >= 0) nextArgs[signalIndex] = signal;
					else nextArgs.push(signal);
					return Promise.resolve(value.apply(target, nextArgs));
				}, bindingGeneration);
			};
		},
	});
}
