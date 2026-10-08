/** Only known server artwork routes support the thumbnail representation. */
export function thumbnailArtworkPath(path?: string): string | undefined {
  if (!path) return path;
  const url = new URL(path, 'https://artwork.invalid');
  if (/^\/v1\/(?:items|metadata\/(?:item|show|season|album|artist|book))\/[^/]+\/art\/[^/]+$/.test(url.pathname) && !url.searchParams.has('candidate')) {
    url.searchParams.set('size', 'thumbnail');
    return path.startsWith('/') ? url.pathname + url.search + url.hash : url.href;
  }
  return path;
}

/**
 * The artwork variant for a frame of this rendered size: one server variant
 * per card size, so the browser cache hits across screens. The server
 * takes `w=400|800|1920`, the longest edge, never enlarged; selected artwork
 * URLs already carry `v=<digest>` and are immutable. Servers without the
 * variants ignore `w`, so up to 800 also keeps `size=thumbnail` (their only
 * small variant) until every server has them.
 */
export function sizedArtworkPath(path: string | undefined, bucket: number): string | undefined {
  if (!path) return path;
  const base = bucket <= 800 ? thumbnailArtworkPath(path)! : path;
  const url = new URL(base, 'https://artwork.invalid');
  if (!/^\/v1\//.test(url.pathname) || url.searchParams.has('candidate')) return base;
  url.searchParams.set('w', String(bucket));
  return base.startsWith('/') ? url.pathname + url.search + url.hash : url.href;
}
