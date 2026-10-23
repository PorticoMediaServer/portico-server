import {retryAfterSecondsFrom, type AuthRecovery, type HttpLocalApi} from '@core/index.ts';
import type {ArtworkFailure} from '@core/artwork/index.ts';
import {createArtworkRuntime, type ArtworkLoadRequest} from '../kit/artwork/source';

export function artworkFailure(error: unknown): ArtworkFailure {
  const failure = error as {status?: number; retryAfterSeconds?: number; name?: string};
  return {retryable: failure.name !== 'AbortError' && (failure.status === undefined || failure.status === 408 || failure.status === 429 || failure.status >= 500 || failure.status === 404 && failure.retryAfterSeconds !== undefined), retryAfterSeconds: failure.retryAfterSeconds};
}
async function imageBytes(response: Response, ceiling: number, signal: AbortSignal): Promise<Blob> {
  const declared = Number(response.headers.get('content-length'));
  if (declared > ceiling) {await response.body?.cancel();throw Object.assign(new Error('Artwork exceeds its size bound.'), {status: 413});}
  const type = response.headers.get('content-type')?.split(';')[0];
  if (!type || !['image/png', 'image/jpeg', 'image/webp', 'image/avif'].includes(type) || !response.body) {
    await response.body?.cancel();throw Object.assign(new Error('Artwork is not an accepted image.'), {status: 415});
  }
  const reader = response.body.getReader(), chunks: Uint8Array<ArrayBuffer>[] = [];let count = 0;
  try {
    for (;;) {
      signal.throwIfAborted();
      const read = await reader.read();if (read.done) break;
      count += read.value.byteLength;
      if (count > ceiling) throw Object.assign(new Error('Artwork exceeds its size bound.'), {status: 413});
      chunks.push(new Uint8Array(read.value));
    }
    return new Blob(chunks, {type});
  } finally {await reader.cancel().catch(() => {});reader.releaseLock();}
}
export type ProtectedImageOptions = Readonly<{
  api: Pick<HttpLocalApi, 'baseUrl' | 'routeFetch'>;auth: AuthRecovery;
  currentToken: () => string;isCurrent: () => boolean;scopeKey: string;
}>;
export type ProtectedImageBounds = Readonly<{maxBytes: number;maxEdge?: number;width?: number;height?: number}>;

/** Shared credential, retry, byte and blob lifetime. Each resource owner must validate its exact URL. */
export function protectedImageRuntime(options: ProtectedImageOptions, validate: (request: ArtworkLoadRequest) => ProtectedImageBounds | undefined) {
  const {api, auth, isCurrent} = options;
  return createArtworkRuntime({
    scopeKey: options.scopeKey,classify: artworkFailure,
    load: async (request, signal) => {
      const bounds = validate(request);
      if (!isCurrent() || !bounds || !request.url.startsWith('/v1/') || request.url.includes('\\') || !Number.isSafeInteger(bounds.maxBytes) || bounds.maxBytes < 1 || bounds.maxBytes > 12 * 1024 * 1024) throw Object.assign(new Error('Artwork authority changed.'), {status: 403});
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
        await response.body?.cancel();throw Object.assign(new Error('Artwork could not be loaded.'), failure);
      }
      const blob = await imageBytes(response, bounds.maxBytes, signal);
      let width: number, height: number;
      try {const image = await createImageBitmap(blob);width = image.width;height = image.height;image.close();}
      catch {throw Object.assign(new Error('Artwork could not be decoded.'), {status: 422});}
      if (bounds.maxEdge !== undefined && Math.max(width, height) > bounds.maxEdge || bounds.width !== undefined && width !== bounds.width || bounds.height !== undefined && height !== bounds.height) throw Object.assign(new Error('Artwork exceeds its dimensions.'), {status: 422});
      signal.throwIfAborted();
      if (!isCurrent() || !validate(request)) throw Object.assign(new Error('Artwork authority changed.'), {status: 403});
      return {src: URL.createObjectURL(blob), bytes: blob.size, width, height};
    },
    release: image => URL.revokeObjectURL(image.src),
  });
}
