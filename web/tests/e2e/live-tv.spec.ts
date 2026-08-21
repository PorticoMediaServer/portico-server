import { expect, test } from '@playwright/test';

test('Live TV keeps Guide, Channels, and DVR usable at every supported viewport', async ({ page }) => {
  await page.goto('/live');
  await expect(page.getByRole('heading', { name: 'Live TV' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Select News 7' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Live programming' })).toBeVisible();
  await expect(page.getByLabel('Program guide')).toBeVisible();

  await page.getByRole('link', { name: 'Channels', exact: true }).click();
  await expect(page.getByLabel('Filter channels')).toBeVisible();
  await page.getByLabel('Filter channels').fill('Cinema');
  await expect(page.getByRole('heading', { name: 'Cinema North' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Select Coastal Sports' })).toHaveCount(0);

  await page.getByRole('link', { name: 'DVR', exact: true }).click();
  await expect(page.getByText('Saturday Cinema')).toBeVisible();
  await page.getByRole('button', { name: /^Rules/ }).click();
  await expect(page.getByText('Coastal Hockey')).toBeVisible();

  const dimensions = await page.evaluate(() => ({ viewport: document.documentElement.clientWidth, document: document.documentElement.scrollWidth }));
  expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
});

test('1280 guide rows and time cells remain bounded inside one horizontal scroller', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'short-laptop', 'Geometry regression is pinned to the 1280x720 laptop viewport.');
  await page.goto('/live');
  const scroller = page.getByLabel('Program guide');
  await expect(scroller).toBeVisible();

  const geometry = await scroller.evaluate((element) => {
    const rows = [...element.querySelectorAll<HTMLElement>('.live-guide-row')];
    const overlaps = rows.flatMap((row, rowIndex) => {
      const cells = [...row.querySelectorAll<HTMLElement>('.live-guide-program-track > button')]
        .map((cell) => cell.getBoundingClientRect())
        .sort((left, right) => left.left - right.left);
      return cells.slice(1).flatMap((cell, index) => cell.left < cells[index].right - 1 ? [{ rowIndex, previousRight: cells[index].right, nextLeft: cell.left }] : []);
    });
    const rowMisalignment = rows.flatMap((row, rowIndex) => {
      const channel = row.querySelector<HTMLElement>('.live-guide-channel')?.getBoundingClientRect();
      const track = row.querySelector<HTMLElement>('.live-guide-program-track')?.getBoundingClientRect();
      return channel && track && Math.abs(channel.top - track.top) > 1 ? [{ rowIndex, channelTop: channel.top, trackTop: track.top }] : [];
    });
    return {
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
      overlaps,
      rowMisalignment,
      documentWidth: document.documentElement.scrollWidth,
      viewportWidth: document.documentElement.clientWidth,
    };
  });

  expect(geometry.scrollWidth).toBeGreaterThanOrEqual(geometry.clientWidth);
  expect(geometry.overlaps).toEqual([]);
  expect(geometry.rowMisalignment).toEqual([]);
  expect(geometry.documentWidth).toBeLessThanOrEqual(geometry.viewportWidth);
});
