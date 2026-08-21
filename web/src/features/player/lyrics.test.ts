import { describe, expect, it } from 'vitest';
import { activeLyricCueIndex, parseLyrics, selectLyricDocument } from './lyrics';

describe('LRC parsing', () => {
  it('normalizes metadata, offsets, repeated timestamps, cue ordering, and enhanced word timing', () => {
    const parsed = parseLyrics(`
[ar:  The Artist\u0000 ]
[al:The Album]
[ti:The Song]
[by:Lyricist]
[la:en]
[length:03:45.50]
[offset:+250]
[00:10.00][00:20.5]Hello <00:10.20>world
[00:05.75]Earlier line
[00:10.00]Second language
[00:30:25]Last line
`);

    expect(parsed.metadata).toEqual({
      album: 'The Album',
      artist: 'The Artist',
      author: 'Lyricist',
      language: 'en',
      lengthSeconds: 225.5,
      offsetMilliseconds: 250,
      title: 'The Song',
    });
    expect(parsed.cues.map(({ startSeconds, text }) => ({ startSeconds, text }))).toEqual([
      { startSeconds: 6, text: 'Earlier line' },
      { startSeconds: 10.25, text: 'Hello world\nSecond language' },
      { startSeconds: 20.75, text: 'Hello world' },
      { startSeconds: 30.5, text: 'Last line' },
    ]);
    expect(parsed.plainText).toBe('Hello world\nEarlier line\nSecond language\nLast line');
  });

  it('clamps extreme offsets and retains empty timed cues as intentional breaks', () => {
    const parsed = parseLyrics('[offset:-999999]\n[10:00.00]Opening\n[10:01.00]');
    expect(parsed.metadata.offsetMilliseconds).toBe(-600_000);
    expect(parsed.cues).toEqual([
      { id: '0-0', startSeconds: 0, text: 'Opening' },
      { id: '1000-1', startSeconds: 1, text: '' },
    ]);
  });

  it('prefers a valid synchronized source and falls back honestly when timing is malformed', () => {
    const selected = selectLyricDocument([
      { id: 'plain', synced: false, text: 'Plain lyric' },
      { id: 'timed', synced: true, text: '[00:00.00]First\n[00:12.00]Second' },
    ]);
    expect(selected?.sourceId).toBe('timed');
    expect(selected?.synchronized).toBe(true);

    const malformed = selectLyricDocument([
      { id: 'broken', synced: true, text: '[00:99.00]Broken timing\nActual words' },
    ]);
    expect(malformed?.synchronized).toBe(false);
    expect(malformed?.plainText).toBe('Broken timing\nActual words');
  });

  it('finds the active cue from the latest authoritative playback position', () => {
    const cues = parseLyrics('[00:05.00]First\n[00:10.00]Second\n[00:20.00]Third').cues;
    expect(activeLyricCueIndex(cues, Number.NaN)).toBe(-1);
    expect(activeLyricCueIndex(cues, 4.999)).toBe(-1);
    expect(activeLyricCueIndex(cues, 5)).toBe(0);
    expect(activeLyricCueIndex(cues, 19.999)).toBe(1);
    expect(activeLyricCueIndex(cues, 99)).toBe(2);
  });
});
