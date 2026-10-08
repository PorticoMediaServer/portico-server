/**
 * Size-aware artwork keys: request exactly one server variant per card size,
 * so the HTTP/browser cache hits across screens.
 *
 * The server encodes display artwork by its LONG edge (server/
 * internal/metadata/artwork_encoding.go: 400 and 1920 today; lane B adds 800).
 * A 2:3 poster 180 px wide at 2× needs a 540 px long edge, so the 800 variant.
 *
 * URL contract (lane B, metadata.md): `?w=400|800|1920` is the maximum long
 * edge (never enlarged; default 1920), plus `v=<digest>` on versioned URLs,
 * which are immutable and cacheable forever. While a variant is being made the
 * server answers 404 with `Retry-After: 5`, which the scheduler retries.
 */
export const artworkSizeBuckets: readonly number[] = [400, 800, 1920];

export type ArtworkBucketOptions = Readonly<{
  /** Rendered width ÷ height (poster 2/3, landscape 16/9, square 1). Default 1. */
  aspect?: number;
  /** The server's variants, ascending. Default `artworkSizeBuckets`. */
  buckets?: readonly number[];
  /** A variant may be up to this much smaller than the rendered pixels (default 1.1 = 10%). */
  tolerance?: number;
}>;

/**
 * The variant to request for artwork rendered `devicePixelWidth` wide
 * (CSS width × devicePixelRatio, or points × screen scale).
 */
export function artworkWidthBucket(devicePixelWidth: number, options: ArtworkBucketOptions = {}): number {
  const buckets = options.buckets?.length ? options.buckets : artworkSizeBuckets;
  const aspect = options.aspect && options.aspect > 0 && Number.isFinite(options.aspect) ? options.aspect : 1;
  if (!(devicePixelWidth > 0) || !Number.isFinite(devicePixelWidth)) return buckets[0]!;
  const longEdge = aspect < 1 ? devicePixelWidth / aspect : devicePixelWidth;
  const needed = longEdge / Math.max(1, options.tolerance ?? 1.1);
  for (const bucket of buckets) if (bucket >= needed) return bucket;
  return buckets[buckets.length - 1]!;
}
