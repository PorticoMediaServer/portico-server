import type {ReaderRange, ReaderSource} from './ports.ts';
import {ReaderError, throwIfAborted} from './errors.ts';
export const MAX_READER_RANGE_BYTES = 16 * 1024 * 1024;
export function validateSource(source: ReaderSource): void {
  if (!source.id || !Number.isSafeInteger(source.size) || source.size <= 0 || typeof source.read !== 'function')
    throw new ReaderError('invalid_source');
}
export async function readRange(source: ReaderSource, range: ReaderRange, signal: AbortSignal): Promise<Uint8Array> {
  throwIfAborted(signal); validateSource(source);
  const {start, endExclusive} = range;
  if (!Number.isSafeInteger(start) || !Number.isSafeInteger(endExclusive) || start < 0 || endExclusive < start || endExclusive > source.size)
    throw new ReaderError('invalid_source');
  if (endExclusive - start > MAX_READER_RANGE_BYTES) throw new ReaderError('limit_exceeded');
  const bytes = await source.read(range, signal);
  throwIfAborted(signal);
  if (!(bytes instanceof Uint8Array) || bytes.byteLength !== endExclusive - start) throw new ReaderError('invalid_source');
  return bytes;
}
export function sourceFromBlob(blob: Blob, id: string): ReaderSource {
  const source: ReaderSource = Object.freeze({id, size: blob.size,
    async read(range: ReaderRange, signal: AbortSignal) {
      throwIfAborted(signal);
      const bytes = new Uint8Array(await blob.slice(range.start, range.endExclusive).arrayBuffer());
      throwIfAborted(signal); return bytes;
    }});
  validateSource(source); return source;
}
