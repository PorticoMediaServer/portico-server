import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

/** The show workspace hook over a service that records what it is asked to select. */
async function workspace(selectGroup: (group: string) => Promise<void>) {
  const h = hooks();
  const service = {select: async () => {}, selectSeason: async () => {}, selectGroup};
  const app = await componentModule(new URL('../src/app/detail.ts', import.meta.url), {
    react: h.react,
    './content': {useService: () => ({service, snapshot: {phase: 'ready'}})},
    './session': {useSession: () => ({api: {}})},
    './viewer-scope': {useViewerScope: () => ({viewerId: 'v', serverId: 's'})},
    './not-interested': {RECOMMENDATIONS_RESET: 'reset', recommendationViewer: () => ''},
  });
  return h.render(() => app.useShowWorkspace({libraryId: 'library', showId: 'show'}));
}

test('choosing an episode group selects the group that was chosen', async () => {
  const asked: string[] = [];
  const show = await workspace(async group => { asked.push(group); });
  // A show split into ranges of 100 offers these in place of seasons; each tab passes its own id.
  for (const group of ['all', 'absolute:101-200', 'specials']) show.selectGroup(group);
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(asked, ['all', 'absolute:101-200', 'specials']);
});

test('a group the server no longer offers does not throw out of the click', async () => {
  const show = await workspace(() => { throw new Error('Choose a server-provided episode group.'); });
  assert.doesNotThrow(() => show.selectGroup('absolute:901-1000'));
  await new Promise(resolve => setImmediate(resolve));
});
