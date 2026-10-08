/**
 * X-12: Remove from Continue Watching, on the server's own dismissal (api/home.md, "Continue
 * Watching dismissal"): `PUT /v1/items/{id}/personal-state {continueDismissed: true}` takes the
 * title off this profile's Continue Watching without marking it watched or touching its resume
 * point; `false` puts it back (Undo). The next playback of the title makes it eligible again.
 *
 * The mutation carries the item's personal revision. A caller that has it (an open title's detail)
 * passes it; otherwise, and once after a `personal_state_conflict`, the current revision is read
 * from the title's detail. Each attempt is a fresh operation.
 */
type Api = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};

export type ContinueDismissalInput = Readonly<{
  itemId: string;
  dismissed: boolean;
  /** The item's current personal revision, when the caller already holds it. */
  revision?: number;
  /** A new operation ID per attempt (`^[A-Za-z0-9_-]{1,128}$`; a UUID fits). */
  operationId: () => string | Promise<string>;
  signal?: AbortSignal;
}>;

const operation = /^[A-Za-z0-9_-]{1,128}$/;
const revisionValue = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;
const itemPath = (itemId: string) => '/v1/items/' + encodeURIComponent(itemId);

/** The personal revision in a detail or a personal-state receipt (`{personal: {revision}}`). */
export function personalRevision(raw: unknown): number | undefined {
  const r = (raw as {personal?: {revision?: unknown}} | null)?.personal?.revision;
  return revisionValue(r) ? r : undefined;
}

async function currentRevision(api: Api, itemId: string, signal?: AbortSignal): Promise<number> {
  const r = personalRevision(await api.request<unknown>(itemPath(itemId) + '/detail', 'GET', undefined, signal));
  if (r === undefined) throw Object.assign(new Error('The title’s personal state could not be read.'), {code: 'invalid_detail'});
  return r;
}

/**
 * Dismisses (or restores) one title on this profile's Continue Watching. Resolves with the
 * personal revision after the change, which an Undo passes back.
 */
export async function setContinueDismissed(api: Api, input: ContinueDismissalInput): Promise<{revision: number}> {
  if (!input.itemId) throw new Error('A title is required.');
  let revision = revisionValue(input.revision) ? input.revision : await currentRevision(api, input.itemId, input.signal);
  for (let attempt = 0; ; attempt++) {
    const operationId = String(await input.operationId());
    if (!operation.test(operationId)) throw new Error('Invalid operation ID.');
    try {
      const receipt = await api.request<unknown>(itemPath(input.itemId) + '/personal-state', 'PUT', {operationId, expectedRevision: revision, continueDismissed: input.dismissed}, input.signal);
      return {revision: personalRevision(receipt) ?? revision + 1};
    } catch (error) {
      // The title changed elsewhere since the revision was read (progress, watchlist…): read it
      // again and retry once. Anything else is the caller's to show.
      if (attempt > 0 || (error as {code?: unknown} | null)?.code !== 'personal_state_conflict') throw error;
      revision = await currentRevision(api, input.itemId, input.signal);
    }
  }
}
