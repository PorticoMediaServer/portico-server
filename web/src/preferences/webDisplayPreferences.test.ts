import { describe, expect, it } from 'vitest';
import {
  completeHomeRowOrder,
  defaultWebDisplayPreferences,
  normalizeWebDisplayPreferences,
  orderHomeRows,
  recordRecentSearch,
} from './webDisplayPreferences';

describe('web display preferences', () => {
  it('defaults every absent network bucket to Original without replacing saved lower quality', () => {
    expect(normalizeWebDisplayPreferences({
      playbackQuality: {wifi: 'standard', cellular: 'data-saver'},
    }).playbackQuality).toEqual({
      local: 'original',
      wifi: 'standard',
      cellular: 'data-saver',
      unknown: 'original',
    });
  });

  it('normalizes unsafe and stale values without losing supported choices', () => {
    expect(normalizeWebDisplayPreferences({
      cardSizePercent: 999,
      homeRowOrder: ['recent', '', 'recent', 'continue'],
      hiddenHomeRows: ['activity'],
      skipBackSeconds: 25,
      skipForwardSeconds: 45,
      upNextCountdownSeconds: 0,
      subtitleSize: 'large',
    })).toMatchObject({
      cardSizePercent: 150,
      homeRowOrder: ['recent', 'continue'],
      hiddenHomeRows: ['activity'],
      skipBackSeconds: defaultWebDisplayPreferences.skipBackSeconds,
      skipForwardSeconds: 45,
      upNextCountdownSeconds: 0,
      subtitleSize: 'large',
    });
  });

  it('pins playback continuity first, then preserves an explicit Home order and appends new rows by server priority', () => {
    const rows = [
      { id: 'continue', priority: 100 },
      { id: 'recent', priority: 80 },
      { id: 'activity', priority: 60 },
      { id: 'new-row', priority: 70 },
    ];
    expect(orderHomeRows(rows, ['activity', 'continue']).map((row) => row.id)).toEqual(['continue', 'activity', 'new-row', 'recent']);
    expect(completeHomeRowOrder(rows, ['activity', 'continue'])).toEqual(['continue', 'activity', 'new-row', 'recent']);
  });

  it('keeps recent searches unique and most-recent first', () => {
    expect(recordRecentSearch(['Fargo', 'The Rookie'], ' fargo ')).toEqual(['fargo', 'The Rookie']);
    expect(recordRecentSearch(['one', 'two'], 'three', 2)).toEqual(['three', 'one']);
  });
});
