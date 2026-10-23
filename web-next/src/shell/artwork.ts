import type {AuthRecovery, HttpLocalApi} from '@core/index.ts';
import {ambientArtworkSource, artworkWidthBucket, type AmbientArtworkIdentity, type ArtworkRequestOptions} from '@core/artwork/index.ts';
import {protectedImageRuntime} from './protected-image';
export {artworkFailure} from './protected-image';

export function foregroundArtwork(url: string | undefined, pixels: number, aspect: number, identity: AmbientArtworkIdentity) {
  // The accepted ambient parser also validates selected route, digest and query identity.
  if (!ambientArtworkSource(url, identity)) return undefined;
  const [path, query] = url!.split('?'), params = new URLSearchParams(query);
  params.delete('size'); params.set('w', String(artworkWidthBucket(pixels, {aspect}))); params.sort();
  const selected = path + '?' + params.toString();
  return {url: selected, cacheKey: JSON.stringify([identity.serverId, identity.viewerId, identity.authorityKey, selected])};
}
/** Real credentials go through the selected route transport, never an img URL or localStorage. */
export function authenticatedArtwork(options: {
  api: Pick<HttpLocalApi, 'baseUrl' | 'routeFetch'>;
  auth: AuthRecovery;
  currentToken: () => string;
  identity: AmbientArtworkIdentity;
  isCurrent: () => boolean;
}) {
  const {identity} = options;
  const runtime = protectedImageRuntime({...options, scopeKey: JSON.stringify([identity.serverId, identity.viewerId, identity.authorityKey])}, request =>
    ambientArtworkSource(request.url, identity) ? request.kind === 'ambient' ? {maxBytes: 20000, maxEdge: 64} : {maxBytes: 12 * 1024 * 1024} : undefined);
  return {
    ...runtime,
    foreground(url: string | undefined, pixels: number, aspect: number, request?: Omit<ArtworkRequestOptions, 'signal'>) {
      const selected = foregroundArtwork(url, pixels, aspect, identity);
      return selected ? runtime.source(selected, request) : undefined;
    },
    ambient(url: string | undefined, request?: Omit<ArtworkRequestOptions, 'signal'>) {
      const selected = ambientArtworkSource(url, identity);
      return selected ? runtime.ambient(selected, request) : undefined;
    },
  };
}
export type ShellArtwork = ReturnType<typeof authenticatedArtwork>;
