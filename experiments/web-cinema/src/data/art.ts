import manifest from './fixtures/art.json';
import type {ArtRef} from './types';

/**
 * Resolves artwork keys ("poster/m-charade") to image URLs. The fixture layer serves WebP files
 * from /art (built by scripts/build-art.mjs); a real adapter would return the server's image
 * endpoint here instead, e.g. `${server}/art/${key}?w=${width}`.
 */
const data = manifest as {keys: string[]; tints: Record<string, string>};
export const artKeys = new Set(data.keys);
export const artTints: Record<string, string> = data.tints;

export const paletteTint: Record<string, string> = {
  teal: '#2fa4a0', amber: '#d99a3a', crimson: '#c2424f', violet: '#8a5fd0', emerald: '#3fae78',
  cobalt: '#3f6fe0', rose: '#d4668f', gold: '#d8b44a', slate: '#6f8796', orange: '#e0763a',
};

export function artUrl(ref: ArtRef | undefined | null): string | undefined {
  if (!ref || !artKeys.has(ref)) return undefined;
  return `${import.meta.env.BASE_URL}art/${ref}.webp`;
}

export function artTint(ref: ArtRef | undefined | null): string | undefined {
  return ref ? artTints[ref] : undefined;
}
