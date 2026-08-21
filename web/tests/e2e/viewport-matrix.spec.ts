import { expect, test, type Locator, type Page } from '@playwright/test';

type Surface = { name: string; path: string; ready: (page: Page) => Locator };

const surfaces: Surface[] = [
  { name: 'Home', path: '/home', ready: (page) => page.getByRole('heading', { name: 'Fargo' }) },
  { name: 'catalog grid', path: '/library/fixture-tv', ready: (page) => page.getByRole('heading', { name: 'TV Shows' }) },
  { name: 'Search', path: '/search', ready: (page) => page.getByRole('heading', { name: 'Search' }) },
  { name: 'details', path: '/media/hurt-locker', ready: (page) => page.getByRole('heading', { name: 'The Hurt Locker' }) },
  { name: 'Saved', path: '/saved', ready: (page) => page.getByRole('heading', { name: 'Saved' }) },
  { name: 'Live TV guide', path: '/live', ready: (page) => page.getByRole('heading', { name: 'Live TV' }) },
  { name: 'Settings', path: '/settings/account', ready: (page) => page.getByRole('heading', { name: 'Account' }).first() },
];

test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce', colorScheme: 'dark' });
});

async function expectContainedAndSemantic(page: Page, surface: string) {
  const audit = await page.evaluate(() => {
    const visible = (element: Element) => {
      const style = getComputedStyle(element);
      const bounds = element.getBoundingClientRect();
      return style.display !== 'none' && style.visibility !== 'hidden' && bounds.width > 0 && bounds.height > 0;
    };
    const unnamedControls = [...document.querySelectorAll('button, a[href], input, select, textarea')]
      .filter(visible)
      .filter((element) => {
        const text = element.textContent?.trim();
        const labelled = element.getAttribute('aria-label') || element.getAttribute('aria-labelledby') || element.getAttribute('title');
        const input = element as HTMLInputElement;
        const label = input.id ? document.querySelector(`label[for="${CSS.escape(input.id)}"]`) : element.closest('label');
        return !text && !labelled && !label;
      })
      .map((element) => element.outerHTML.slice(0, 180));
    const unlabelledImages = [...document.querySelectorAll('img')]
      .filter(visible)
      .filter((image) => !image.hasAttribute('alt'))
      .map((image) => image.outerHTML.slice(0, 180));
    return {
      viewportWidth: document.documentElement.clientWidth,
      documentWidth: document.documentElement.scrollWidth,
      mains: document.querySelectorAll('main, [role="main"]').length,
      headings: document.querySelectorAll('h1, h2, h3, h4, h5, h6, [role="heading"]').length,
      unnamedControls,
      unlabelledImages,
    };
  });

  expect(audit.documentWidth, `${surface} document overflow`).toBeLessThanOrEqual(audit.viewportWidth);
  expect(audit.mains, `${surface} has one primary content landmark`).toBe(1);
  expect(audit.headings, `${surface} exposes a heading`).toBeGreaterThan(0);
  expect(audit.unnamedControls, `${surface} has unnamed controls`).toEqual([]);
  expect(audit.unlabelledImages, `${surface} has images without alt text`).toEqual([]);
}

test('core product surfaces remain contained, reachable, and semantic', async ({ page }) => {
  for (const surface of surfaces) {
    await test.step(surface.name, async () => {
      await page.goto(surface.path);
      await expect(surface.ready(page)).toBeVisible();
      await expectContainedAndSemantic(page, surface.name);
    });
  }
});

test('critical phone controls meet the touch-target contract', async ({ page }) => {
  test.skip((page.viewportSize()?.width ?? 0) > 430, 'Touch sizing is a phone-width contract.');
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();

  const critical = page.locator('.topbar button:visible, .mobile-tabs a:visible, .mobile-tabs button:visible');
  expect(await critical.count()).toBeGreaterThan(0);
  const undersized = await critical.evaluateAll((elements) => elements.flatMap((element) => {
    const bounds = element.getBoundingClientRect();
    return bounds.width < 44 || bounds.height < 44
      ? [{ name: element.getAttribute('aria-label') || element.textContent?.trim(), width: bounds.width, height: bounds.height }]
      : [];
  }));
  expect(undersized).toEqual([]);
});

test('keyboard navigation, drawer dismissal, and focus return work without motion', async ({ page }) => {
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();

  await page.keyboard.press('Tab');
  await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('#main-content')).toBeFocused();

  const reducedMotion = await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches);
  expect(reducedMotion).toBe(true);

  const trigger = page.locator('.menu-button');
  if (await trigger.isVisible()) {
    await trigger.click();
    const drawer = page.getByRole('dialog', { name: 'Primary navigation' });
    await expect(drawer).toBeVisible();
    await expect(drawer.locator('a:focus, button:focus').first()).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(drawer).toBeHidden();
    await expect(trigger).toBeFocused();
  } else {
    const libraries = page.getByRole('link', { name: 'Libraries', exact: true }).first();
    await libraries.focus();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/libraries$/);
    await expect(page.locator('#main-content')).toBeFocused();
  }
});

test('player shell remains bounded and exposes an accessible recovery or playback surface', async ({ page }) => {
  await page.goto('/watch/fargo');
  const player = page.locator('.player-full, .player-state').first();
  await expect(player).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  const namedSurface = await player.evaluate((element) => Boolean(
    element.getAttribute('aria-label')
      || element.getAttribute('role')
      || element.querySelector('[aria-label], [role="alert"], [aria-live]'),
  ));
  expect(namedSurface).toBe(true);
});
