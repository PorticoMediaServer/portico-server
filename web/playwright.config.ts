import { defineConfig, devices } from '@playwright/test';

const baseURL = 'http://127.0.0.1:19115';
const realServerBaseURL = process.env.PORTICO_REAL_SERVER_URL?.trim().replace(/\/+$/, '') || baseURL;
const fixtureOnlyTestIgnore = /(?:viewport-matrix|playback-conformance|real-server-critical)\.spec\.ts/;
const criticalViewport = { width: 1440, height: 1000 };

// These projects intentionally cover only the opt-in critical real-server
// suite. The wider fixture suite remains Chromium-backed and keeps its
// existing viewport breadth; see playwright.real-server.config.ts for the
// external-only runner that avoids starting the fixture web server.
export const criticalRealServerProjects = [
  {
    name: 'critical-chromium',
    testMatch: /real-server-critical\.spec\.ts/,
    use: {
      ...devices['Desktop Chrome'],
      baseURL: realServerBaseURL,
      viewport: criticalViewport,
      reducedMotion: 'reduce' as const,
    },
  },
  {
    name: 'critical-webkit',
    testMatch: /real-server-critical\.spec\.ts/,
    use: {
      ...devices['Desktop Safari'],
      baseURL: realServerBaseURL,
      viewport: criticalViewport,
      reducedMotion: 'reduce' as const,
    },
  },
  {
    name: 'critical-firefox',
    testMatch: /real-server-critical\.spec\.ts/,
    use: {
      ...devices['Desktop Firefox'],
      baseURL: realServerBaseURL,
      viewport: criticalViewport,
      reducedMotion: 'reduce' as const,
    },
  },
] as const;

export default defineConfig({
  testDir: './tests/e2e',
  testIgnore: /playback-conformance\.spec\.ts/,
  timeout: 30_000,
  expect: { timeout: 8_000 },
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: [['list']],
  use: {
    baseURL,
    colorScheme: 'dark',
    reducedMotion: 'reduce',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    video: 'retain-on-failure',
  },
  webServer: {
    command: 'VITE_PORTICO_RUNTIME_MODE=fixtures npm run dev -- --port 19115',
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
  projects: [
    {
      name: 'desktop',
      testIgnore: fixtureOnlyTestIgnore,
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 1000 } },
    },
    {
      name: 'short-laptop',
      testIgnore: fixtureOnlyTestIgnore,
      use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 720 } },
    },
    {
      name: 'phone',
      testIgnore: fixtureOnlyTestIgnore,
      use: { ...devices['Pixel 7'], viewport: { width: 390, height: 844 } },
    },
    ...[
      ['phone-320', 320, 720, true],
      ['phone-360', 360, 780, true],
      ['phone-390', 390, 844, true],
      ['phone-430', 430, 932, true],
      ['phone-landscape', 844, 390, true],
      ['tablet-portrait', 768, 1024, true],
      ['tablet-landscape', 1024, 768, true],
      ['viewport-desktop', 1440, 1000, false],
    ].map(([name, width, height, touch]) => ({
      name: String(name),
      testMatch: /viewport-matrix\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: Number(width), height: Number(height) },
        hasTouch: Boolean(touch),
        isMobile: false,
        reducedMotion: 'reduce' as const,
      },
    })),
    ...criticalRealServerProjects,
  ],
});
