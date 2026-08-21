import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

const playbackProxyTarget = (globalThis as typeof globalThis & {
	process?: { env?: Record<string, string | undefined> };
}).process?.env?.PORTICO_PLAYBACK_FIXTURE_URL;
if (playbackProxyTarget) {
	const target = new URL(playbackProxyTarget);
	if (target.protocol !== 'http:' || !['127.0.0.1', '::1', '[::1]'].includes(target.hostname) || target.username || target.password) {
		throw new Error('PORTICO_PLAYBACK_FIXTURE_URL must be an uncredentialed loopback HTTP URL.');
	}
}

export default defineConfig({
  plugins: [react()],
  server: {
		host: '127.0.0.1', port: 19105,
		...(playbackProxyTarget ? { proxy: { '/api': { target: playbackProxyTarget }, '/__portico_playback_fixture': { target: playbackProxyTarget } } } : {}),
	},
  preview: { host: '127.0.0.1', port: 19105 },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('/node_modules/')) return undefined;
          if (id.includes('/hls.js/')) return 'hls';
          // React adapters must initialize with React. Keeping TanStack's React
          // package in the generic vendor chunk creates a vendor <-> React
          // cycle that can execute createContext before React is initialized.
          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/react-router') || id.includes('/@tanstack/')) return 'react-vendor';
          return 'vendor';
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    css: true,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
});
