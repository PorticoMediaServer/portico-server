/**
 * Captured focus context for a real keyboard removal (Root71 UW-03).
 *
 * The caller supplies the bounded root, the ordered focus targets and the
 * surviving section/page starting controls. Nothing here enumerates a library,
 * guesses a route or falls back to the document body. A capture is only valid
 * for the DOM it references; a detached or replaced root returns no target.
 */
export type RemovalFocusStart = Readonly<{node: HTMLElement; before: boolean}>;

export type RemovalFocusCapture = Readonly<{
  root: HTMLElement;
  /** Ordered actual focus targets in the bounded region. */
  items: readonly HTMLElement[];
  /** The actual target focused when the removal began. */
  focused: HTMLElement;
  focusedIndex: number;
  /** The focused item's section/page starting control, when the caller has one. */
  section: HTMLElement | null;
  /** Surviving section/page starts in document order, with their side of the focus. */
  starts: readonly RemovalFocusStart[];
}>;

export type RemovalFocusSource = 'next' | 'previous' | 'section' | 'start';

export type RemovalFocusResult =
  | Readonly<{action: 'focus'; node: HTMLElement; source: RemovalFocusSource}>
  /** The focused item was removed, but every captured fallback is gone or ineligible. */
  | Readonly<{action: 'none'}>
  /** The focused item was not removed; unrelated or pointer-only work never moves focus. */
  | Readonly<{action: 'ignore'}>;

export type RemovalFocusInput = Readonly<{
  root: HTMLElement | null | undefined;
  items: readonly (HTMLElement | null | undefined)[];
  /** Defaults to the root document's active element. */
  focused?: HTMLElement | null;
  section?: HTMLElement | null;
  starts?: readonly (HTMLElement | null | undefined)[];
}>;

const DOCUMENT_POSITION_PRECEDING = 2;

function precedes(node: HTMLElement, reference: HTMLElement): boolean {
  const position = reference.compareDocumentPosition?.(node);
  return typeof position === 'number' && (position & DOCUMENT_POSITION_PRECEDING) !== 0;
}

function inRemoved(node: HTMLElement, removed: readonly (HTMLElement | null | undefined)[]): boolean {
  return removed.some(value => !!value && (value === node || value.contains(node)));
}

/** Connected, inside the captured root, not inert/hidden/aria-hidden and enabled. */
function eligible(node: HTMLElement | null | undefined, root: HTMLElement, removed: readonly (HTMLElement | null | undefined)[]): node is HTMLElement {
  if (!node || !node.isConnected || !root.contains(node) || inRemoved(node, removed)) return false;
  for (let current: HTMLElement | null = node; current; current = current.parentElement) {
    if (current.inert || current.hidden || current.getAttribute('aria-hidden') === 'true' || current.getAttribute('aria-disabled') === 'true') return false;
  }
  if ((node as HTMLButtonElement).disabled) return false;
  const rects = node.getClientRects?.();
  if (rects && rects.length === 0) return false;
  return typeof node.focus === 'function';
}

/**
 * Capture the bounded context before the outgoing node becomes inert. Returns
 * null rather than inventing context when the root is detached or the focused
 * target is not one of the supplied ordered items.
 */
export function captureRemovalFocus(input: RemovalFocusInput): RemovalFocusCapture | null {
  const {root} = input;
  if (!root?.isConnected) return null;
  const focused = input.focused ?? (root.ownerDocument?.activeElement as HTMLElement | null | undefined) ?? null;
  if (!focused || !focused.isConnected || !root.contains(focused)) return null;
  const items = [...new Set(input.items.filter((node): node is HTMLElement => !!node && node.isConnected && root.contains(node)))];
  const focusedIndex = items.indexOf(focused);
  if (focusedIndex < 0) return null;
  const section = input.section && input.section.isConnected && root.contains(input.section) ? input.section : null;
  const starts = (input.starts ?? [])
    .filter((node): node is HTMLElement => !!node && node.isConnected && root.contains(node))
    .map(node => ({node, before: precedes(node, focused)}));
  return {root, items, focused, focusedIndex, section, starts};
}

/**
 * Pure target decision: next eligible item, previous, the focused section's
 * start, then the nearest surviving start (preceding before following).
 */
export function removalFocusTarget(capture: RemovalFocusCapture | null, removed: readonly (HTMLElement | null | undefined)[]): RemovalFocusResult {
  if (!capture || !capture.root.isConnected) return {action: 'none'};
  if (!inRemoved(capture.focused, removed)) return {action: 'ignore'};
  // A target moved out of the captured root is no longer ours to focus.
  if (capture.focused.isConnected && !capture.root.contains(capture.focused)) return {action: 'ignore'};
  for (let index = capture.focusedIndex + 1; index < capture.items.length; index += 1) {
    if (eligible(capture.items[index], capture.root, removed)) return {action: 'focus', node: capture.items[index], source: 'next'};
  }
  for (let index = capture.focusedIndex - 1; index >= 0; index -= 1) {
    if (eligible(capture.items[index], capture.root, removed)) return {action: 'focus', node: capture.items[index], source: 'previous'};
  }
  if (eligible(capture.section, capture.root, removed)) return {action: 'focus', node: capture.section, source: 'section'};
  for (let index = capture.starts.length - 1; index >= 0; index -= 1) {
    const start = capture.starts[index];
    if (start.before && eligible(start.node, capture.root, removed)) return {action: 'focus', node: start.node, source: 'start'};
  }
  for (const start of capture.starts) {
    if (!start.before && eligible(start.node, capture.root, removed)) return {action: 'focus', node: start.node, source: 'start'};
  }
  return {action: 'none'};
}

/**
 * Apply/cancel half. Focus moves only for the captured focused target, only
 * inside the still-connected original root. Anything else is an explicit no-op.
 * The caller runs this before marking the outgoing node inert; this helper never
 * queries a successor document, route or fresh selector.
 */
export function applyRemovalFocus(capture: RemovalFocusCapture | null, removed: readonly (HTMLElement | null | undefined)[]): RemovalFocusResult {
  const result = removalFocusTarget(capture, removed);
  if (result.action !== 'focus') return result;
  if (!capture?.root.isConnected || !eligible(result.node, capture.root, removed)) return {action: 'none'};
  try {
    result.node.focus({preventScroll: true});
  } catch {
    return {action: 'none'};
  }
  return result;
}
