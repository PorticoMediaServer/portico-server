// End-to-end walk-through: sign-in → profile PIN → home → search → play → music → watchlist → live TV → console.
// Usage: npm run smoke   (B=http://127.0.0.1:5414 npm run smoke for the production preview)
import { chromium } from 'playwright';
const B = process.env.B ?? "http://127.0.0.1:5404";
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
const p = await ctx.newPage();
p.setDefaultTimeout(8000);
const errs = [];
p.on('pageerror', (e) => errs.push(e.message));
p.on('console', (m) => m.type() === 'error' && errs.push(m.text()));
let failed = 0;
const step = async (name, fn) => { try { await fn(); console.log('ok  ', name); } catch (e) { failed++; console.log('FAIL', name, e.message.split('\n')[0]); } };
await p.goto(B + '/?as=signed-out&latency=fast');
await step('redirect to welcome', async () => { await p.waitForURL('**/welcome'); });
await step('sign in', async () => {
  await p.getByRole('link', { name: 'Sign In' }).click();
  await p.fill('#password', 'harbor-demo');
  await p.getByRole('button', { name: 'Sign In', exact: true }).click();
  await p.getByLabel('Digit 1').waitFor();
  await p.keyboard.type('123456');
  await p.waitForURL('**/servers');
});
await step('choose server', async () => { await p.getByRole('button', { name: /Harbor/ }).first().click(); await p.waitForURL('**/profiles'); });
await step('pin', async () => {
  await p.locator('.profile-tile', { hasText: 'Maya' }).click();
  for (const d of '1234') await p.getByRole('button', { name: d, exact: true }).click();
  await p.waitForURL(B + '/');
});
await step('home hero', async () => { await p.locator('.hero-content').waitFor(); });
await step('palette search navigates', async () => {
  await p.keyboard.press('Meta+k');
  await p.keyboard.type('meridian');
  await p.waitForTimeout(500);
  await p.keyboard.press('Enter');
  await p.waitForURL('**/show/meridian');
});
await step('play episode, video advances', async () => {
  await p.getByRole('button', { name: /Resume S3/ }).click();
  await p.waitForURL('**/watch/**');
  await p.waitForTimeout(3500);
  const t = await p.evaluate(() => document.querySelector('video')?.currentTime ?? -1);
  if (!(t > 0.5)) throw new Error('video time ' + t);
});
await step('player keyboard + close', async () => {
  await p.keyboard.press('c');
  await p.waitForTimeout(300);
  await p.keyboard.press('Escape');
  await p.waitForURL('**/show/meridian');
});
await step('music plays', async () => {
  await p.goto(B + '/album/northern-static');
  await p.getByRole('button', { name: 'Play', exact: true }).click();
  await p.locator('.mini').waitFor();
  await p.waitForTimeout(3000);
  const t = await p.evaluate(() => document.querySelector('audio')?.currentTime ?? -1);
  if (!(t > 0.5)) throw new Error('audio time ' + t);
});
await step('now playing opens with lyrics', async () => {
  await p.getByRole('button', { name: 'Lyrics' }).click();
  await p.locator('.lyric').first().waitFor();
});
await step('watchlist toggle', async () => {
  await p.keyboard.press('Escape');
  await p.waitForTimeout(400);
  await p.goto(B + '/movie/northbound');
  await p.getByRole('button', { name: 'Add to Watchlist' }).first().click();
  await p.getByText('Added to your Watchlist').waitFor();
});
await step('record from guide', async () => {
  await p.goto(B + '/live');
  await p.locator('.prog.now').nth(4).click();
  await p.getByRole('button', { name: /Record Rest of Show|Record Episode/ }).click();
  await p.getByText(/Recording .* at/).waitFor();
});
await step('console overview + users sheet', async () => {
  await p.goto(B + '/console');
  await p.locator('.cstat').first().waitFor();
  await p.goto(B + '/console/users');
  await p.getByText('Ingrid Lindqvist').click();
  await p.getByText('Library access').waitFor();
});
await step('logs filter', async () => {
  await p.goto(B + '/console/logs');
  await p.locator('.logline').first().waitFor();
  await p.fill('.cfilter input', 'SMART');
  await p.waitForTimeout(800);
  const n = await p.locator('.logline').count();
  const txt = await p.locator('.logline .lm').first().textContent();
  if (!txt.includes('SMART')) throw new Error('filter failed: ' + txt);
});
console.log(errs.length ? `console errors: ${errs.slice(0, 10).join(' | ')}` : 'No console errors.');
await b.close();
if (failed || errs.length) process.exit(1);
