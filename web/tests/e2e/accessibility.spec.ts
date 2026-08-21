import { expect, test } from '@playwright/test';

function relativeLuminance(hex: string) {
  const match = /^#([0-9a-f]{6})$/i.exec(hex.trim());
  if (!match) throw new Error(`Expected a six-digit hex color, received ${hex}`);
  const channels = [0, 2, 4].map((offset) => Number.parseInt(match[1].slice(offset, offset + 2), 16) / 255);
  const linear = channels.map((channel) => channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
  return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2];
}

function contrastRatio(foreground: string, background: string) {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  return (Math.max(foregroundLuminance, backgroundLuminance) + 0.05)
    / (Math.min(foregroundLuminance, backgroundLuminance) + 0.05);
}

test('approved text tokens meet WCAG AA contrast on core dark surfaces', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'Token contrast is checked once at the desktop breakpoint.');
  await page.goto('/home');
  const tokens = await page.evaluate(() => {
    const styles = getComputedStyle(document.documentElement);
    const read = (names: string[]) => Object.fromEntries(names.map((name) => [name, styles.getPropertyValue(name).trim()]));
    return {
      foregrounds: read(['--silver-muted', '--silver-dim', '--silver-soft']),
      backgrounds: read(['--projector', '--recess', '--slate', '--slate-raised', '--slate-bright']),
    };
  });
  const failures = Object.entries(tokens.foregrounds).flatMap(([foregroundName, foreground]) => Object.entries(tokens.backgrounds).flatMap(([backgroundName, background]) => {
    const ratio = contrastRatio(foreground, background);
    return ratio >= 4.5 ? [] : [{ foregroundName, foreground, backgroundName, background, ratio: Number(ratio.toFixed(2)) }];
  }));
  expect(failures, 'normal-size text tokens must remain at least 4.5:1 against approved dark surfaces').toEqual([]);
});

test('Web font distribution keeps the Manrope OFL notice discoverable', async ({ request }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'Static distribution evidence is checked once at the desktop breakpoint.');
  const [notice, readme, ...fonts] = await Promise.all([
    request.get('/fonts/OFL.txt'),
    request.get('/fonts/README.md'),
    ...['400', '500', '600', '700'].map((weight) => request.get(`/fonts/manrope-${weight}.ttf`)),
  ]);
  expect(notice.ok()).toBe(true);
  expect(await notice.text()).toContain('SIL OPEN FONT LICENSE Version 1.1');
  expect(readme.ok()).toBe(true);
  expect(await readme.text()).toContain('Keep `OFL.txt` with redistributed copies.');
  for (const font of fonts) {
    expect(font.ok()).toBe(true);
    expect(Number(font.headers()['content-length'] ?? 0)).toBeGreaterThan(0);
  }
});

test('keyboard users can skip the shell and route changes restore content focus', async ({ page }) => {
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();

  await page.keyboard.press('Tab');
  const skipLink = page.getByRole('link', { name: 'Skip to content' });
  await expect(skipLink).toBeFocused();
  await expect(skipLink).toBeVisible();

  await page.keyboard.press('Enter');
  await expect(page.locator('#main-content')).toBeFocused();

  await page.getByRole('link', { name: 'Libraries', exact: true }).first().click();
  await expect(page).toHaveURL(/\/libraries$/);
  await expect(page.locator('#main-content')).toBeFocused();
});

test('quick search results and profile menu are operable without a pointer', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name === 'phone', 'The phone shell uses the full Search destination.');
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();

  await page.keyboard.press('Control+K');
  const search = page.getByRole('combobox', { name: 'Quick search' });
  await expect(search).toBeFocused();
  await search.fill('Fargo');
  await expect(page.locator('.quick-search-panel')).toBeVisible();
  await expect(page.locator('.quick-search-panel a').first()).toBeVisible();
  await search.press('ArrowDown');
  await expect(search).toBeFocused();
  const activeOptionId = await search.getAttribute('aria-activedescendant');
  expect(activeOptionId).toBeTruthy();
  await expect(page.locator(`[data-search-result][id="${activeOptionId}"]`)).toHaveAttribute('data-active', 'true');
  await page.keyboard.press('Escape');
  await expect(search).toBeFocused();

  const profile = page.getByRole('button', { name: /Open profile menu/ });
  await profile.focus();
  await profile.press('ArrowDown');
  const menuItems = page.getByRole('menuitem');
  await expect(page.locator('[role="menuitem"]:focus')).toBeVisible();
  await page.keyboard.press('End');
  await expect(menuItems.last()).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(profile).toBeFocused();
});

test('normal-motion route transitions settle focus, scroll, and overlays', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'Normal-motion interaction coverage runs once at the desktop breakpoint.');
  await page.emulateMedia({ reducedMotion: 'no-preference', colorScheme: 'dark' });
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();
  await page.evaluate(() => window.scrollTo({ top: 320, behavior: 'auto' }));

  const transitionObserved = page.waitForFunction(() => document.querySelector('#main-content')?.classList.contains('route-entering') === true);
  await page.getByRole('link', { name: 'Libraries', exact: true }).first().click();
  await transitionObserved;
  await expect(page).toHaveURL(/\/libraries$/);
  await expect(page.locator('#main-content')).toBeFocused();
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
  await expect.poll(() => page.locator('#main-content').getAttribute('class')).not.toContain('route-entering');

  const profile = page.getByRole('button', { name: /Open profile menu/ });
  await profile.click();
  const menu = page.getByRole('menu');
  await expect(menu).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(menu).toBeHidden();
  await expect(profile).toBeFocused();
});

test('product reduced-motion preference disables route transitions when the browser allows motion', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'Product-motion preference coverage runs once at the desktop breakpoint.');
  await page.emulateMedia({ reducedMotion: 'no-preference', colorScheme: 'dark' });
  await page.addInitScript(() => localStorage.setItem('portico.web.installation-preferences.v1', JSON.stringify({ reduceMotion: true })));
  await page.goto('/home');
  await expect(page.getByRole('heading', { name: 'Fargo' })).toBeVisible();
  await expect(page.locator('.shell[data-reduce-motion="true"]')).toBeVisible();

  await page.getByRole('link', { name: 'Libraries', exact: true }).first().click();
  await expect(page).toHaveURL(/\/libraries$/);
  await expect(page.locator('#main-content')).toBeFocused();
  await expect(page.locator('#main-content')).not.toHaveClass(/route-entering/);
});
