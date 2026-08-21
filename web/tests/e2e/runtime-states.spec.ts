import { expect, test, type Page, type Route } from '@playwright/test';
import { PORTICO_FOUNDATION_COMPATIBILITY } from '@portico/client-core';

const json = (route: Route, body: unknown, status = 200) => route.fulfill({
  status,
  contentType: 'application/json',
  body: JSON.stringify(body),
});

async function useRuntime(page: Page, mode: 'bundled' | 'hosted') {
  await page.addInitScript((runtimeMode) => {
    window.__PORTICO_CONFIG__ = {
      mode: runtimeMode,
      hostedApiBaseUrl: 'https://api.getportico.tv',
      routeProbeTimeoutMs: 500,
    };
  }, mode);
  const compatibility = {
    ...PORTICO_FOUNDATION_COMPATIBILITY,
    requiredSemantics: Object.keys(PORTICO_FOUNDATION_COMPATIBILITY.semanticRevisions),
    capabilities: [{ id: 'system', revision: 1, state: 'available', requiredSemantics: ['product'] }],
  };
  await page.route(mode === 'hosted' ? 'https://api.getportico.tv/api/system' : '**/api/system', (route) => json(route, {
    name: 'Portico', status: 'ok', apiVersion: 'v1', compatibility,
  }));
}

function desktopOnly(testInfo: { project: { name: string } }) {
  test.skip(testInfo.project.name !== 'desktop', 'Runtime state matrix runs once at the desktop breakpoint');
}

test('local first-run setup offers Hosted and local-only ownership honestly', async ({ page }, testInfo) => {
  desktopOnly(testInfo);
  await useRuntime(page, 'bundled');
  await page.route('**/api/auth/capabilities', (route) => json(route, {
    setupRequired: true,
    localCredentialsEnabled: true,
    porticoAccountAuthEnabled: true,
    serverFriendlyName: 'Workshop Server',
    publicUserPickerEnabled: false,
    visibleUsers: [],
    generatedAt: new Date().toISOString(),
  }));
  await page.route('**/api/auth/me', (route) => json(route, {
    authenticated: false,
    setupRequired: true,
    serverFriendlyName: 'Workshop Server',
  }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Set Up Your Portico Server' })).toBeVisible();
  await expect(page.getByRole('button', { name: /Portico Account/ })).toBeVisible();
  await page.getByRole('button', { name: /Server Authentication Only/ }).click();
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Password', exact: true })).toHaveAttribute('minlength', '8');
  await expect(page.getByText(/do not create a Portico Account/)).toBeVisible();
});

test('local sign-in exposes only server-declared methods and retains useful errors', async ({ page }, testInfo) => {
  desktopOnly(testInfo);
  await useRuntime(page, 'bundled');
  await page.route('**/api/auth/capabilities', (route) => json(route, {
    setupRequired: false,
    localCredentialsEnabled: true,
    porticoAccountAuthEnabled: true,
    serverFriendlyName: 'Workshop Server',
    publicUserPickerEnabled: false,
    visibleUsers: [],
    generatedAt: new Date().toISOString(),
  }));
  await page.route('**/api/auth/me', (route) => json(route, {
    authenticated: false,
    setupRequired: false,
    serverFriendlyName: 'Workshop Server',
  }));
  await page.route('**/api/auth/browser-accounts', (route) => json(route, {
    accounts: [],
    automaticSignIn: true,
    selectionRequired: false,
    canAddAccount: true,
  }));
  await page.route('**/api/auth/profile-authentications/local', (route) => json(route, {
    type: 'https://getportico.tv/problems/invalid-credentials',
    code: 'invalid_credentials',
    detail: 'The username or password was not accepted.',
  }, 401));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign in to Workshop Server' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign in with This Server' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Continue with Portico Account' })).toBeVisible();
  await page.getByLabel('Username or email').fill('owner');
  await page.getByRole('textbox', { name: 'Password', exact: true }).fill('incorrect password');
  await page.getByRole('button', { name: 'Sign in with This Server' }).click();
  await expect(page.getByRole('alert')).toHaveText('Check your username or email and password, then try again.');
  await expect(page.getByLabel('Username or email')).toHaveValue('owner');
});

test('local bootstrap failure presents a focused recovery action', async ({ page }, testInfo) => {
  desktopOnly(testInfo);
  await useRuntime(page, 'bundled');
  await page.route('**/api/auth/capabilities', (route) => route.abort('failed'));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Server unavailable' })).toBeFocused();
  await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
  await expect(page.getByText(/couldn't reach this server/)).toBeVisible();
});

test('Hosted sign-in reveals MFA only after the account requests it', async ({ page }, testInfo) => {
  desktopOnly(testInfo);
  await useRuntime(page, 'hosted');
  await page.route('https://api.getportico.tv/api/auth/me', (route) => json(route, { authenticated: false }));
  await page.route('https://api.getportico.tv/api/auth/login', (route) => json(route, {
    type: 'https://getportico.tv/problems/mfa-required',
    code: 'mfa_required',
    detail: 'Enter a verification code to continue.',
  }, 401));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign in to Portico' })).toBeVisible();
  await expect(page.getByLabel('Verification code')).toHaveCount(0);
  await page.getByLabel('Username or email').fill('owner@example.com');
  await page.getByRole('textbox', { name: 'Password', exact: true }).fill('correct horse battery staple');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByLabel('Verification code')).toBeVisible();
  await page.getByRole('button', { name: 'Use a recovery code' }).click();
  await expect(page.getByLabel('Recovery code')).toBeVisible();
});

test('Hosted sign-in stays centered and contained on a phone', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone', 'Mobile containment is checked at the phone breakpoint');
  await useRuntime(page, 'hosted');
  await page.route('https://api.getportico.tv/api/auth/me', (route) => json(route, { authenticated: false }));

  await page.goto('/');
  const heading = page.getByRole('heading', { name: 'Sign in to Portico' });
  await expect(heading).toBeVisible();
  await expect(heading).toBeFocused();

  const geometry = await page.evaluate(() => {
    const panel = document.querySelector<HTMLElement>('.auth-panel');
    const bounds = panel?.getBoundingClientRect();
    return {
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
      documentWidth: document.documentElement.scrollWidth,
      documentHeight: document.documentElement.scrollHeight,
      panelLeft: bounds?.left ?? -1,
      panelRight: bounds?.right ?? Number.POSITIVE_INFINITY,
      outlineStyle: getComputedStyle(document.querySelector<HTMLElement>('.runtime-route-heading')!).outlineStyle,
    };
  });

  expect(geometry.documentWidth).toBeLessThanOrEqual(geometry.viewportWidth);
  expect(geometry.documentHeight).toBeLessThanOrEqual(geometry.viewportHeight + 1);
  expect(geometry.panelLeft).toBeGreaterThanOrEqual(0);
  expect(geometry.panelRight).toBeLessThanOrEqual(geometry.viewportWidth);
  expect(geometry.outlineStyle).toBe('none');
});

test('Hosted account without memberships offers claim and invitation recovery', async ({ page }, testInfo) => {
  desktopOnly(testInfo);
  await useRuntime(page, 'hosted');
  await page.route('https://api.getportico.tv/api/auth/me', (route) => json(route, {
    authenticated: true,
    user: { id: 'account-1', email: 'owner@example.com', displayName: 'Owner' },
  }));
  await page.route('https://api.getportico.tv/api/account/servers*', (route) => json(route, {
    items: [],
    total: 0,
    limit: 50,
    hasMore: false,
  }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'No servers yet' })).toBeVisible();
  await expect(page.getByLabel('Server claim code')).toBeVisible();
  await page.getByRole('tab', { name: 'Accept an invite' }).click();
  await expect(page.getByLabel('Invitation code')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Refresh servers' })).toBeVisible();
});
