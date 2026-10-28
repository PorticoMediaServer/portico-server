import {ReadingError, type ReadingClient} from '@portico/client-core';

/** Only the genuine reading resource can classify an item as Read. Audiobook
 * ContentEntry.kind='book' never participates in this decision. */
export async function readItemEligibility(client: ReadingClient, itemId: string, signal?: AbortSignal) {
  try {
    const result = await client.versions(itemId, signal);
    return result.versions.length ? Object.freeze({itemId, versions: result.versions}) : null;
  } catch (error) {if (error instanceof ReadingError && error.status === 404) return null; throw error;}
}
export function readDestination(itemId: string, sourceId: string) {
  return {to: '/read/$itemId' as const, params: {itemId}, search: {sourceId}};
}
export type ReadItemEligibility = NonNullable<Awaited<ReturnType<typeof readItemEligibility>>>;
