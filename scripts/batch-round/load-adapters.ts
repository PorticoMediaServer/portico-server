/** Operator driver adapters only. No product API or persistence semantics are replaced. */
import {open, lstat, rename, unlink} from 'node:fs/promises';
import {constants} from 'node:fs';
import {join} from 'node:path';
import {randomUUID} from 'node:crypto';
import {HttpLocalApi, type LocalSession} from '../../packages/client-core/src/index.ts';
import {emptyUserMutationState, validateUserMutationState, parseUserMutationReceipt,
  USER_MUTATION_MAX_BYTES, type DurableUserMutationStore, type UserMutationState} from '../../packages/client-core/src/user-mutations.ts';
import {PERSONAL_BATCH_PATH, personalBatchBody, parsePersonalBatchReceipt, type PersonalBatchItem} from '../../packages/client-core/src/personal-saved.ts';
import {SessionsClient} from '../../packages/client-core/src/playback-v1/sessions.ts';
import type {V1Http} from '../../packages/client-core/src/playback-v1/http.ts';

/** Single JS owner: CAS has no await between comparison and replacement. Bounded by core validation. */
export class MemoryMutationStore implements DurableUserMutationStore {
  private state = emptyUserMutationState();
  async read() {return structuredClone(this.state);}
  async compareAndSwap(revision: number, next: UserMutationState) {
    if (this.state.revision !== revision) return false;
    if (next.revision !== revision + 1) throw new Error('invalid_store_revision');
    this.state = structuredClone(validateUserMutationState(next)); return true;
  }
}
async function privateDirectory(path: string) {
  const stat = await lstat(path);
  if (!stat.isDirectory() || stat.isSymbolicLink() || stat.uid !== process.getuid?.() || (stat.mode & 0o777) !== 0o700)
    throw new Error('mutation_directory_requires_owned_0700');
}
/** One exclusive driver owns this directory. A crash leaves the lock for operator review;
 * never remove it automatically or discard pending records to make a new run start. */
export async function lockMutationDirectory(path: string) {
  await privateDirectory(path);
  const lockPath = join(path, 'driver.lock');
  const lock = await open(lockPath, constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY | constants.O_NOFOLLOW, 0o600);
  await lock.writeFile(String(process.pid)); await lock.sync();
  return async () => {await lock.close(); await unlink(lockPath);};
}
/** Atomic, fsynced replacement under the driver directory lock. No tokens are stored. */
export class FileMutationStore implements DurableUserMutationStore {
  private queue: Promise<unknown> = Promise.resolve();
  constructor(private path: string) {}
  private async readFile() {
    let file;
    try {file = await open(this.path, constants.O_RDONLY | constants.O_NOFOLLOW);}
    catch (error) {if ((error as NodeJS.ErrnoException).code === 'ENOENT') return emptyUserMutationState(); throw error;}
    try {
      const stat = await file.stat();
      if (!stat.isFile() || stat.uid !== process.getuid?.() || (stat.mode & 0o777) !== 0o600 || stat.size > USER_MUTATION_MAX_BYTES)
        throw new Error('invalid_mutation_store_file');
      return structuredClone(validateUserMutationState(JSON.parse(await file.readFile('utf8'))));
    } finally {await file.close();}
  }
  async read() {await this.queue; return this.readFile();}
  compareAndSwap(revision: number, next: UserMutationState): Promise<boolean> {
    const captured = structuredClone(validateUserMutationState(next));
    const operation = this.queue.then(async () => {
      if ((await this.readFile()).revision !== revision) return false;
      if (captured.revision !== revision + 1) throw new Error('invalid_store_revision');
      const temporary = this.path + '.' + randomUUID() + '.tmp';
      const file = await open(temporary, constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY | constants.O_NOFOLLOW, 0o600);
      try {
        try {await file.writeFile(JSON.stringify(captured)); await file.sync();} finally {await file.close();}
        await rename(temporary, this.path);
        const directory = await open(join(this.path, '..'), constants.O_RDONLY | constants.O_DIRECTORY);
        try {await directory.sync();} finally {await directory.close();}
        return true;
      } finally {await unlink(temporary).catch(error => {if (error.code !== 'ENOENT') throw error;});}
    });
    this.queue = operation.catch(() => {}); return operation;
  }
}
export async function mutationClient(target: string, session: LocalSession, store: DurableUserMutationStore, transport: typeof fetch) {
  const api = new HttpLocalApi(target, session.accessToken, transport);
  api.setUserMutationSession(session);
  // Do not silently replay old work as part of a fresh measured workload. Operator recovery is separate.
  if ((await store.read()).entries.length) throw new Error('pending_mutations_require_operator_recovery');
  const coordinator = api.configureUserMutations(store);
  try {await api.awaitUserMutationsReady();} catch (error) {coordinator.dispose(); throw error;}
  const http: V1Http = {send: request => api.requestRaw(request.path, request.method, request),
    acknowledgeUserMutation: intent => api.acknowledgeUserMutation(intent)};
  return {api, coordinator, sessions: new SessionsClient(http), http};
}
/** Caller identity remains fixed across ambiguity. Only parsed completion is acknowledged. */
export async function savedBatch(api: HttpLocalApi, serverId: string, operationId: string, items: readonly PersonalBatchItem[], signal?: AbortSignal) {
  const raw = await api.request<unknown>(PERSONAL_BATCH_PATH, 'PUT', personalBatchBody(operationId, items), signal);
  const entry = await api.getUserMutationCoordinator()?.inspectIntent(operationId);
  if (!entry || entry.state !== 'confirmed' || !entry.receipt) throw new Error('saved_receipt_unconfirmed');
  const receipt = parseUserMutationReceipt(entry.receipt, entry);
  if (receipt.status !== 'applied') throw new Error('saved_receipt_failed');
  let updated: number | undefined;
  if (raw && typeof raw === 'object' && 'mutationReceipt' in raw) {
    // A recovered tiny receipt proves the operation, never historical per-item counts.
    const marker = parseUserMutationReceipt(raw.mutationReceipt, entry);
    if (JSON.stringify(marker) !== JSON.stringify(receipt)) throw new Error('saved_recovery_mismatch');
  } else {
    const batch = parsePersonalBatchReceipt(raw, serverId, items.map(item => item.itemId));
    if (batch.operationId !== operationId || batch.failed || batch.updated !== items.length) throw new Error('saved_receipt_failed');
    updated = batch.updated;
  }
  await api.acknowledgeUserMutation(operationId);
  return {updated, recovered: updated === undefined};
}
/** Freeze the first final position for this exact generation, including a failed retry. */
export function finalStop(sessions: SessionsClient, id: string, generation: number) {
  let position: number | undefined, accepted: Promise<void> | undefined;
  return (positionMs?: number, signal?: AbortSignal) => {
    position ??= Math.max(0, Math.round(positionMs ?? 0));
    if (accepted) return accepted;
    const attempt = sessions.stop(id, position, signal, generation);
    accepted = attempt;
    void attempt.catch(() => {if (accepted === attempt) accepted = undefined;});
    return attempt;
  };
}
