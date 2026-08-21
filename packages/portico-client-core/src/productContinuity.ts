/**
 * Platform-neutral product continuity semantics.
 *
 * These states describe what a person should experience, not the transport
 * currently doing work. DNS probes, token rotation, event reconnects, and
 * route selection remain implementation details until they produce a
 * terminal product outcome.
 */
export type ProductContinuityPhase =
  | "restoring"
  | "connecting"
  | "ready"
  | "refreshing"
  | "stale"
  | "unavailable"
  | "unauthorized"
  | "security-blocked";

export type ProductContinuityFailureKind =
  | "transient"
  | "unavailable"
  | "unauthorized"
  | "security-blocked";

export interface ProductContinuityFailure {
  kind: ProductContinuityFailureKind;
  /** Stable product-language identifier; never raw transport prose. */
  messageId?: string;
  cause?: unknown;
}

export type ProductContinuityPresentation = "content" | "reserved" | "terminal-error";

export interface ProductContinuityInput<T> {
  data?: T;
  restoring?: boolean;
  connecting?: boolean;
  refreshing?: boolean;
  stale?: boolean;
  failure?: ProductContinuityFailure;
}

export interface ProductContinuityState<T> {
  phase: ProductContinuityPhase;
  presentation: ProductContinuityPresentation;
  data?: T;
  failure?: ProductContinuityFailure;
  /** Terminal failures may be reported without replacing retained content. */
  showFailure: boolean;
}

/**
 * Projects platform/network state into consistent product behavior.
 * Successful data is retained during refresh and ordinary transport failure;
 * fail-closed identity/security failures never retain a previous scope.
 */
export function resolveProductContinuity<T>(input: ProductContinuityInput<T>): ProductContinuityState<T> {
  const hasData = input.data !== undefined;
  const failure = input.failure;

  if (failure?.kind === "security-blocked") {
    return { phase: "security-blocked", presentation: "terminal-error", failure, showFailure: true };
  }
  if (failure?.kind === "unauthorized") {
    return { phase: "unauthorized", presentation: "terminal-error", failure, showFailure: true };
  }

  if (hasData) {
    if (failure?.kind === "unavailable") {
      return { phase: "unavailable", presentation: "content", data: input.data, failure, showFailure: true };
    }
    if (input.refreshing) {
      return { phase: "refreshing", presentation: "content", data: input.data, failure, showFailure: false };
    }
    if (input.stale || failure?.kind === "transient") {
      return { phase: "stale", presentation: "content", data: input.data, failure, showFailure: false };
    }
    return { phase: "ready", presentation: "content", data: input.data, showFailure: false };
  }

  if (failure?.kind === "unavailable") {
    return { phase: "unavailable", presentation: "terminal-error", failure, showFailure: true };
  }
  if (input.connecting || failure?.kind === "transient") {
    return { phase: "connecting", presentation: "reserved", failure, showFailure: false };
  }
  return { phase: "restoring", presentation: "reserved", failure, showFailure: false };
}

export type ReservedSlotResolution = "unresolved" | "ready" | "empty" | "failed";

export interface ReservedSurfaceSlot<TDescriptor, TData> {
  id: string;
  descriptor: TDescriptor;
  resolution: ReservedSlotResolution;
  data?: TData;
}

function normalizedSlotId(value: string): string {
  const id = value.trim();
  if (!id || id.length > 256) throw new TypeError("reserved surface slot id is invalid");
  return id;
}

/**
 * Reconciles an authoritative, customizable descriptor order while retaining
 * already resolved slot contents. Every advertised descriptor remains
 * represented until its current resolution is known.
 */
export function reserveOrderedSurfaceSlots<TDescriptor, TData>(
  descriptors: readonly TDescriptor[],
  idOf: (descriptor: TDescriptor) => string,
  previous: readonly ReservedSurfaceSlot<TDescriptor, TData>[] = []
): ReservedSurfaceSlot<TDescriptor, TData>[] {
  const previousById = new Map(previous.map(slot => [normalizedSlotId(slot.id), slot]));
  const seen = new Set<string>();
  return descriptors.map(descriptor => {
    const id = normalizedSlotId(idOf(descriptor));
    if (seen.has(id)) throw new TypeError(`duplicate reserved surface slot id: ${id}`);
    seen.add(id);
    const retained = previousById.get(id);
    return retained
      ? { ...retained, id, descriptor }
      : { id, descriptor, resolution: "unresolved" };
  });
}

/** Resolves one slot without changing authoritative order or neighboring geometry. */
export function resolveReservedSurfaceSlot<TDescriptor, TData>(
  slots: readonly ReservedSurfaceSlot<TDescriptor, TData>[],
  idValue: string,
  resolution: Exclude<ReservedSlotResolution, "unresolved">,
  data?: TData
): ReservedSurfaceSlot<TDescriptor, TData>[] {
  const id = normalizedSlotId(idValue);
  let found = false;
  const next = slots.map(slot => {
    if (slot.id !== id) return slot;
    found = true;
    if (resolution === "ready" && data === undefined) throw new TypeError("ready reserved surface slots require data");
    return resolution === "ready"
      ? { ...slot, resolution, data }
      : { id: slot.id, descriptor: slot.descriptor, resolution };
  });
  if (!found) throw new TypeError(`reserved surface slot is not advertised: ${id}`);
  return next;
}
