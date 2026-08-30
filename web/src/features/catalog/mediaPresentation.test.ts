import { describe, expect, it } from 'vitest';
import type { MediaItem } from '../../data/models';
import { canonicalMediaKind, mediaPresentation, searchGroupFilterKinds } from './mediaPresentation';

function media(entityKind: string): Pick<MediaItem, 'entityKind'> {
  return { entityKind: entityKind as MediaItem['entityKind'] };
}

describe('canonical media presentation', () => {
  it('keeps first-class families distinct and treats unknown values neutrally', () => {
    expect(mediaPresentation(media('audiobook'))).toMatchObject({ kind: 'audiobook', group: 'audiobooks', artworkShape: 'poster', completionNoun: 'finished' });
    expect(mediaPresentation(media('live-channel'))).toMatchObject({ kind: 'live-channel', group: 'live-dvr', artworkShape: 'landscape' });
    expect(mediaPresentation(media('recording'))).toMatchObject({ group: 'live-dvr', label: 'DVR recording' });
    expect(mediaPresentation(media('future-provider-kind'))).toMatchObject({ kind: 'unknown', group: 'other', label: 'Media' });
    expect(canonicalMediaKind(media('person'))).toBe('person');
    expect(canonicalMediaKind(media('live_channel'))).toBe('unknown');
  });

  it('expands one visible search group to every allowed member kind', () => {
    const group = { entityKind: 'track', resultKinds: ['artist', 'album', 'track'] };
    expect(searchGroupFilterKinds(group as never)).toEqual(['track', 'artist', 'album']);
    expect(searchGroupFilterKinds(group as never, new Set(['artist', 'album']))).toEqual(['artist', 'album']);
  });
});
