import test from 'node:test';
import assert from 'node:assert/strict';
import * as presentation from '@core/presentation/index.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';

async function load() {
  const h = hooks();
  return componentModule(new URL('../src/app/errors.tsx', import.meta.url), {react: h.react, '@core/presentation/index.ts': presentation, '../ui': {Notice: 'Notice', StateView: 'StateView'}});
}

test('X-04: raw error text is never shown; the catalogue says what failed and what to do', async () => {
  const m = await load();
  let retried = 0;
  const view = m.ErrorState({error: new Error('ECONNRESET at 10.0.0.4:32400'), context: 'library', retry: () => retried++});
  assert.equal(view.type, 'StateView');
  assert.equal(view.props.title, 'This library couldn’t load');
  assert.doesNotMatch(String(view.props.body), /ECONNRESET/);
  assert.equal(view.props.action.label, 'Try again');
  view.props.action.onClick();
  assert.equal(retried, 1);
});

test('X-04: a change conflict offers Refresh, not Try again, as a warning', async () => {
  const m = await load();
  let refreshed = 0;
  const notice = m.ErrorNotice({error: m.changedError, context: 'search', retry: () => {}, refresh: () => refreshed++});
  assert.equal(notice.props.tone, 'warning');
  assert.equal(notice.props.action.label, 'Refresh');
  notice.props.action.onClick();
  assert.equal(refreshed, 1);
});

test('X-04: permission failures offer no retry; cancellations render nothing', async () => {
  const m = await load();
  const denied = m.ErrorNotice({error: {code: 'forbidden', status: 403}, context: 'saved', retry: () => {}});
  assert.equal(denied.props.action, undefined);
  assert.match(String(denied.props.children), /access/);
  assert.equal(m.ErrorState({error: Object.assign(new Error('aborted'), {name: 'AbortError'}), context: 'home', retry: () => {}}), null);
  assert.equal(m.errorText(Object.assign(new Error('aborted'), {name: 'AbortError'}), 'playlist', 'save'), '');
  assert.match(m.errorText(new TypeError('Failed to fetch'), 'playlist', 'save'), /can’t reach your server/);
});
