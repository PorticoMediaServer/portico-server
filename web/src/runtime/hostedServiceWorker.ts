import type { RuntimeMode } from './runtimeMachine';

type ServiceWorkerRegistrationLike = {
	unregister: () => Promise<boolean> | boolean;
	scope?: string;
	active?: { scriptURL?: string } | null;
	waiting?: { scriptURL?: string } | null;
	installing?: { scriptURL?: string } | null;
};
type ServiceWorkerContainerLike = {
	register: ServiceWorkerContainer['register'];
	getRegistrations?: () => Promise<ReadonlyArray<ServiceWorkerRegistrationLike>>;
};
type CacheStorageLike = Pick<CacheStorage, 'keys' | 'delete'>;

export type HostedServiceWorkerOptions = {
	/** A deployment identifier used to force an update when the script URL is cached. */
	buildId?: string;
};

const PORTICO_HOSTED_CACHE_PREFIXES = ['portico-hosted-shell-', 'portico-hosted-assets-'];

function validBuildId(value: unknown): value is string {
	return typeof value === 'string' && /^[A-Za-z0-9._-]{1,128}$/.test(value);
}

function configuredBuildId(): string | undefined {
	const config = (globalThis as typeof globalThis & { __PORTICO_CONFIG__?: { buildId?: unknown } }).__PORTICO_CONFIG__;
	return validBuildId(config?.buildId) ? config.buildId : undefined;
}

function isPorticoRegistration(registration: ServiceWorkerRegistrationLike): boolean {
	const scriptURL = registration.active?.scriptURL ?? registration.waiting?.scriptURL ?? registration.installing?.scriptURL;
	if (!scriptURL) return false;
	try {
		const script = new URL(scriptURL, globalThis.location?.origin ?? 'https://portico.invalid');
		if (script.pathname !== '/portico-service-worker.js') return false;
		if (!registration.scope) return true;
		return new URL(registration.scope, script.origin).pathname === '/';
	} catch {
		return false;
	}
}

async function cleanupBundledWorkerState(
	container: ServiceWorkerContainerLike | undefined,
	cacheStorage: CacheStorageLike | undefined,
) {
	try {
		const registrations = await container?.getRegistrations?.() ?? [];
		await Promise.allSettled(registrations.filter(isPorticoRegistration).map((registration) => registration.unregister()));
	} catch {
		// Registration cleanup is best-effort. A browser policy failure must not
		// block the bundled application from starting.
	}
	try {
		const cacheNames = await cacheStorage?.keys() ?? [];
		await Promise.allSettled(cacheNames
			.filter((name) => PORTICO_HOSTED_CACHE_PREFIXES.some((prefix) => name.startsWith(prefix)))
			.map((name) => cacheStorage!.delete(name)));
	} catch {
		// Quota and private-browsing policies may reject cache enumeration/deletes.
	}
}

/**
 * Hosted Web caches immutable, content-addressed assets after they are fetched.
 * Navigation HTML, API responses, and media remain network-only so authenticated
 * state and viewer-specific content can never be replayed from a shared cache.
 */
export async function registerHostedServiceWorker(
	mode: RuntimeMode | undefined,
	container: ServiceWorkerContainerLike | undefined = globalThis.navigator?.serviceWorker,
	cacheStorage: CacheStorageLike | undefined = globalThis.caches,
	options: HostedServiceWorkerOptions = {},
): Promise<ServiceWorkerRegistration | undefined> {
	if (mode !== 'hosted') {
		await cleanupBundledWorkerState(container, cacheStorage);
		return undefined;
	}
	if (!container) return undefined;
	try {
		const buildId = validBuildId(options.buildId) ? options.buildId : configuredBuildId();
		const scriptUrl = buildId ? `/portico-service-worker.js?build=${encodeURIComponent(buildId)}` : '/portico-service-worker.js';
		return await container.register(scriptUrl, { scope: '/', updateViaCache: 'none' });
	} catch {
		// Keep the previously active worker and its caches intact when an update
		// fails. Private browsing and hardened storage policies may disable workers;
		// the live Hosted Web application remains usable without offline caching.
		return undefined;
	}
}
