import {test} from 'node:test';
import assert from 'node:assert/strict';
import type {DockTraceReceipt} from '../src/kit/dock/dock-trace.ts';

const sample = (overrides: Partial<DockTraceReceipt['samples'][number]>) => ({
  at: 0, target: 'bar', connected: true, parent: 'section', opacity: '1', transform: 'none', top: 0,
  duration: 320, delay: 0, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)', playState: 'running', currentTime: 0, ...overrides,
} as DockTraceReceipt['samples'][number]);

test('the observation names entry, staged words, partial handover and host survival from samples alone', async () => {
  const {dockTraceFindings} = await import('../src/kit/dock/dock-trace.ts');
  const receipt: DockTraceReceipt = {
    startedAt: 0, endedAt: 900, visibleAtStart: true,
    samples: [
      sample({at: 16, target: 'bar', transform: 'matrix(1, 0, 0, 1, 0, 92)', opacity: '0'}),
      sample({at: 96, target: 'identity', opacity: '0', delay: 300}),
      sample({at: 480, target: 'identity', opacity: '0.35', duration: 160, delay: 0, easing: 'linear'}),
      sample({at: 700, target: 'bar', connected: false, parent: null}),
    ],
  };
  const findings = dockTraceFindings(receipt).join('\n');
  assert.match(findings, /entry: bar sampled/);
  assert.match(findings, /words: sampled with the authored 300ms delay/);
  assert.match(findings, /handover: partial word opacity 0\.35 retargeted in 160ms linear/);
  assert.match(findings, /host: bar sampled disconnected/);
});

test('a settled bar is reported as unobserved entrance, never replayed', async () => {
  const {dockTraceFindings} = await import('../src/kit/dock/dock-trace.ts');
  const findings = dockTraceFindings({startedAt: 0, endedAt: 100, visibleAtStart: false, samples: [sample({at: 10, opacity: '1'})]}).join('\n');
  assert.match(findings, /bar was not visibly connected at the start; entrance timing is unobserved/);
  assert.match(findings, /no partial-opacity 160ms linear sample/);
  assert.match(findings, /host: no disconnection sampled/);
});
