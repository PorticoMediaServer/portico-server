import {retryAfterSecondsFrom, type AuthRecovery, type HttpLocalApi} from '@core/index.ts';
import {ambientArtworkSource, artworkWidthBucket, type AmbientArtworkIdentity, type ArtworkFailure, type ArtworkRequestOptions} from '@core/artwork/index.ts';
import {createArtworkRuntime, type ArtworkLoadRequest} from '../kit/artwork/source';
import type {ArtworkImage} from '../kit/artwork/types';

export function foregroundArtwork(url: string | undefined, pixels: number, aspect: number, identity: AmbientArtworkIdentity) {
  // The accepted ambient parser also validates selected route, digest and query identity.
  if (!ambientArtworkSource(url, identity)) return undefined;
  const [path, query] = url!.split('?'), params = new URLSearchParams(query);
  params.delete('size'); params.set('w', String(artworkWidthBucket(pixels, {aspect}))); params.sort();
  const selected = path + '?' + params.toString();
  return {url: selected, cacheKey: JSON.stringify([identity.serverId, identity.viewerId, identity.authorityKey, selected])};
}
export function artworkFailure(error: unknown): ArtworkFailure {
  const failure = error as {status?: number; retryAfterSeconds?: number; name?: string};
  return {retryable: failure.name !== 'AbortError' && (failure.status === undefined || failure.status === 408 || failure.status === 429 || failure.status >= 500 || failure.status === 404 && failure.retryAfterSeconds !== undefined), retryAfterSeconds: failure.retryAfterSeconds};
}
async function imageBytes(response: Response, ceiling: number, signal: AbortSignal): Promise<Blob> {
  const declared = Number(response.headers.get('content-length'));
  if (declared > ceiling) { await response.body?.cancel(); throw Object.assign(new Error('Artwork exceeds its size bound.'), {status: 413}); }
  const type = response.headers.get('content-type')?.split(';')[0];
  if (!type || !['image/png', 'image/jpeg', 'image/webp', 'image/avif'].includes(type) || !response.body) {
    await response.body?.cancel(); throw Object.assign(new Error('Artwork is not an accepted image.'), {status: 415});
  }
  const reader = response.body.getReader(), chunks: Uint8Array<ArrayBuffer>[] = []; let count = 0;
  try {
    for (;;) {
      signal.throwIfAborted();
      const read = await reader.read(); if (read.done) break;
      count += read.value.byteLength;
      if (count > ceiling) throw Object.assign(new Error('Artwork exceeds its size bound.'), {status: 413});
      chunks.push(new Uint8Array(read.value));
    }
    return new Blob(chunks, {type});
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}

/** Real credentials go through the selected route transport, never an img URL or localStorage. */
export function authenticatedArtwork(options: {
  api: Pick<HttpLocalApi, 'baseUrl' | 'routeFetch'>;
  auth: AuthRecovery;
  currentToken: () => string;
  identity: AmbientArtworkIdentity;
  isCurrent: () => boolean;
}) {
  const {api, auth, identity, isCurrent} = options;
  const runtime = createArtworkRuntime({
    scopeKey: JSON.stringify([identity.serverId, identity.viewerId, identity.authorityKey]),
    classify: artworkFailure,
    load: async (request: ArtworkLoadRequest, signal: AbortSignal): Promise<ArtworkImage> => {
      if (!isCurrent() || !ambientArtworkSource(request.url, identity)) throw Object.assign(new Error('Artwork authority changed.'), {status: 403});
      const token = await auth.token(options.currentToken());
      signal.throwIfAborted();
      if (!isCurrent()) throw Object.assign(new Error('Artwork authority changed.'), {status: 403});
      const fetchImage = (access: string) => api.routeFetch(api.baseUrl + request.url, {method: 'GET', signal, redirect: 'error', credentials: 'omit', referrerPolicy: 'no-referrer', headers: {Authorization: 'Bearer ' + access, Accept: 'image/avif,image/webp,image/png,image/jpeg'}});
      let response = await fetchImage(token);
      if (response.status === 401 && !signal.aborted) {
        await response.body?.cancel();
        const renewed = await auth.recover(token);
        if (!renewed || !isCurrent()) throw Object.assign(new Error('Artwork sign-in ended.'), {status: 401});
        response = await fetchImage(renewed);
      }
      if (!response.ok) {
        const failure = {status: response.status, retryAfterSeconds: retryAfterSecondsFrom(response.headers.get('Retry-After'))};
        if (response.status === 403) void auth.permissionDenied?.();
        await response.body?.cancel(); throw Object.assign(new Error('Artwork could not be loaded.'), failure);
      }
      const blob = await imageBytes(response, request.kind === 'ambient' ? 20000 : 12 * 1024 * 1024, signal);
      let width: number, height: number;
      try {
        const image = await createImageBitmap(blob);
        width = image.width; height = image.height; image.close();
      } catch { throw Object.assign(new Error('Artwork could not be decoded.'), {status: 422}); }
      if (request.kind === 'ambient' && Math.max(width, height) > 64) throw Object.assign(new Error('Ambient artwork exceeds its dimensions.'), {status: 422});
      signal.throwIfAborted();
      if (!isCurrent()) throw Object.assign(new Error('Artwork authority changed.'), {status: 403});
      return {src: URL.createObjectURL(blob), bytes: blob.size, width, height};
    },
    release: image => URL.revokeObjectURL(image.src),
  });
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
