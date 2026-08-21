import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.PORTICO_PLAYBACK_WEB_URL ?? 'http://127.0.0.1:19125';
const playbackTest = /playback-conformance\.spec\.ts/;

export default defineConfig({
	testDir: './tests/e2e',
	timeout: 90_000,
	expect: { timeout: 15_000 },
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	// WEB-017: keep the real-server playback gate bounded to the two browser
	// engines whose source-adapter behavior the conformance spec verifies.
	projects: [
		{
			name: 'chromium',
			testMatch: playbackTest,
			use: {
				...devices['Desktop Chrome'],
				baseURL,
				trace: 'retain-on-failure',
			},
		},
		{
			name: 'webkit',
			testMatch: playbackTest,
			use: {
				// Playwright's WebKit runtime is a browser-engine signal only; this
				// project does not stand in for physical Safari evidence.
				...devices['Desktop Safari'],
				baseURL,
				trace: 'retain-on-failure',
			},
		},
	],
});
