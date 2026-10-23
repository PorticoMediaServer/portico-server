import type {RouteResponse} from '../../contracts/src/routes.ts';
import type {schemas} from '../../contracts/src/schemas.ts';

/** Library identity only. `book` content entries and progress still mean
 * audiobooks; Read has no published content/position producer. */
export type ReadLibraryKind = 'book' | 'comic';
type CapabilityFields = typeof schemas.GetLibrariesResponse.properties.items.items.properties.capabilities.properties;
/** Narrow the generated structural DTO with its actual published enum schema.
 * The raw decoder's enum validation is separate from this checked boundary. */
export type LibraryMediaCapabilities = Readonly<Omit<NonNullable<RouteResponse<'get_libraries'>['items'][number]['capabilities']>, 'reading' | 'cardShape'> & {
  reading: CapabilityFields['reading']['enum'][number];
  cardShape: CapabilityFields['cardShape']['enum'][number];
}>;

/** Read support is a server fact. Missing or malformed facts confer no media
 * capability; an unknown library can still be listed by the tolerant directory. */
export function parseLibraryMediaCapabilities(raw: unknown): LibraryMediaCapabilities | undefined {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return undefined;
  const v = raw as Record<string, unknown>;
  if (typeof v.tv !== 'boolean' || v.reading !== false || !['poster', 'square', 'portrait'].includes(v.cardShape as string)) return undefined;
  return Object.freeze({tv: v.tv, reading: false, cardShape: v.cardShape as LibraryMediaCapabilities['cardShape']});
}

/** TV/Roku navigation consumes the published flag; no library-kind allowlist. */
export function libraryVisibleOnTV(library: Readonly<{capabilities?: LibraryMediaCapabilities}>): boolean {
  return library.capabilities?.tv === true;
}
