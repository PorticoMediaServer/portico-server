import type {
  HostedTerminalMutationDurabilityAdapter,
  PendingHostedTerminalMutation,
} from "@porticomediaserver/client-core";

const STORAGE_KEY = "portico.hosted-terminal-mutations.v1";
const MAX_PENDING = 64;
let mutation: Promise<void> = Promise.resolve();

function storage(): Storage {
  if (typeof globalThis.localStorage === "undefined")
    throw new Error("Durable browser storage is unavailable.");
  return globalThis.localStorage;
}

function read(): PendingHostedTerminalMutation[] {
  const raw = storage().getItem(STORAGE_KEY);
  if (!raw) return [];
  const parsed = JSON.parse(raw) as unknown;
  return Array.isArray(parsed)
    ? (parsed as PendingHostedTerminalMutation[]).slice(-MAX_PENDING)
    : [];
}

function enqueue(operation: () => void): Promise<void> {
  const current = mutation.then(operation, operation);
  mutation = current.catch(() => undefined);
  return current;
}

export const browserHostedTerminalMutationDurability: HostedTerminalMutationDurabilityAdapter = {
  load: async () => {
    await mutation;
    return read();
  },
  save: (record) =>
    enqueue(() => {
      const records = [
        ...read().filter(
          (candidate) => candidate.idempotencyKey !== record.idempotencyKey,
        ),
        record,
      ].slice(-MAX_PENDING);
      storage().setItem(STORAGE_KEY, JSON.stringify(records));
    }),
  remove: (idempotencyKey) =>
    enqueue(() => {
      const records = read().filter(
        (candidate) => candidate.idempotencyKey !== idempotencyKey,
      );
      if (records.length === 0) storage().removeItem(STORAGE_KEY);
      else storage().setItem(STORAGE_KEY, JSON.stringify(records));
    }),
};
