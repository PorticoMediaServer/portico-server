import { expect, test, type Page } from '@playwright/test';

async function expectNoDocumentOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    document: document.documentElement.scrollWidth,
  }));
  expect(dimensions.document, `document width ${dimensions.document}px exceeds ${dimensions.viewport}px`).toBeLessThanOrEqual(dimensions.viewport);
}

async function expectReadableVisibleText(page: Page) {
  const undersized = await page.evaluate(() => {
    const visible = (element: Element) => {
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      return rect.width > 0 && rect.height > 0 && rect.bottom > 0 && rect.top < innerHeight
        && style.display !== 'none' && style.visibility !== 'hidden';
    };
    return [...document.querySelectorAll('body *')]
      .filter((element) => visible(element) && element.children.length === 0 && element.textContent?.trim())
      .map((element) => ({
        text: element.textContent?.trim().slice(0, 60),
        fontSize: Number(getComputedStyle(element).fontSize.replace('px', '')),
      }))
      .filter((entry) => entry.fontSize < 14);
  });
  expect(undersized).toEqual([]);
}

test('Home preserves the artwork-led shell without overflow or tiny text', async ({ page }) => {
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Continue Watching' })).toBeVisible();
  await expect(page.getByText('People on this server are watching')).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Primary navigation' }).first()).toBeVisible();
  await expectNoDocumentOverflow(page);
  await expectReadableVisibleText(page);
});

test('TV library exposes the canonical hierarchy and poster presentation', async ({ page }) => {
  await page.goto('/library/fixture-tv');
  await expect(page.getByRole('heading', { name: 'TV Shows' })).toBeVisible();
  for (const pivot of ['Discover', 'Shows', 'Episodes', 'Collections', 'Categories']) {
    await expect(page.getByRole('link', { name: pivot, exact: true })).toBeVisible();
  }
  await page.getByRole('link', { name: 'Shows', exact: true }).click();
  const firstPoster = page.locator('.library-media-grid.poster .artwork-stage').first();
  await expect(firstPoster).toBeVisible();
  const ratio = await firstPoster.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    return rect.width / rect.height;
  });
  expect(ratio).toBeCloseTo(2 / 3, 1);
  await expect(page.getByRole('button', { name: 'Jump to beginning' })).toBeVisible();
  await expectNoDocumentOverflow(page);
  await expectReadableVisibleText(page);
});

test('quick search and profile actions use real overlays above the page', async ({ page }) => {
  await page.goto('/home');
  const isPhone = (page.viewportSize()?.width ?? 0) <= 620;
  if (isPhone) {
    await page.getByRole('button', { name: 'Open full search' }).click();
    await expect(page).toHaveURL(/\/search$/);
  } else {
    const search = page.getByRole('search').getByLabel('Quick search');
    await search.fill('Fargo');
    const searchPanel = page.locator('.quick-search-panel');
    await expect(searchPanel).toBeVisible();
    await expect(searchPanel.getByRole('link', { name: /Open Fargo/ }).first()).toBeVisible();
    const panelLayer = await searchPanel.evaluate((element) => Number(getComputedStyle(element).zIndex));
    expect(panelLayer).toBeGreaterThanOrEqual(500);
  }

  await page.goto('/home');
  await expect(page.locator('.notification-count-badge')).toHaveCSS('font-size', '14px');
  await page.getByRole('button', { name: /Open profile menu/ }).click();
  const profileMenu = page.getByRole('menu');
  await expect(profileMenu).toBeVisible();
  await expect(profileMenu.getByRole('menuitem', { name: /Account settings/ })).toBeVisible();
  await expect(profileMenu.locator('.profile-menu-count')).toHaveCSS('font-size', '14px');
});

test('Settings keeps a useful second rail at laptop widths and readable fields everywhere', async ({ page }) => {
  await page.goto('/settings/account');
  await expect(page.getByRole('heading', { name: 'Account' }).first()).toBeVisible();
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Email')).toBeVisible();
  const width = page.viewportSize()?.width ?? 0;
  if (width > 1380) {
    const layout = page.locator('.portico-settings-layout');
    const navigation = page.locator('.portico-settings-nav');
    const content = page.locator('.portico-settings-workspace');
    await expect(layout).toHaveCSS('display', 'grid');
    const [navigationBox, contentBox] = await Promise.all([navigation.boundingBox(), content.boundingBox()]);
    expect(navigationBox).not.toBeNull();
    expect(contentBox).not.toBeNull();
    expect(navigationBox!.x + navigationBox!.width).toBeLessThan(contentBox!.x);
    expect(contentBox!.width).toBeGreaterThan(560);
  }
  await expectNoDocumentOverflow(page);
  await expectReadableVisibleText(page);
});

test('phone shell gives primary chrome full-size targets', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone', 'Phone-only interaction target contract');
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();
  const controls = await page.locator('.topbar button').evaluateAll((elements) => elements.map((element) => {
    const rect = element.getBoundingClientRect();
    return { label: element.getAttribute('aria-label'), width: rect.width, height: rect.height };
  }));
  expect(controls.length).toBeGreaterThanOrEqual(3);
  for (const control of controls) {
    expect(control.width, `${control.label} width`).toBeGreaterThanOrEqual(44);
    expect(control.height, `${control.label} height`).toBeGreaterThanOrEqual(44);
  }
  await expectNoDocumentOverflow(page);
});
