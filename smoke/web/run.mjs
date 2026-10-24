#!/usr/bin/env node
// M0 web smoke: the core flows from M0-HANDS-ON.md sections 1-9 on the deployed web app.
//
//   SMOKE_ORIGIN=https://demo.getportico.tv \
//   SMOKE_CREDENTIALS=/path/to/private/credentials.json \
//   SMOKE_OUT=/path/to/scratch/run-1 \
//   SMOKE_CHECKPOINT=v2-M0-001 \
//   node smoke/web/run.mjs
//
// Optional: SMOKE_ONLY=boot,signin (comma list), SMOKE_VIEWPORTS=1440x900,390x844,
//           SMOKE_CHANNEL=chrome, SMOKE_PLAYWRIGHT=/path/to/playwright/index.mjs
//
// Every failure records the step, the expectation, what was actually seen, and the
// screenshot path. Credentials are read from a private file and never written to the
// report or the console.
import {mkdirSync} from 'node:fs';
import {join} from 'node:path';
import {createReport, assert, blocked} from '../lib/report.mjs';
import {loadCredentials, redactor} from '../lib/creds.mjs';
import {
  loadPlaywright, launch, Evidence, settle, clickAny, findAny, bodyText,
  samplePlayback, playbackEvidence, surfaceState,
} from '../lib/page.mjs';

const origin = (process.env.SMOKE_ORIGIN || 'https://demo.getportico.tv').replace(/\/$/, '');
const out = process.env.SMOKE_OUT || join(process.cwd(), 'smoke-out');
const checkpoint = process.env.SMOKE_CHECKPOINT || '';
const only = new Set((process.env.SMOKE_ONLY || '').split(',').map(s => s.trim()).filter(Boolean));
const viewports = (process.env.SMOKE_VIEWPORTS || '1440x900,390x844').split(',')
  .map(pair => pair.split('x').map(Number)).map(([width, height]) => ({width, height}));
const desktop = viewports[0];
const mobile = viewports[1] || {width: 390, height: 844};

// Media's measured fixtures on the demo (45 s H.264 640x360 + AAC; Abbey Road 2019 mix).
const FIXTURES = {
  movie: process.env.SMOKE_MOVIE_ID || 'jJxPchu5fFfLRAJWCOyfvQ',
  album: process.env.SMOKE_ALBUM_ID || 'MxluJLkSmbFU2RFs2sO_9Q',
};

mkdirSync(out, {recursive: true});
const credentials = loadCredentials(process.env.SMOKE_CREDENTIALS);
const redact = redactor(credentials.password);
const playwright = await loadPlaywright();
const report = createReport({surface: 'web', origin, checkpoint, out});

function want(id) { return only.size === 0 || only.has(id); }

async function failure(page, evidence, name, error, extra) {
  const shot = await evidence.shot(page, name);
  const state = await surfaceState(page).catch(() => ({}));
  const message = redact(error && error.message ? error.message : error);
  const wrapped = new Error(message + ` [screenshot ${shot}; url ${state.url || '?'}]`);
  wrapped.evidence = {screenshot: shot, surface: state, ...(extra || {})};
  return wrapped;
}

/** Records the app's own authorised API traffic; in-page fetch is not authenticated. */
function watchApi(page, calls = []) {
  page.on('response', response => {
    const url = new URL(response.url());
    if (!url.pathname.startsWith('/v1/')) return;
    calls.push({status: response.status(), method: response.request().method(), path: url.pathname});
    if (calls.length > 600) calls.shift();
  });
  return calls;
}

async function signIn(page, evidence) {
  await page.goto(origin + '/', {waitUntil: 'domcontentloaded', timeout: 45000});
  await settle(page, {quiet: 1200});
  if (!/\/sign-in/.test(page.url())) return {profile: 'already signed in', landing: page.url()};

  await page.locator('input[name=username]').first().fill(credentials.username, {timeout: 15000});
  await page.locator('input[name=password]').first().fill(credentials.password, {timeout: 15000});
  await clickAny(page, [{role: 'button', name: 'Sign in'}, {css: 'button[type=submit]'}], {label: 'Sign in button'});
  await page.waitForTimeout(3000);
  await settle(page, {quiet: 1200});

  if (/\/sign-in/.test(page.url())) {
    const text = await bodyText(page);
    const shot = await evidence.shot(page, 'signin-failed');
    throw Object.assign(new Error('sign-in did not leave the sign-in screen: ' + text.slice(0, 300)),
      {evidence: {screenshot: shot, text: text.slice(0, 1200)}});
  }

  if (/\/profiles/.test(page.url())) {
    const choice = await findAny(page, [
      {role: 'button', name: /Main profile/},
      {role: 'button', name: new RegExp(credentials.username, 'i')},
      {css: 'main button'},
    ]);
    assert(choice, 'profile screen shows no profile to choose');
    await choice.click({timeout: 10000});
    await page.waitForTimeout(3200);
    await settle(page, {quiet: 1000});
    assert(!/\/profiles/.test(page.url()), 'choosing a profile did not leave the profile screen');
    return {profile: 'chosen', landing: page.url()};
  }
  return {profile: 'single', landing: page.url()};
}

async function openItem(page, id) {
  await page.goto(`${origin}/item/${id}`, {waitUntil: 'domcontentloaded', timeout: 40000});
  await settle(page, {quiet: 2500});
}

/** Plays the open item and reports whether a real video surface appeared or the app died. */
async function playOpenItem(page, evidence, api, label) {
  const before = await page.evaluate(() => ({elements: document.querySelectorAll('*').length}));
  const apiMark = api.length;
  const control = await findAny(page, [
    {css: '[data-size=hero][data-variant=primary]'},
    {role: 'button', name: /^(Play|Resume|Start over)/},
  ]);
  assert(control, 'the item screen draws no Play control');
  const controlName = (await control.innerText().catch(() => '')).trim();
  await control.click({timeout: 10000});

  const readings = await samplePlayback(page, {samples: 8, interval: 700});
  const verdict = playbackEvidence(readings);
  const after = await page.evaluate(() => ({
    elements: document.querySelectorAll('*').length,
    textLength: (document.body.innerText || '').trim().length,
    videos: document.querySelectorAll('video').length,
    url: location.href,
  }));
  const errors = page.__smokeErrors || [];
  const crashed = after.elements < 200 && after.textLength < 40;
  const detail = {
    control: controlName, verdict: verdict.ok ? 'decoded' : verdict.reason, after,
    crashErrors: errors.slice(-3), api: api.slice(apiMark).filter(c => /playback|media|session/.test(c.path)).slice(0, 8),
  };
  const shot = await evidence.shot(page, label);
  return {verdict, after, crashed, detail, shot, before};
}

async function main() {
  const {browser, context, page: firstPage, events} = await launch(playwright, {viewport: desktop});
  const evidence = new Evidence(out);
  const api = watchApi(firstPage);
  let page = firstPage;
  page.__smokeErrors = [];
  page.on('pageerror', error => page.__smokeErrors.push(String(error).slice(0, 200)));

  // A playback crash can take the whole renderer with it. Later checks must not inherit that.
  async function recover() {
    if (!page.isClosed()) return false;
    page = await context.newPage();
    page.__smokeErrors = [];
    page.on('pageerror', error => page.__smokeErrors.push(String(error).slice(0, 200)));
    watchApi(page, api);
    return true;
  }

  const readBuild = async () => {
    const out = {};
    try {
      const response = await context.request.get(origin + '/v1/system', {timeout: 20000});
      const json = await response.json();
      out.version = json.version; out.id = json.id; out.name = json.name;
    } catch { out.version = null; }
    try {
      const response = await context.request.get(origin + '/portico-build.json', {timeout: 20000});
      out.build = response.status() === 200 ? await response.json() : null;
    } catch { out.build = null; }
    return out;
  };
  report.buildStart = await readBuild();
  report.buildInfo = report.buildStart;
  const expectedSha = (process.env.SMOKE_EXPECT_SHA || '').trim();
  const servedSha = report.buildStart.build && report.buildStart.build.commit;
  if (expectedSha) {
    const matches = servedSha && servedSha.startsWith(expectedSha.slice(0, 12));
    if (!matches) {
      report.note('build-pin', `STOP: expected ${expectedSha.slice(0, 12)} but the deployment serves ${servedSha || 'no build file'}; not running the flows`);
      const endBuild = await readBuild();
  const moved = JSON.stringify(endBuild.build) !== JSON.stringify(report.buildStart.build) ||
    endBuild.version !== report.buildStart.version;
  if (moved) {
    report.note('build-drift', `the deployment changed during the run: start ${JSON.stringify(report.buildStart)} end ${JSON.stringify(endBuild)}`);
  }
  const result = report.write();
  if (moved) {
    console.log('\nBUILD DRIFT: the deployment changed while the flows ran; treat this run as invalid');
    process.exit(4);
  }
      console.log(`\nSTOPPED on a build mismatch (expected ${expectedSha.slice(0, 12)}, served ${servedSha || 'none'})`);
      process.exit(3);
    }
    report.note('build-pin', `deployment serves ${servedSha.slice(0, 12)}, matching SMOKE_EXPECT_SHA`);
  }

  let signedIn = false;
  const libraryNames = [];
  const check = (id, run, options) => report.check(id, async () => { await recover(); return run(); }, options);

  async function openLibrary(name) {
    const link = page.locator('nav a[href]').filter({hasText: new RegExp(`^${name}$`, 'i')}).first();
    const present = await link.count();
    assert(present, `the sidebar has no ${name} library`);
    await link.click({timeout: 8000});
    await settle(page, {quiet: 1200});
    await page.locator('article[data-card-kind]').first().waitFor({timeout: 15000}).catch(() => {});
    await settle(page, {quiet: 800});
    return page.url();
  }

  try {
    if (want('boot')) {
      await check('boot', async () => {
        const response = await page.goto(origin + '/', {waitUntil: 'domcontentloaded', timeout: 45000});
        await settle(page, {quiet: 1200});
        const state = await surfaceState(page);
        assert(response && response.status() < 400, `document answered ${response ? response.status() : 'no response'}`);
        assert(state.textLength > 40, `blank surface on launch: ${JSON.stringify(state)}`);
        assert(events.pageErrors.length === 0, `page error on launch: ${events.pageErrors[0]}`);
        return {detail: `version ${report.buildStart && report.buildStart.version ? report.buildStart.version : 'unknown'}; landed on ${new URL(state.url).pathname}`,
          evidence: {screenshot: await evidence.shot(page, 'boot')}};
      });
    }

    if (want('signin')) {
      await check('signin', async () => {
        const result = await signIn(page, evidence);
        signedIn = true;
        const state = await surfaceState(page);
        assert(state.textLength > 40, `signed-in surface is blank: ${JSON.stringify(state)}`);
        return {detail: `${result.profile}; landed on ${new URL(state.url).pathname}`,
          evidence: {screenshot: await evidence.shot(page, 'after-signin')}};
      });
    } else if (want('home')) {
      // A home-only run still needs a session.
      await signIn(page, evidence).then(() => { signedIn = true; }).catch(() => {});
    }

    if (signedIn && want('home')) {
      await check('home', async () => {
        await page.goto(origin + '/', {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 2500});
        const state = await surfaceState(page);
        const cards = await page.locator('article[data-card-kind]').count();
        assert(state.textLength > 120, `Home rendered almost no text: ${JSON.stringify(state)}`);
        assert(cards >= 3, `Home has ${cards} cards; expected rows of fixture titles`);
        const rows = await page.evaluate(() => [...document.querySelectorAll('h2,[role=heading]')]
          .map(h => (h.textContent || '').trim()).filter(Boolean).slice(0, 10));
        const titles = await page.evaluate(() => [...document.querySelectorAll('article[data-card-kind] button[aria-label]')]
          .map(b => b.getAttribute('aria-label')).slice(0, 8));
        return {detail: `${cards} cards; rows: ${rows.slice(0, 6).join(' / ')}; titles: ${titles.slice(0, 3).join(' / ')}`,
          evidence: {screenshot: await evidence.shot(page, 'home'), rows, titles}};
      });
    }

    if (signedIn && want('library')) {
      await check('library', async () => {
        const nav = await page.evaluate(() => [...document.querySelectorAll('nav a[href]')]
          .map(a => ({name: (a.textContent || '').trim(), href: a.getAttribute('href')})).filter(l => l.href));
        libraryNames.push(...nav.map(item => item.name).filter(Boolean));
        assert(nav.length >= 3, `sidebar lists ${nav.length} entries: ${JSON.stringify(nav)}`);
        const target = nav.find(item => /^(Movies|Anime)$/i.test(item.name)) || nav.find(item => item.href.startsWith('/library/'));
        assert(target, `no library entry in the sidebar: ${JSON.stringify(nav.map(n => n.name))}`);
        await page.goto(new URL(target.href, origin).href, {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 2000});
        const text = await bodyText(page);
        const cards = await page.locator('article[data-card-kind]').count();
        assert(text.trim().length > 60, `library ${target.name} rendered almost no text`);
        assert(cards >= 1, `library ${target.name} shows no cards`);
        return {detail: `${target.name}: ${cards} cards; sidebar: ${nav.slice(0, 8).map(n => n.name).join(', ')}`,
          evidence: {screenshot: await evidence.shot(page, 'library'), cards}};
      });
    }

    if (signedIn && want('library-controls')) {
      await check('library-controls', async () => {
        let where = await openLibrary('Anime');
        const answer = async () => page.evaluate(() => {
          const text = document.body.innerText || '';
          const match = text.match(/(\d+)\s+of\s+\d+/);
          return {count: match ? match[0] : `${document.querySelectorAll('article[data-card-kind]').length} cards`,
            applied: [...document.querySelectorAll('[aria-label]')].map(e => e.getAttribute('aria-label')).filter(name => /filter/i.test(name))};
        });
        const before = await answer();
        const quick = await findAny(page, [
          {role: 'button', name: /^Unwatched/},
          {role: 'button', name: /^In progress/},
          {role: 'button', name: /^Favorites/},
        ], {timeout: 4000});
        assert(quick, `the library toolbar draws no quick filter (at ${new URL(where).pathname})`);
        const quickName = (await quick.innerText()).trim();
        await quick.click({timeout: 8000});
        await settle(page, {quiet: 1800});
        const a = await answer();
        await quick.click({timeout: 8000}).catch(() => {});
        await settle(page, {quiet: 1200});
        const filter = await findAny(page, [{role: 'button', name: /^Filters/}, {css: '[data-filters]'}], {timeout: 4000});
        assert(filter, 'the library toolbar draws no Filters control');
        await filter.click({timeout: 8000});
        await settle(page, {quiet: 1200});
        const panel = await page.evaluate(() => {
          const dialog = document.querySelector('[role=dialog]');
          return {open: !!dialog, title: dialog ? (dialog.querySelector('h1,h2,h3,[role=heading]')?.textContent || '').trim().slice(0, 40) : '',
            controls: dialog ? [...dialog.querySelectorAll('button,select,input,[role=checkbox],[role=radio],[role=menuitem],[role=option]')].length : 0,
            text: dialog ? (dialog.innerText || '').replace(/\n+/g, ' | ').slice(0, 200) : ''};
        });
        assert(panel.open, 'the Filters button opened no dialog');
        const confirm = await findAny(page, [{role: 'button', name: /^(Show \d|Apply|Done)/}], {timeout: 2000});
        if (confirm) { await confirm.click({timeout: 6000}).catch(() => {}); }
        else { await page.keyboard.press('Escape').catch(() => {}); }
        await page.waitForTimeout(700);
        const sort = await findAny(page, [{role: 'button', name: /^Sort/}, {role: 'button', name: /—|A–Z/}, {css: '[data-sort]'}], {timeout: 4000});
        assert(sort, 'the library toolbar draws no Sort control');
        const sortLabelBefore = (await sort.innerText().catch(() => '')).trim();
        await sort.click({timeout: 8000});
        await page.waitForTimeout(1000);
        const sorts = await page.evaluate(() => {
          const selectors = ['[role=menuitemradio]', '[role=menuitem]', '[role=option]', '[role=radio]',
            '[data-radix-popper-content-wrapper] button', '[role=dialog] button', '[role=listbox] button'];
          const seen = [];
          for (const selector of selectors) {
            for (const node of document.querySelectorAll(selector)) {
              const text = (node.textContent || '').trim();
              if (text && text.length < 60 && !seen.includes(text)) seen.push(text);
            }
          }
          return seen.slice(0, 14);
        });
        assert(sorts.length > 0, `the Sort control opened no options; it reads "${sortLabelBefore}"`);
        // Choose a different sort and prove the toolbar reflects it.
        const wanted = sorts.find(text => !sortLabelBefore.includes(text.split('\n')[0]));
        let appliedSort = 'no distinct option offered';
        if (wanted) {
          const option = page.getByText(wanted.split('\n')[0], {exact: true}).last();
          await option.click({timeout: 6000}).catch(() => {});
          await page.waitForTimeout(1800);
          appliedSort = (await sort.innerText().catch(() => '')).trim();
        }
        await page.keyboard.press('Escape').catch(() => {});
        return {detail: `quick ${quickName}: ${before.count} -> ${a.count}; filter panel ${panel.controls} controls; sort "${sortLabelBefore}" -> "${appliedSort}"; options: ${sorts.slice(0, 5).join(', ')}`,
          evidence: {screenshot: await evidence.shot(page, 'library-controls'), panel, sorts, before, after: a}};
      });
    }

    if (signedIn && want('library-tv-controls')) {
      await check('library-tv-controls', async () => {
        const report = {};
        for (const name of ['Movies', 'TV Shows']) {
          const link = page.locator('nav a[href]').filter({hasText: new RegExp(`^${name}$`, 'i')}).first();
          if (!(await link.count())) { report[name] = 'no sidebar entry'; continue; }
          await link.click({timeout: 8000});
          await settle(page, {quiet: 2000});
          const seeAll = await findAny(page, [{role: 'link', name: /^See all/}, {role: 'button', name: /^See all/}, {text: /^See all$/}], {timeout: 4000});
          if (seeAll) {
            await seeAll.click({timeout: 8000}).catch(() => {});
            await settle(page, {quiet: 1500});
            await page.locator('article[data-card-kind]').first().waitFor({timeout: 15000}).catch(() => {});
          }
          report[name] = await page.evaluate(() => {
            const labels = [...document.querySelectorAll('button')].map(b => (b.innerText || '').trim());
            return {filters: labels.some(text => /^Filters/.test(text)), sort: labels.some(text => /^Sort/.test(text) || /A–Z|—/.test(text)),
              seeAll: [...document.querySelectorAll('a,button')].some(e => /^see all/i.test((e.innerText || '').trim())),
              cards: document.querySelectorAll('article[data-card-kind]').length};
          });
        }
        const broken = Object.entries(report).filter(([, value]) => value && typeof value === 'object' && (!value.filters || !value.sort));
        assert(broken.length === 0, `these libraries draw no sort or filter control: ${broken.map(([name, value]) => `${name} (${JSON.stringify(value)})`).join('; ')}`);
        return {detail: JSON.stringify(report), evidence: {screenshot: await evidence.shot(page, 'library-tv-controls'), report}};
      });
    }

    if (signedIn && want('detail')) {
      await check('detail', async () => {
        await openItem(page, FIXTURES.movie);
        const state = await surfaceState(page);
        assert(/\/item\//.test(state.url), `movie detail did not open (at ${state.url})`);
        assert(state.textLength > 80, `movie detail rendered almost no text: ${JSON.stringify(state)}`);
        const play = await findAny(page, [{css: '[data-size=hero][data-variant=primary]'}, {role: 'button', name: /^Play/}]);
        assert(play, 'movie detail draws no Play control');
        return {detail: `12 Angry Men detail: ${state.textLength} chars, Play control present`,
          evidence: {screenshot: await evidence.shot(page, 'detail')}};
      });
    }

    if (signedIn && want('detail-show')) {
      await check('detail-show', async () => {
        await openLibrary('TV Shows').catch(() => openLibrary('Anime'));
        const showsTab = await findAny(page, [{role: 'button', name: /^Shows$/}, {role: 'tab', name: /^Shows$/}]);
        if (showsTab) { await showsTab.click({timeout: 6000}).catch(() => {}); await settle(page, {quiet: 1500}); }
        const card = page.locator('article[data-card-kind] button[aria-label]').first();
        assert(await card.count() > 0, `the Anime library lists no show to open (Shows tab ${showsTab ? 'present' : 'absent'})`);
        const title = await card.getAttribute('aria-label');
        await card.focus();
        await page.keyboard.press('Enter');
        await page.waitForTimeout(3500);
        await settle(page, {quiet: 1200});
        const state = await surfaceState(page);
        assert(/\/item\//.test(state.url), `opening show ${title} did not reach a detail screen (at ${state.url})`);
        assert(state.textLength > 80, 'show detail rendered almost no text');
        const playControl = await findAny(page, [{css: '[data-size=hero][data-variant=primary]'}, {role: 'button', name: /^(Play|Resume)/}]);
        return {detail: `${title}: ${state.textLength} chars at ${new URL(state.url).pathname}; hero control ${playControl ? 'present' : 'missing'}`,
          evidence: {screenshot: await evidence.shot(page, 'detail-show')}};
      });
    }

    // Core flow 5, first half. A 2xx never counts as playback: the surface must decode.
    if (signedIn && want('playback-direct')) {
      await check('playback-direct', async () => {
        await openItem(page, FIXTURES.movie);
        const result = await playOpenItem(page, evidence, api, 'playback-direct');
        if (result.crashed) {
          throw await failure(page, evidence, 'playback-direct-crash',
            new Error(`pressing ${result.detail.control || 'Play'} blanked the app: ${JSON.stringify(result.after)}`),
            result.detail);
        }
        if (!result.verdict.ok) throw await failure(page, evidence, 'playback-direct-failed', new Error(result.verdict.reason), result.detail);
        return {detail: `${result.verdict.width}x${result.verdict.height}, advanced ${result.verdict.advanced.toFixed(1)}s of ${result.verdict.duration}s`,
          evidence: {screenshot: result.shot, control: result.detail.control}};
      });
    }

    if (signedIn && want('playback-controls')) {
      await check('playback-controls', async () => {
        const live = await page.evaluate(() => {
          const video = document.querySelector('video');
          return video && video.readyState >= 2 ? {currentTime: video.currentTime, paused: video.paused} : null;
        });
        assert(live, 'no playing video to control (playback-direct must pass first)');
        await page.keyboard.press('Space');
        await page.waitForTimeout(900);
        const paused = await page.evaluate(() => document.querySelector('video')?.paused);
        await page.keyboard.press('Space');
        await page.waitForTimeout(900);
        const resumed = await page.evaluate(() => document.querySelector('video')?.paused);
        await page.keyboard.press('ArrowRight');
        await page.waitForTimeout(1400);
        const forward = await page.evaluate(() => document.querySelector('video')?.currentTime ?? null);
        assert(paused === true, `Space did not pause (paused=${paused})`);
        assert(resumed === false, `Space did not resume (paused=${resumed})`);
        assert(forward !== null && forward > live.currentTime, `seek did not advance (${live.currentTime} -> ${forward})`);
        return {detail: `pause ok, resume ok, ${live.currentTime.toFixed(2)}s -> ${forward.toFixed(2)}s`,
          evidence: {screenshot: await evidence.shot(page, 'playback-controls')}};
      });
    }

    if (signedIn && want('playback-subtitles')) {
      await check('playback-subtitles', async () => {
        const playing = await page.evaluate(() => [...document.querySelectorAll('video')].some(v => v.readyState >= 2));
        if (!playing) {
          throw blocked('no playing video to show captions on (see playback-direct)');
        }
        const toggle = await findAny(page, [
          {role: 'button', name: /subtitles|captions/i},
          {role: 'menuitem', name: /subtitles|captions/i},
          {css: '[data-subtitles]'},
        ]);
        assert(toggle, 'the player draws no captions or subtitles control on this title');
        await toggle.click({timeout: 8000});
        await page.waitForTimeout(900);
        const tracks = await page.evaluate(() => [...document.querySelectorAll('video')]
          .flatMap(v => [...v.textTracks].map(t => ({kind: t.kind, mode: t.mode, label: t.label}))));
        await toggle.click({timeout: 8000}).catch(() => {});
        await page.waitForTimeout(700);
        const after = await page.evaluate(() => [...document.querySelectorAll('video')]
          .flatMap(v => [...v.textTracks].map(t => ({kind: t.kind, mode: t.mode}))));
        assert(tracks.length > 0, 'the captions control produced no text track');
        assert(tracks.some(track => track.mode === 'showing'), `no text track is showing after the toggle: ${JSON.stringify(tracks)}`);
        return {detail: `tracks on: ${JSON.stringify(tracks)}; after off: ${JSON.stringify(after)}`,
          evidence: {screenshot: await evidence.shot(page, 'playback-subtitles'), tracks, after}};
      });
    }

    if (signedIn && want('playback-transcode')) {
      await check('playback-transcode', async () => {
        await openItem(page, FIXTURES.movie);
        const control = await findAny(page, [{css: '[data-size=hero][data-variant=primary]'}, {role: 'button', name: /^(Play|Resume)/}]);
        assert(control, 'no Play control to start a transcode check');
        const apiMark = api.length;
        await control.click({timeout: 10000});
        await page.waitForTimeout(2500);
        // The drawn quality control decides the transcode; 240p is the measured case.
        const quality = await findAny(page, [{role: 'button', name: /quality|240p/i}, {css: '[data-quality]'}]);
        if (quality) {
          await quality.click({timeout: 6000}).catch(() => {});
          await page.waitForTimeout(700);
          const choice = await findAny(page, [{role: 'menuitem', name: /240p/i}, {role: 'option', name: /240p/i}, {text: /^240p$/}]);
          if (choice) { await choice.click({timeout: 6000}).catch(() => {}); await page.waitForTimeout(2500); }
        }
        const readings = await samplePlayback(page, {samples: 8, interval: 700});
        const verdict = playbackEvidence(readings);
        const calls = api.slice(apiMark);
        const session = calls.find(c => c.method === 'POST' && c.path === '/v1/playback/sessions');
        const media = calls.filter(c => c.method === 'GET' && c.path.startsWith('/v1/media/'));
        const crashed = await page.evaluate(() => document.querySelectorAll('*').length < 200).catch(() => true);
        if (crashed) {
          throw blocked('the web app blank-screened on Play, so no transcode decision can be observed; see playback-direct');
        }
        if (!verdict.ok) {
          throw await failure(page, evidence, 'playback-transcode-failed',
            new Error(`transcoded playback did not decode: ${verdict.reason}; session ${session ? session.status : 'none'}; media ${media.map(m => m.status).join(',') || 'none'}`),
            {calls});
        }
        return {detail: `decoded ${verdict.width}x${verdict.height}, advanced ${verdict.advanced.toFixed(1)}s; media ${media.map(m => m.status).join(',')}; quality control ${quality ? 'used' : 'not drawn'}`,
          evidence: {screenshot: await evidence.shot(page, 'playback-transcode'), readings: readings.slice(0, 3)}};
      });
    }

    if (signedIn && want('album')) {
      await check('album', async () => {
        await page.goto(`${origin}/item/${FIXTURES.album}`, {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 2500});
        const text = await bodyText(page);
        assert(/\/item\//.test(page.url()), `album ${FIXTURES.album} did not resolve to an item screen (at ${page.url()})`);
        assert(/Abbey Road|album|song|track/i.test(text), `item ${FIXTURES.album} does not look like the album: ${text.replace(/\n/g, ' ').slice(0, 200)}`);
        const play = await findAny(page, [{role: 'button', name: /^(Play|Play all|Shuffle)/}, {css: '[data-size=hero][data-variant=primary]'}]);
        if (!play) return {detail: `album screen drew no play control; text ${text.slice(0, 120)}`, evidence: {screenshot: await evidence.shot(page, 'album')}};
        const apiMark = api.length;
        await play.click({timeout: 10000});
        await page.waitForTimeout(4000);
        const before = await page.evaluate(() => {
          const media = [...document.querySelectorAll('audio,video')];
          return media.map(element => ({tag: element.tagName.toLowerCase(), readyState: element.readyState, paused: element.paused, currentTime: Number((element.currentTime || 0).toFixed(2)), duration: element.duration})).slice(0, 3);
        });
        await page.waitForTimeout(2500);
        const audio = await page.evaluate(() => {
          const media = [...document.querySelectorAll('audio,video')];
          return media.map(element => ({tag: element.tagName.toLowerCase(), readyState: element.readyState, paused: element.paused, currentTime: Number((element.currentTime || 0).toFixed(2)), duration: element.duration})).slice(0, 3);
        });
        const calls = api.slice(apiMark).filter(c => /playback|media|session|queue/.test(c.path));
        const playing = audio.some((item, index) => item.readyState >= 2 && !item.paused && item.currentTime > (before[index]?.currentTime || 0));
        if (!playing) {
          throw await failure(page, evidence, 'album-failed',
            new Error(`album playback did not start: ${audio.map(a => `${a.tag} readyState ${a.readyState} ${a.paused ? 'paused' : 'playing'} at ${a.currentTime}s`).join('; ') || 'no media element'}`),
            {audio, calls});
        }
        return {detail: `album playing: ${audio.map(a => `${a.tag} ${a.readyState} at ${a.currentTime}s`).join('; ')}`,
          evidence: {screenshot: await evidence.shot(page, 'album'), audio, calls}};
      });
    }

    if (signedIn && want('search')) {
      await check('search', async () => {
        await page.goto(origin + '/search', {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 1200});
        const input = await findAny(page, [{role: 'searchbox'}, {css: 'input[type=search]'}, {role: 'textbox'}, {css: 'input[name=q]'}]);
        assert(input, 'Search draws no search field');
        await input.click({timeout: 8000});
        await input.fill('angry', {timeout: 8000});
        await page.keyboard.press('Enter');
        await page.waitForTimeout(3000);
        const cards = await page.locator('article[data-card-kind]').count();
        const text = await bodyText(page);
        assert(text.trim().length > 40, 'Search rendered almost no text for a query');
        assert(cards >= 1, `Search for "angry" returned ${cards} result cards`);
        return {detail: `query "angry": ${cards} cards`, evidence: {screenshot: await evidence.shot(page, 'search'), cards}};
      });
    }

    if (signedIn && want('search-open')) {
      await check('search-open', async () => {
        const card = page.locator('article[data-card-kind] button[aria-label]').first();
        assert(await card.count() > 0, 'search results hold no card to open');
        const name = await card.getAttribute('aria-label');
        await card.focus();
        await page.keyboard.press('Enter');
        await page.waitForTimeout(3000);
        await settle(page, {quiet: 1000});
        assert(/\/item\//.test(page.url()), `opening search result ${name} went to ${page.url()}`);
        return {detail: `opened ${name} at ${new URL(page.url()).pathname}`,
          evidence: {screenshot: await evidence.shot(page, 'search-open')}};
      });
    }

    if (signedIn && want('settings')) {
      await check('settings', async () => {
        await page.goto(origin + '/settings', {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 1500});
        const text = await bodyText(page);
        assert(text.trim().length > 60, `Settings rendered almost no text: ${JSON.stringify(text.slice(0, 120))}`);
        const entries = await page.evaluate(() => [...document.querySelectorAll('main h2,main h3,[role=heading]')]
          .map(h => (h.textContent || '').trim()).filter(Boolean).slice(0, 14));
        const drawn = ['Profile & account', 'Privacy', 'Playback', 'Audio & subtitles', 'Quality & data', 'Appearance'];
        const missing = drawn.filter(name => !text.includes(name));
        assert(missing.length === 0, `Settings is missing drawn sections: ${missing.join(', ')}`);
        return {detail: `sections: ${entries.join(' / ') || 'none'}`,
          evidence: {screenshot: await evidence.shot(page, 'settings'), entries}};
      });
    }

    if (signedIn && want('settings-owner')) {
      await check('settings-owner', async () => {
        await page.goto(origin + '/dashboard', {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 2500});
        const dash = await bodyText(page);
        assert(dash.trim().length > 60, 'owner Dashboard rendered almost no text');
        await page.goto(origin + '/settings', {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 2000});
        const general = await findAny(page, [{role: 'button', name: /^General$/}, {role: 'tab', name: /^General$/}, {text: /^General$/}], {timeout: 5000});
        assert(general, 'Settings draws no General (server) group for the owner');
        await general.click({timeout: 8000});
        await settle(page, {quiet: 1500});
        const librariesEntry = await findAny(page, [{role: 'button', name: /^Libraries$/}, {role: 'tab', name: /^Libraries$/}, {text: /^Libraries$/}], {timeout: 4000});
        if (librariesEntry) { await librariesEntry.click({timeout: 8000}); await settle(page, {quiet: 1800}); }
        const libs = await bodyText(page);
        assert(libs.trim().length > 40, 'owner Libraries settings rendered almost no text');
        const scan = await findAny(page, [{role: 'button', name: /scan/i}, {text: /^Scan/}], {timeout: 4000});
        if (!scan) {
          throw blocked(`the owner server settings never rendered, so no scan control could be checked; page was: ${libs.replace(/\n/g, ' ').slice(0, 200)}`);
        }
        await scan.click({timeout: 8000});
        await page.waitForTimeout(3000);
        const scanResult = 'scan control pressed';
        return {detail: `dashboard ${dash.length} chars; libraries ${libs.length} chars; ${scanResult}`,
          evidence: {screenshot: await evidence.shot(page, 'settings-libraries')}};
      });
    }

    if (signedIn && want('read')) {
      await check('read', async () => {
        await openLibrary('Books');
        const card = page.locator('article[data-card-kind]').first();
        assert(await card.count() > 0, 'the Books library lists no book');
        const title = await card.locator('button[aria-label]').first().getAttribute('aria-label').catch(() => 'unknown');
        const readButton = card.getByRole('button', {name: /^Read$/}).first();
        if (await readButton.count()) {
          await readButton.click({timeout: 8000});
        } else {
          await card.locator('button[aria-label]').first().focus();
          await page.keyboard.press('Enter');
        }
        await page.waitForTimeout(3500);
        await settle(page, {quiet: 1500});
        const state = await surfaceState(page);
        assert(/\/read\//.test(state.url), `book ${title} did not open a reader (at ${state.url})`);
        const text = await bodyText(page);
        assert(text.trim().length > 200, `reader rendered only ${text.trim().length} characters of a book`);
        await page.keyboard.press('ArrowRight');
        await page.waitForTimeout(1200);
        return {detail: `read ${title}: ${text.length} chars, page turn sent`,
          evidence: {screenshot: await evidence.shot(page, 'read'), excerpt: text.slice(0, 200)}};
      });
    }

    if (signedIn && want('live')) {
      await check('live', async () => {
        await page.goto(origin + '/live', {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 2500});
        const text = await bodyText(page);
        const channels = await page.evaluate(() => [...document.querySelectorAll('article[data-card-kind], [data-channel]')].length);
        assert(text.trim().length > 80, `Live screen rendered almost no text: ${JSON.stringify(text.slice(0, 120))}`);
        assert(channels > 0, 'Live guide shows no channels');
        let tuned = 'not tuned';
        const target = page.locator('article[data-card-kind] button[aria-label]').first();
        if (await target.count()) {
          const label = await target.getAttribute('aria-label');
          await target.focus();
          await page.keyboard.press('Enter');
          await page.waitForTimeout(3500);
          const readings = await samplePlayback(page, {samples: 6, interval: 700});
          const verdict = playbackEvidence(readings);
          tuned = `${label}: ${verdict.ok ? 'decoded' : verdict.reason}`;
          if (!verdict.ok) {
            throw await failure(page, evidence, 'live-play-failed',
              new Error(`tuning ${label} did not decode: ${verdict.reason}`), {readings: readings.slice(0, 4)});
          }
        }
        return {detail: `${channels} channels; ${tuned}`, evidence: {screenshot: await evidence.shot(page, 'live'), tuned}};
      });
    }

    if (signedIn && want('signout')) {
      await check('signout', async () => {
        await page.goto(origin + '/', {waitUntil: 'domcontentloaded'});
        await settle(page, {quiet: 1500});
        const menu = await findAny(page, [
          {role: 'button', name: /account menu|switch profile/i},
          {css: '[data-profile-menu]'},
        ]);
        assert(menu, 'the shell draws no account menu');
        await menu.click({timeout: 8000});
        await settle(page, {quiet: 800});
        const out = await findAny(page, [{role: 'menuitem', name: /sign out/i}, {role: 'button', name: /sign out/i}, {text: /^Sign out$/}]);
        assert(out, 'the account menu has no Sign out entry');
        await out.click({timeout: 8000});
        await page.waitForTimeout(3000);
        await settle(page, {quiet: 1000});
        const state = await surfaceState(page);
        assert(/\/sign-in/.test(state.url), `sign out did not return to sign-in (at ${state.url})`);
        return {detail: `returned to ${new URL(state.url).pathname}`, evidence: {screenshot: await evidence.shot(page, 'signout')}};
      });
    }

    if (want('mobile')) {
      await check('mobile', async () => {
        const run = await launch(playwright, {viewport: mobile});
        try {
          const mobileEvidence = new Evidence(out);
          run.page.on('pageerror', error => (run.page.__smokeErrors ||= []).push(String(error).slice(0, 200)));
          await run.page.goto(origin + '/', {waitUntil: 'domcontentloaded', timeout: 45000});
          await settle(run.page, {quiet: 2000});
          let state = await surfaceState(run.page);
          if (/\/sign-in/.test(state.url)) {
            await run.page.locator('input[name=username]').first().fill(credentials.username, {timeout: 15000});
            await run.page.locator('input[name=password]').first().fill(credentials.password, {timeout: 15000});
            await clickAny(run.page, [{role: 'button', name: 'Sign in'}, {css: 'button[type=submit]'}], {label: 'mobile sign in'});
            await run.page.waitForTimeout(3200);
            if (/\/profiles/.test(run.page.url())) {
              await clickAny(run.page, [{role: 'button', name: /Main profile/}, {css: 'main button'}], {label: 'mobile profile'});
              await run.page.waitForTimeout(3000);
            }
            state = await surfaceState(run.page);
          }
          assert(state.textLength > 60, `phone width rendered a blank surface: ${JSON.stringify(state)}`);
          const overflow = await run.page.evaluate(() => ({
            scrollWidth: document.documentElement.scrollWidth,
            clientWidth: document.documentElement.clientWidth,
          }));
          const cards = await run.page.locator('article[data-card-kind]').count();
          return {detail: `${state.url}: ${cards} cards, ${state.textLength} chars; horizontal overflow ${overflow.scrollWidth > overflow.clientWidth + 1 ? `yes (${overflow.scrollWidth}>${overflow.clientWidth})` : 'no'}`,
            evidence: {screenshot: await mobileEvidence.shot(run.page, 'mobile-home'), overflow, cards}};
        } finally {
          await run.context.close();
          await run.browser.close();
        }
      });
    }
  } finally {
    // Read the build before the context closes, or the drift check sees only nulls.
    try { report.buildEnd = await readBuild(); } catch (error) { report.buildEnd = {error: String(error).slice(0, 120)}; }
    await context.close();
    await browser.close();
  }

  const endBuild = report.buildEnd || {};
  report.buildInfo = {start: report.buildStart, end: endBuild, expected: expectedSha || null};
  const comparable = !!endBuild.build && !!report.buildStart.build;
  const moved = comparable && (JSON.stringify(endBuild.build) !== JSON.stringify(report.buildStart.build) ||
    endBuild.version !== report.buildStart.version);
  if (!comparable) {
    report.note('build-drift', `could not re-read the build at the end of the run (${JSON.stringify(endBuild).slice(0, 160)}); stability is unproven`);
  }
  if (moved) {
    report.note('build-drift', `the deployment changed during the run: start ${JSON.stringify(report.buildStart)} end ${JSON.stringify(endBuild)}`);
  }
  const result = report.write();
  if (moved) {
    console.log('\nBUILD DRIFT: the deployment changed while the flows ran; treat this run as invalid');
    process.exit(4);
  }
  console.log(`\n${result.verdict.toUpperCase()} ${result.counts.pass} pass / ${result.counts.fail} fail / ${result.counts.blocked} blocked`);
  console.log(`report: ${join(out, 'report.json')}`);
  console.log(`console errors ${events.consoleErrors.length}, page errors ${events.pageErrors.length}, failed requests ${events.failedRequests.length}`);
  process.exit(result.verdict === 'fail' ? 1 : 0);
}

main().catch(error => {
  console.error('smoke run crashed: ' + redact(error && error.stack ? error.stack : error).slice(0, 2000));
  process.exit(2);
});
