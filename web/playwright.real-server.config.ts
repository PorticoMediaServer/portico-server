import { defineConfig } from '@playwright/test';
import { criticalRealServerProjects } from './playwright.config';

// The opt-in real-server runner deliberately has no webServer fixture. It
// must exercise the URL supplied by the caller, not silently fall back to the
// development fixture server.
export default defineConfig({
  testDir: './tests/e2e',
  testMatch: /real-server-critical\.spec\.ts/,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: [['list']],
  use: {
    colorScheme: 'dark',
    reducedMotion: 'reduce',
  },
  projects: [...criticalRealServerProjects],
});
