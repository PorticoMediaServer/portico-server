import { describe, expect, it, vi } from 'vitest';
import { registerHostedServiceWorker } from './hostedServiceWorker';

describe('Hosted Web service worker registration', () => {
	it('registers only for Hosted Web and leaves bundled server UI alone', async () => {
		const register = vi.fn().mockResolvedValue({ scope: 'https://web.getportico.tv/' });
		await expect(registerHostedServiceWorker('hosted', { register }, undefined, { buildId: 'release-42' })).resolves.toEqual({ scope: 'https://web.getportico.tv/' });
		expect(register).toHaveBeenCalledWith('/portico-service-worker.js?build=release-42', { scope: '/', updateViaCache: 'none' });

    register.mockClear();
    const unregister = vi.fn().mockResolvedValue(true);
		const getRegistrations = vi.fn().mockResolvedValue([
			{ unregister, scope: 'https://web.getportico.tv/', active: { scriptURL: 'https://web.getportico.tv/portico-service-worker.js' } },
			{ unregister: vi.fn(), scope: 'https://web.getportico.tv/', active: { scriptURL: 'https://web.getportico.tv/unrelated-worker.js' } },
		]);
    const keys = vi.fn().mockResolvedValue(['portico-hosted-shell-v1', 'portico-hosted-assets-v2', 'unrelated-cache']);
    const remove = vi.fn().mockResolvedValue(true);
    await expect(registerHostedServiceWorker('bundled', { register, getRegistrations } as never, { keys, delete: remove })).resolves.toBeUndefined();
    expect(register).not.toHaveBeenCalled();
		expect(unregister).toHaveBeenCalledOnce();
    expect(remove).toHaveBeenCalledTimes(2);
    expect(remove).not.toHaveBeenCalledWith('unrelated-cache');
	});

	it('preserves the active worker when a versioned registration fails', async () => {
		const register = vi.fn().mockRejectedValue(new DOMException('Blocked', 'QuotaExceededError'));
		const getRegistrations = vi.fn();
		const deleteCache = vi.fn();

		await expect(registerHostedServiceWorker('hosted', { register, getRegistrations }, { keys: vi.fn(), delete: deleteCache }, { buildId: 'release-43' })).resolves.toBeUndefined();
		expect(register).toHaveBeenCalledWith('/portico-service-worker.js?build=release-43', { scope: '/', updateViaCache: 'none' });
		expect(getRegistrations).not.toHaveBeenCalled();
		expect(deleteCache).not.toHaveBeenCalled();
	});

	it('continues bounded cleanup when one hosted cache rejects, including quota errors', async () => {
		const unregister = vi.fn().mockResolvedValue(true);
		const getRegistrations = vi.fn().mockResolvedValue([
			{ unregister, scope: '/', active: { scriptURL: 'https://web.getportico.tv/portico-service-worker.js' } },
		]);
		const remove = vi.fn()
			.mockRejectedValueOnce(new DOMException('Storage full', 'QuotaExceededError'))
			.mockResolvedValueOnce(true);

		await expect(registerHostedServiceWorker('bundled', { register: vi.fn(), getRegistrations }, {
			keys: vi.fn().mockResolvedValue(['portico-hosted-shell-old', 'portico-hosted-assets-old']),
			delete: remove,
		})).resolves.toBeUndefined();
		expect(unregister).toHaveBeenCalledOnce();
		expect(remove).toHaveBeenCalledTimes(2);
	});

  it('does not prevent startup when browser storage policy rejects workers', async () => {
    const register = vi.fn().mockRejectedValue(new DOMException('Blocked', 'SecurityError'));
    await expect(registerHostedServiceWorker('hosted', { register }, undefined)).resolves.toBeUndefined();
  });
});
