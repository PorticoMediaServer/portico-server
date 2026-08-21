import { expect, test } from '@playwright/test';

type RealServerCredentials = {
  baseURL: string;
  username: string;
  password: string;
};

const rawBaseURL = process.env.PORTICO_REAL_SERVER_URL?.trim();
const username = process.env.PORTICO_REAL_SERVER_USERNAME?.trim();
const password = process.env.PORTICO_REAL_SERVER_PASSWORD;
const realServerRequested = Boolean(rawBaseURL || username || password);

function resolveRealServerCredentials(): RealServerCredentials | undefined {
  const missing = [
    !rawBaseURL && 'PORTICO_REAL_SERVER_URL',
    !username && 'PORTICO_REAL_SERVER_USERNAME',
    !password && 'PORTICO_REAL_SERVER_PASSWORD',
  ].filter((name): name is string => Boolean(name));

  if (realServerRequested && missing.length > 0) {
    throw new Error(`Real-server E2E was requested but is missing ${missing.join(', ')}. Set all three variables; no credentials are stored in the repository.`);
  }
  if (!rawBaseURL || !username || !password) return undefined;

  let parsed: URL;
  try {
    parsed = new URL(rawBaseURL);
  } catch {
    throw new Error('PORTICO_REAL_SERVER_URL must be an absolute http(s) URL for the server-hosted Portico Web application.');
  }
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new Error('PORTICO_REAL_SERVER_URL must use http or https.');
  }
  if (parsed.username || parsed.password) {
    throw new Error('PORTICO_REAL_SERVER_URL must not contain embedded credentials; pass the dedicated username/password variables instead.');
  }

  return {
    baseURL: rawBaseURL.replace(/\/+$/, ''),
    username,
    password,
  };
}

const realServer = resolveRealServerCredentials();
const unavailableReason = 'Real-server E2E is opt-in. Set PORTICO_REAL_SERVER_URL, PORTICO_REAL_SERVER_USERNAME, and PORTICO_REAL_SERVER_PASSWORD for a disposable test server; fixture coverage remains available by default.';

test('real server authenticates, exposes server data, and opens a library', async ({ page }) => {
  if (!realServer) test.skip(true, unavailableReason);
  test.setTimeout(60_000);

  const documentResponse = await page.goto('/', { waitUntil: 'domcontentloaded' });
  expect(documentResponse?.ok(), 'the configured real-server URL must serve the Portico Web application').toBeTruthy();
  expect(new URL(page.url()).origin).toBe(new URL(realServer!.baseURL).origin);

  await expect(page.getByRole('heading', { name: /^Sign in to .+/ })).toBeVisible();
  await page.getByRole('textbox', { name: 'Username or email' }).fill(realServer!.username);
  await page.getByRole('textbox', { name: 'Password', exact: true }).fill(realServer!.password);
  await page.getByRole('button', { name: 'Sign in with This Server' }).click();

  await expect(page.locator('#main-content')).toBeVisible();
  await expect(page.getByRole('button', { name: /Open profile menu/ })).toBeVisible();

  const session = await page.context().request.get('/api/auth/me');
  expect(session.ok(), `authenticated session check returned HTTP ${session.status()}`).toBeTruthy();
  const sessionBody = await session.json() as { authenticated?: unknown };
  expect(sessionBody.authenticated).toBe(true);

  await page.getByRole('link', { name: 'Libraries', exact: true }).first().click();
  await expect(page).toHaveURL(/\/libraries$/);
  await expect(page.getByRole('heading', { name: 'Libraries', exact: true })).toBeVisible();

  const directory = page.getByRole('region', { name: 'Available libraries' });
  await expect(directory).toBeVisible();
  const firstLibrary = directory.getByRole('link').first();
  const libraryName = (await firstLibrary.locator('strong').innerText()).trim();
  await firstLibrary.click();
  await expect(page).toHaveURL(/\/library\/[^/?#]+$/);
  await expect(page.getByRole('heading', { name: libraryName, exact: true })).toBeVisible();

  const dimensions = await page.evaluate(() => ({
    viewportWidth: document.documentElement.clientWidth,
    documentWidth: document.documentElement.scrollWidth,
  }));
  expect(dimensions.documentWidth, 'critical real-server routes must not introduce document overflow').toBeLessThanOrEqual(dimensions.viewportWidth);
});
