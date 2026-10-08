import type {ImageSourcePropType} from 'react-native';
import {images, tints} from './fixtures/art.generated';
import type {ArtRef} from './types';

/**
 * Resolves artwork keys ("poster/m-charade") to image sources. The fixture layer bundles images;
 * a real adapter would return `{uri: serverUrl + '/art/' + key + '?w=…'}` here instead.
 */
export function resolveArt(ref: ArtRef | undefined | null): ImageSourcePropType | undefined {
  if (!ref) return undefined;
  return images[ref];
}

export function artTint(ref: ArtRef | undefined | null): string | undefined {
  return ref ? tints[ref] : undefined;
}
