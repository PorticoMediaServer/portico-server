import {decodeGetCapabilitiesResponse} from '../../../../packages/contracts/src/decoders.ts';
import {routePath} from '../../../../packages/contracts/src/routes.ts';

/** The captured shell's own API request. */
export type CapabilityRequest = (path: string, method: 'GET', body: undefined, signal: AbortSignal) => Promise<unknown>;

/**
 * The server's published protocol features, read from GET /v1/capabilities. Absent on older
 * servers, so a feature is only ever acted on when the descriptor names it. No readiness,
 * permission or count is inferred from a flag.
 */
export async function readServerFeatures(request: CapabilityRequest, signal: AbortSignal): Promise<Readonly<Record<string, string>>> {
  const response = decodeGetCapabilitiesResponse(await request(routePath('get_capabilities'), 'GET', undefined, signal));
  const features: Record<string, string> = {};
  for (const [name, value] of Object.entries(response.features ?? {})) {
    if (typeof value === 'string' && /^[a-z0-9_]{1,64}$/.test(name) && value.length > 0 && value.length <= 64 && !/[\x00-\x1f\x7f]/.test(value)) features[name] = value;
  }
  return Object.freeze(features);
}

/** The opt-in reading Discover series row is requested only where the descriptor says it exists. */
export function readingSeriesDiscoveryEnabled(features: Readonly<Record<string, string>>): boolean {
  return features.reading_discover_series === 'enabled';
}
