import test from 'node:test';
import assert from 'node:assert/strict';

const store = new Map<string, string>();
(globalThis as any).localStorage = {getItem: (key: string) => store.get(key) ?? null, setItem: (key: string, value: string) => { store.set(key, value); }};
const {readReminders, toggleReminder} = await import('../src/app/channel-reminders.ts');

test('saving reminder 101 keeps every upcoming programme', () => {
  const start = Date.now() + 3_600_000;
  for (let i = 0; i < 101; i++) toggleReminder({programId: `programme-${i}`, channelId: 'channel', title: `Programme ${i}`, channelName: 'Channel', start});
  assert.equal(readReminders().length, 101);
  assert.equal(readReminders()[0]?.programId, 'programme-0');
  assert.equal(JSON.parse(store.get('portico.channels.reminders.v1')!).length, 101);
});
