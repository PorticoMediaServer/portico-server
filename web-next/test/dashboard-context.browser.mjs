import assert from 'node:assert/strict';

// The caller owns one browser worker, Vite, its profile and cleanup. These
// labelled synthetic reads exercise the unchanged production HistoryScreen;
// they are not genuine producer, authentication or frame-parity acceptance.
export async function checkHistoryContext(page, origin) {
  const checks = [];
  await page.goto(`${origin}/test/dashboard.html?case=context&run=${Date.now()}#history`, {waitUntil: 'networkidle'});
  const search = page.getByRole('searchbox', {name: 'Search history', exact: true});
  await page.locator('[data-table-row^="original-"]').first().waitFor();
  await search.fill('Fixture');
  await page.waitForFunction(() => window.dashboardProof.queries.some(q => q.kind === 'plays' && q.filters.search === 'Fixture'));
  assert.equal(await search.evaluate(e => e === document.activeElement), true);
  checks.push('Debounced query replacement retains the mounted search input and keyboard focus');

  for (const [name, value] of [['Media', 'Movies'], ['Range', 'Last 7 days']]) {
    await page.getByRole('combobox', {name, exact: true}).click();
    await page.getByRole('option', {name: value, exact: true}).click();
  }
  await page.getByRole('columnheader', {name: 'Title'}).getByRole('button').click();
  await page.waitForFunction(() => {
    const q = window.dashboardProof.queries.filter(q => q.kind === 'plays').at(-1);
    return q.filters.search === 'Fixture' && q.filters.media === 'movies' && q.filters.period === '7d' && q.filters.sort === 'title';
  });
  await page.locator('[class*="tableScroll"]').hover();
  await page.mouse.wheel(0, 10000);
  await page.waitForFunction(() => window.dashboardProof.queries.some(q => q.kind === 'plays' && q.cursor === '100'));
  await page.getByRole('button', {name: 'Fixture refresh context', exact: true}).click();
  await page.locator('[data-table-row^="refresh-"]').first().waitFor();
  let observed = await page.evaluate(() => {
    const queries = window.dashboardProof.queries.filter(q => q.kind === 'plays');
    const initial = queries[0].context;
    return {initial, fresh: queries.filter(q => q.context.until !== initial.until), actions: window.dashboardProof.actions};
  });
  assert.equal(observed.fresh[0].cursor ?? '', '');
  assert.notEqual(observed.initial.until, observed.fresh[0].context.until);
  assert.equal(observed.fresh[0].filters.search, 'Fixture');
  assert.equal(observed.fresh[0].filters.media, 'movies');
  assert.equal(observed.fresh[0].filters.period, '7d');
  assert.equal(observed.fresh[0].filters.sort, 'title');
  assert.ok(observed.actions.includes('old-history-aborted'));
  assert.equal(await page.locator('[data-table-row^="original-"]').count(), 0);
  assert.equal(await page.getByText('100 plays', {exact: true}).count(), 1);
  checks.push('New cutoff starts at the first page, discards old rows/count/cursor and retains selected query');

  await page.getByRole('button', {name: 'Fixture release late page', exact: true}).click();
  await page.waitForFunction(() => window.dashboardProof.actions.includes('old-history-released'));
  await page.waitForTimeout(100);
  assert.equal(await page.locator('[data-table-row^="original-"]').count(), 0);
  assert.equal(await page.getByText('100 plays', {exact: true}).count(), 1);
  checks.push('An aborted old continuation resolving late cannot append its rows or replace the new total');

  await page.getByRole('button', {name: 'Fixture change zone', exact: true}).click();
  await page.locator('[data-table-row^="zone-"]').first().waitFor();
  observed = await page.evaluate(() => window.dashboardProof.queries.filter(q => q.kind === 'plays' && q.context.timeZone === 'Asia/Kathmandu'));
  assert.equal(observed[0].cursor ?? '', '');
  assert.equal(observed[0].filters.search, 'Fixture');
  assert.equal(observed[0].filters.media, 'movies');
  assert.equal(observed[0].filters.period, '7d');
  assert.equal(observed[0].filters.sort, 'title');
  assert.equal(await page.locator('[data-table-row^="refresh-"]').count(), 0);
  checks.push('New device-zone context starts at the first page without relabelling previous-zone rows');

  await page.getByRole('button', {name: 'Fixture same context', exact: true}).click();
  await page.waitForFunction(() => window.dashboardProof.queries.filter(q => q.kind === 'plays' && q.context.timeZone === 'Asia/Kathmandu').length >= 2);
  assert.equal(await search.inputValue(), 'Fixture');
  assert.equal(await page.locator('[data-table-row^="zone-"]').count() > 0, true);
  checks.push('Same-context binding replacement retains the query and current results');

  await page.getByRole('button', {name: 'Fixture other server', exact: true}).click();
  await page.waitForFunction(() => window.dashboardProof.queries.some(q => q.kind === 'plays' && q.serverId === 'fixture-other'));
  assert.equal(await search.inputValue(), '');
  const scopeQuery = await page.evaluate(() => window.dashboardProof.queries.find(q => q.kind === 'plays' && q.serverId === 'fixture-other'));
  assert.equal(scopeQuery.cursor ?? '', '');
  assert.equal(scopeQuery.filters.media, undefined);
  assert.equal(scopeQuery.filters.period, 'all');
  checks.push('Actual scope replacement still resets query and pagination independently of context refresh');

  await page.goto(`${origin}/test/dashboard.html?case=context&run=${Date.now()}#history`, {waitUntil: 'networkidle'});
  await page.locator('[data-table-row^="original-"]').first().waitFor();
  await page.locator('[class*="tableScroll"]').hover();
  await page.mouse.wheel(0, 10000);
  await page.waitForFunction(() => window.dashboardProof.queries.some(q => q.kind === 'plays' && q.cursor === '100'));
  const before = await page.evaluate(() => window.dashboardProof.queries.filter(q => q.kind === 'plays').length);
  await page.getByRole('button', {name: 'Fixture fresh capture same quarter', exact: true}).click();
  await page.waitForFunction(before => window.dashboardProof.queries.filter(q => q.kind === 'plays').length > before, before);
  const fresh = await page.evaluate(before => window.dashboardProof.queries.filter(q => q.kind === 'plays').slice(before), before);
  assert.equal(fresh[0].cursor ?? '', '');
  const context = await page.evaluate(() => window.dashboardProof.queries.filter(q => q.kind === 'plays')[0].context);
  assert.deepEqual(fresh[0].context, context);
  checks.push('A fresh captured context in the same completed quarter also restarts at the first page');
  return checks;
}
