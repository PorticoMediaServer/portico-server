import { ViewerRuntimeTeardownError, type ViewerScope } from '@portico/client-core';
import { describe, expect, it, vi } from 'vitest';
import { scopedDataSource, WebViewerRuntime } from './viewerRuntime';

const first: ViewerScope = {
  authority: 'hosted',
  accountId: 'household-one',
  serverId: 'server-one',
  profileId: 'profile-adult',
  authorizationRevision: 'policy-1',
};

const child: ViewerScope = {
  ...first,
  profileId: 'profile-child',
  authorizationRevision: 'policy-2',
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('WebViewerRuntime', () => {
	it('owns exactly one sync coordinator per verified viewer generation and fences it synchronously', async () => {
		const runtime = new WebViewerRuntime();
		runtime.activateInitial(first);
		const original = runtime.viewerSync();
		expect(original).toBeDefined();
		let streamSignal: AbortSignal | undefined;
		original!.leaseSubscription({
			key: 'application',
			start: (signal) => {
				streamSignal = signal as AbortSignal;
				return new Promise<void>((resolve) => signal.addEventListener('abort', resolve, { once: true }));
			},
		});
		await Promise.resolve();
		const transition = runtime.transition(child, 'profile-switch');
		expect(streamSignal?.aborted).toBe(true);
		expect(original?.isCurrent).toBe(false);
		await transition;
		expect(runtime.viewerSync()).toBeDefined();
		expect(runtime.viewerSync()).not.toBe(original);
		expect(runtime.viewerSync()?.isCurrent).toBe(true);
	});

	it('performs one authoritative reconcile after foreground continuity resumes', async () => {
		const runtime = new WebViewerRuntime();
		runtime.activateInitial(first);
		const refresh = vi.fn();
		runtime.viewerSync()!.registerResource({ key: 'cache', tags: ['*'], refresh });
		runtime.setRuntimeState({ foreground: false, online: true });
		runtime.setRuntimeState({ foreground: true, online: true });
		runtime.setRuntimeState({ foreground: true, online: true });
		await Promise.resolve();
		expect(refresh).toHaveBeenCalledOnce();
		expect([...refresh.mock.calls[0][0].tags]).toContain('runtime:reconcile');
	});

	it('turns terminal synchronization authorization into one lifecycle signal without retrying', async () => {
		const runtime = new WebViewerRuntime();
		const lifecycle = vi.fn();
		runtime.setSyncLifecycleHandler(lifecycle);
		runtime.activateInitial(first);
		const start = vi.fn(async () => { throw Object.assign(new Error('revoked'), { status: 403, code: 'viewer_access_revoked' }); });
		runtime.viewerSync()!.leaseSubscription({ key: 'application', start });
		await Promise.resolve();
		await Promise.resolve();
		expect(start).toHaveBeenCalledOnce();
		expect(lifecycle).toHaveBeenCalledOnce();
		expect(lifecycle.mock.calls[0][0]).toMatchObject({ reason: 'forbidden', status: 403 });
	});

  it('keeps synchronous development source factories synchronous', () => {
    const runtime = new WebViewerRuntime();
    const settings = { settings: vi.fn() };
    const watch = { connect: vi.fn() };
    const source = scopedDataSource({
      settingsDataSource: () => settings,
      watchWithFriendsSource: () => watch,
    }, runtime);

    expect(source.settingsDataSource()).toBe(settings);
    expect(source.watchWithFriendsSource()).toBe(watch);
  });

  it('fences and drains delayed query and mutation work before activating another profile on the same account', async () => {
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(first);
    const oldRootKey = runtime.activeScopeKey();
    const query = deferred<string>();
    const mutation = deferred<string>();
    const querySignal = vi.fn();
    const mutationSignal = vi.fn();
    const oldQuery = runtime.run('home', [], async (signal) => {
      signal.addEventListener('abort', querySignal, { once: true });
      return query.promise;
    });
    const oldMutation = runtime.run('setWatchlist', ['media-one', true], async (signal) => {
      signal.addEventListener('abort', mutationSignal, { once: true });
      return mutation.promise;
    });

    let transitionSettled = false;
    const transition = runtime.transition(child, 'profile-switch').then(() => { transitionSettled = true; });
    await Promise.resolve();
    expect(querySignal).toHaveBeenCalledOnce();
    expect(mutationSignal).toHaveBeenCalledOnce();
    expect(transitionSettled).toBe(false);

    query.resolve('adult-home');
    mutation.resolve('adult-write');
    await expect(oldQuery).rejects.toMatchObject({ name: 'AbortError' });
    await expect(oldMutation).rejects.toMatchObject({ name: 'AbortError' });
    await transition;
    expect(runtime.activeScope()).toEqual(child);
    expect(runtime.activeScopeKey()).not.toBe(oldRootKey);
    await expect(runtime.run('home', [], async () => 'child-home')).resolves.toBe('child-home');
  });

  it('awaits realtime, artwork, overlay, focus, and local cleanup on sign-out and revocation', async () => {
    for (const reason of ['sign-out', 'profile-revoked'] as const) {
      const runtime = new WebViewerRuntime();
      runtime.activateInitial(first);
      const calls: string[] = [];
      runtime.register('playback', async () => { calls.push('playback'); });
      runtime.register('realtime', async () => { calls.push('realtime'); });
      runtime.register('artwork', async () => { calls.push('artwork'); });
      runtime.register('overlay', async () => { calls.push('overlay'); });
      runtime.register('focus', async () => { calls.push('focus'); });
      runtime.register('profile-local', async () => { calls.push('profile-local'); });
      await runtime.transition(undefined, reason);
      expect(new Set(calls)).toEqual(new Set(['playback', 'realtime', 'artwork', 'overlay', 'focus', 'profile-local']));
      expect(runtime.activeScope()).toBeUndefined();
      await expect(runtime.run('home', [], async () => 'stale')).rejects.toMatchObject({ name: 'AbortError' });
    }
  });

  it('fails closed and never activates the next profile when teardown fails', async () => {
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(first);
    runtime.register('realtime', async () => { throw new Error('stream refused to close'); });
    await expect(runtime.transition(child, 'profile-switch')).rejects.toBeInstanceOf(ViewerRuntimeTeardownError);
    expect(runtime.activeScope()).toEqual(first);
    expect(runtime.isTransitioning()).toBe(true);
    await expect(runtime.run('home', [], async () => 'unsafe')).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('clears the authenticated scope on sign-out even when a teardown hook fails', async () => {
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(first);
    runtime.register('playback', async () => { throw new Error('receiver did not acknowledge close'); });
    await expect(runtime.transition(undefined, 'sign-out')).rejects.toBeInstanceOf(ViewerRuntimeTeardownError);
    expect(runtime.activeScope()).toBeUndefined();
    expect(runtime.isTransitioning()).toBe(false);
    await expect(runtime.run('home', [], async () => 'unsafe')).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('freezes old actions during candidate durability and restores A through a fresh generation after B fails', async () => {
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(first);
    const transport = vi.fn(async () => runtime.activeScope()?.serverId);
    const source = { setWatchlist: transport };
    const oldA = scopedDataSource(source, runtime);

    const staged = await runtime.stage({ ...child, serverId: 'server-two' });
    await expect(oldA.setWatchlist()).rejects.toMatchObject({ name: 'AbortError' });
    expect(transport).not.toHaveBeenCalled();

    staged.fenceRollback('restore-previous');
    staged.rollback('restore-previous');
    expect(runtime.activeScope()).toEqual(first);
    await expect(oldA.setWatchlist()).rejects.toMatchObject({ name: 'AbortError' });
    const restoredA = scopedDataSource(source, runtime);
    await expect(restoredA.setWatchlist()).resolves.toBe('server-one');
    expect(transport).toHaveBeenCalledOnce();
  });

  it('does not let an abort-ignoring query block a newer staged selection', async () => {
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(first);
    const query = deferred<string>();
    const oldQuery = runtime.run('home', [], async () => query.promise);

    const next = { ...child, serverId: 'server-two' };
    const staged = await runtime.stage(next);
    staged.publish();
    expect(runtime.activeScope()).toEqual(next);

    query.resolve('late adult result');
    await expect(oldQuery).rejects.toMatchObject({name: 'AbortError'});
    await expect(runtime.run('home', [], async () => 'new result')).resolves.toBe('new result');
  });

  it('publishes a staged scope only after the transaction commits and can fail closed after publication', async () => {
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(first);
    const next = { ...child, serverId: 'server-two' };
    const staged = await runtime.stage(next);
    expect(runtime.activeScope()).toEqual(first);
    expect(runtime.isTransitioning()).toBe(true);
    staged.publish();
    expect(runtime.activeScope()).toEqual(next);
    staged.fenceRollback('fail-closed');
    staged.rollback('fail-closed');
    expect(runtime.activeScope()).toBeUndefined();
  });

  it('keeps both candidate and previous runtime fenced until credential compensation explicitly completes', async () => {
    const runtime = new WebViewerRuntime();
    runtime.activateInitial(first);
    const next = { ...child, serverId: 'server-two' };
    const staged = await runtime.stage(next);
    staged.publish();
    expect(runtime.activeScope()).toEqual(next);

    staged.fenceRollback('restore-previous');
    expect(runtime.activeScope()).toBeUndefined();
    expect(runtime.isTransitioning()).toBe(true);
    await expect(runtime.run('home', [], async () => 'unsafe')).rejects.toMatchObject({name: 'AbortError'});

    // Represents an arbitrarily delayed credential/durable-record restoration.
    await Promise.resolve();
    expect(runtime.activeScope()).toBeUndefined();
    expect(runtime.isTransitioning()).toBe(true);

    staged.rollback('restore-previous');
    expect(runtime.activeScope()).toEqual(first);
    expect(runtime.isTransitioning()).toBe(false);
  });
});
