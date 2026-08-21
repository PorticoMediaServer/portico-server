import { type ReactNode, type RefObject, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

type Placement = 'bottom-start' | 'bottom-end' | 'top-start' | 'top-end' | 'right-start';

type Point = { left: number; top: number; maxHeight: number; anchorWidth: number };
export type OverlayDismissReason = 'escape' | 'outside' | 'focus-leave' | 'route-transition' | 'viewer-transition';
export const OVERLAY_DISMISS_EVENT = 'portico:overlay-dismiss';
export const VIEWER_TRANSITION_EVENT = 'portico:viewer-transition';

type InertSnapshot = { hadInert: boolean; ariaHidden: string | null };

const modalBackdrops: HTMLElement[] = [];
const inertSnapshots = new Map<HTMLElement, InertSnapshot>();
let bodyOverflowBeforeModal: string | undefined;

function setModalInert(element: HTMLElement) {
  if (!inertSnapshots.has(element)) {
    inertSnapshots.set(element, {
      hadInert: element.hasAttribute('inert'),
      ariaHidden: element.getAttribute('aria-hidden'),
    });
  }
  element.setAttribute('inert', '');
  element.setAttribute('aria-hidden', 'true');
}

function restoreModalInert(element: HTMLElement) {
  const snapshot = inertSnapshots.get(element);
  if (!snapshot) return;
  if (snapshot.hadInert) element.setAttribute('inert', '');
  else element.removeAttribute('inert');
  if (snapshot.ariaHidden === null) element.removeAttribute('aria-hidden');
  else element.setAttribute('aria-hidden', snapshot.ariaHidden);
  inertSnapshots.delete(element);
}

function syncModalEnvironment() {
  const top = modalBackdrops.at(-1);
  const root = overlayRoot();
  const shouldBeInert = new Set<HTMLElement>();

  if (top) {
    for (const child of document.body.children) {
      if (child instanceof HTMLElement && child !== root) shouldBeInert.add(child);
    }
    for (const backdrop of modalBackdrops) {
      if (backdrop !== top) shouldBeInert.add(backdrop);
    }
  }

  for (const element of [...inertSnapshots.keys()]) {
    if (!shouldBeInert.has(element)) restoreModalInert(element);
  }
  for (const element of shouldBeInert) setModalInert(element);
}

function acquireModal(backdrop: HTMLElement) {
  if (modalBackdrops.includes(backdrop)) return;
  if (modalBackdrops.length === 0) {
    bodyOverflowBeforeModal = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
  }
  modalBackdrops.push(backdrop);
  syncModalEnvironment();
}

function releaseModal(backdrop: HTMLElement) {
  const index = modalBackdrops.lastIndexOf(backdrop);
  if (index >= 0) modalBackdrops.splice(index, 1);
  syncModalEnvironment();
  if (modalBackdrops.length === 0 && bodyOverflowBeforeModal !== undefined) {
    document.body.style.overflow = bodyOverflowBeforeModal;
    bodyOverflowBeforeModal = undefined;
  }
}

function isTopModal(backdrop: HTMLElement | null) {
  return Boolean(backdrop && modalBackdrops.at(-1) === backdrop);
}

export function dismissOverlays(reason: Exclude<OverlayDismissReason, 'escape' | 'outside' | 'focus-leave'>) {
  window.dispatchEvent(new CustomEvent<OverlayDismissReason>(OVERLAY_DISMISS_EVENT, { detail: reason }));
}

function useExternalDismiss(onDismiss: (reason: OverlayDismissReason) => void) {
  const onDismissRef = useRef(onDismiss);
  useLayoutEffect(() => {
    onDismissRef.current = onDismiss;
  }, [onDismiss]);
  useEffect(() => {
    const dismiss = (event: Event) => onDismissRef.current((event as CustomEvent<OverlayDismissReason>).detail ?? 'route-transition');
    const viewerDismiss = () => onDismissRef.current('viewer-transition');
    window.addEventListener(OVERLAY_DISMISS_EVENT, dismiss);
    window.addEventListener(VIEWER_TRANSITION_EVENT, viewerDismiss);
    return () => {
      window.removeEventListener(OVERLAY_DISMISS_EVENT, dismiss);
      window.removeEventListener(VIEWER_TRANSITION_EVENT, viewerDismiss);
    };
  }, []);
}

function overlayRoot() {
  return document.getElementById('portico-overlays') ?? document.body;
}

function focusable(root: HTMLElement) {
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => !element.hasAttribute('hidden') && element.getAttribute('aria-hidden') !== 'true');
}

function compositeItems(root: HTMLElement, role: 'menu' | 'listbox') {
  const selector = role === 'listbox' ? '[role="option"]' : '[role="menuitem"], button:not([disabled]), a[href]';
  return Array.from(root.querySelectorAll<HTMLElement>(selector))
    .filter((element) => !element.hasAttribute('disabled') && !element.hasAttribute('hidden') && element.getAttribute('aria-hidden') !== 'true');
}

function calculatePosition(anchor: DOMRect, overlay: DOMRect, placement: Placement, gap: number): Point {
  const viewportPadding = 12;
  const visualViewport = window.visualViewport;
  const viewportLeft = visualViewport?.offsetLeft ?? 0;
  const viewportTop = visualViewport?.offsetTop ?? 0;
  const viewportWidth = visualViewport?.width ?? window.innerWidth;
  const viewportHeight = visualViewport?.height ?? window.innerHeight;
  const viewportRight = viewportLeft + viewportWidth;
  const viewportBottom = viewportTop + viewportHeight;
  const availableBelow = viewportBottom - anchor.bottom - gap - viewportPadding;
  const availableAbove = anchor.top - viewportTop - gap - viewportPadding;
  const explicitlyAbove = placement.startsWith('top');
  const placeAbove = explicitlyAbove || (placement.startsWith('bottom') && availableBelow < Math.min(overlay.height, 280) && availableAbove > availableBelow);

  let left = placement.endsWith('end') ? anchor.right - overlay.width : anchor.left;
  let top = placeAbove ? anchor.top - overlay.height - gap : anchor.bottom + gap;

  if (placement === 'right-start') {
    const rightFits = anchor.right + gap + overlay.width <= viewportRight - viewportPadding;
    left = rightFits ? anchor.right + gap : anchor.left - overlay.width - gap;
    top = anchor.top - 10;
  }

  left = Math.max(viewportLeft + viewportPadding, Math.min(left, viewportRight - overlay.width - viewportPadding));
  top = Math.max(viewportTop + viewportPadding, Math.min(top, viewportBottom - overlay.height - viewportPadding));

  return {
    left,
    top,
    maxHeight: Math.max(160, viewportBottom - top - viewportPadding),
    anchorWidth: anchor.width,
  };
}

export function AnchoredOverlay({
  id,
  anchorRef,
  returnFocusRef,
  children,
  className,
  onDismiss,
  placement = 'bottom-start',
  role,
  labelledBy,
  ariaLabel,
  matchAnchorWidth = false,
  minAnchorWidth = false,
  autoFocusComposite = true,
}: {
  id?: string;
  anchorRef: React.RefObject<HTMLElement | null>;
  returnFocusRef?: React.RefObject<HTMLElement | null>;
  children: ReactNode;
  className: string;
  onDismiss: (reason?: OverlayDismissReason) => void;
  placement?: Placement;
  role?: 'menu' | 'listbox' | 'dialog';
  labelledBy?: string;
  ariaLabel?: string;
  matchAnchorWidth?: boolean;
  minAnchorWidth?: boolean;
  autoFocusComposite?: boolean;
}) {
  const contentRef = useRef<HTMLDivElement>(null);
  const typeahead = useRef({ value: '', timer: 0 });
  const restoreFocus = useRef(true);
  const [point, setPoint] = useState<Point>({ left: -10_000, top: -10_000, maxHeight: 480, anchorWidth: 0 });
  useExternalDismiss((reason) => {
    restoreFocus.current = reason !== 'route-transition' && reason !== 'viewer-transition';
    onDismiss(reason);
  });

  useLayoutEffect(() => () => {
    if (!restoreFocus.current) return;
    const target = returnFocusRef?.current ?? anchorRef.current;
    window.requestAnimationFrame(() => {
      const current = document.activeElement;
      const focusWasLost = !current || current === document.body || !(current as Node).isConnected;
      if (focusWasLost && target?.isConnected && !target.closest('[inert]')) target.focus({ preventScroll: true });
    });
  }, [anchorRef, returnFocusRef]);

  useLayoutEffect(() => {
    const update = () => {
      if (!anchorRef.current || !contentRef.current) return;
      setPoint(calculatePosition(anchorRef.current.getBoundingClientRect(), contentRef.current.getBoundingClientRect(), placement, 7));
    };
    update();
    const frame = window.requestAnimationFrame(update);
    window.addEventListener('resize', update);
    window.addEventListener('scroll', update, true);
    window.visualViewport?.addEventListener('resize', update);
    window.visualViewport?.addEventListener('scroll', update);
    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener('resize', update);
      window.removeEventListener('scroll', update, true);
      window.visualViewport?.removeEventListener('resize', update);
      window.visualViewport?.removeEventListener('scroll', update);
    };
  }, [anchorRef, placement]);

  useEffect(() => {
    if (role !== 'menu' && role !== 'listbox' && role !== 'dialog') return;
    if (!autoFocusComposite) return;
    const frame = window.requestAnimationFrame(() => {
      if (!contentRef.current) return;
      if (role === 'dialog') {
        (focusable(contentRef.current)[0] ?? contentRef.current).focus();
        return;
      }
      const items = compositeItems(contentRef.current, role);
      const initial = role === 'listbox' ? items.find((item) => item.getAttribute('aria-selected') === 'true') : undefined;
      (initial ?? items[0])?.focus();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [autoFocusComposite, role]);

  useEffect(() => {
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (contentRef.current?.contains(target) || anchorRef.current?.contains(target)) return;
      restoreFocus.current = false;
      onDismiss('outside');
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onDismiss('escape');
        window.requestAnimationFrame(() => (returnFocusRef?.current ?? anchorRef.current)?.focus());
        return;
      }
      if (role === 'dialog' && event.key === 'Tab') {
        const items = contentRef.current ? focusable(contentRef.current) : [];
        if (!contentRef.current || !items.length) {
          event.preventDefault();
          contentRef.current?.focus();
          return;
        }
        const first = items[0];
        const last = items[items.length - 1];
        if (event.shiftKey && (document.activeElement === first || !contentRef.current.contains(document.activeElement))) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && (document.activeElement === last || !contentRef.current.contains(document.activeElement))) {
          event.preventDefault();
          first.focus();
        }
        return;
      }
      if (role !== 'menu' && role !== 'listbox') return;
      if (role === 'listbox' && !autoFocusComposite) return;
      const items = contentRef.current ? compositeItems(contentRef.current, role) : [];
      if (!items.length) return;
      const currentIndex = items.indexOf(document.activeElement as HTMLElement);
      if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
        event.preventDefault();
      } else if (event.key.length === 1 && !event.altKey && !event.ctrlKey && !event.metaKey) {
        window.clearTimeout(typeahead.current.timer);
        typeahead.current.value += event.key.toLocaleLowerCase();
        const start = currentIndex < 0 ? 0 : currentIndex + 1;
        const ordered = [...items.slice(start), ...items.slice(0, start)];
        const match = ordered.find((item) => item.textContent?.trim().toLocaleLowerCase().startsWith(typeahead.current.value));
        match?.focus();
        typeahead.current.timer = window.setTimeout(() => { typeahead.current.value = ''; }, 700);
        return;
      } else {
        return;
      }
      if (event.key === 'Home') {
        items[0].focus();
      } else if (event.key === 'End') {
        items[items.length - 1].focus();
      } else {
        const delta = event.key === 'ArrowDown' ? 1 : -1;
        const nextIndex = currentIndex < 0 ? (delta > 0 ? 0 : items.length - 1) : (currentIndex + delta + items.length) % items.length;
        items[nextIndex].focus();
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('keydown', onKeyDown);
      window.clearTimeout(typeahead.current.timer);
    };
  }, [anchorRef, autoFocusComposite, onDismiss, returnFocusRef, role]);

  return createPortal(
    <div
      id={id}
      ref={contentRef}
      className={className}
      role={role}
      aria-labelledby={labelledBy}
      aria-label={ariaLabel}
      tabIndex={role === 'dialog' ? -1 : undefined}
      style={{
        position: 'fixed',
        zIndex: 'var(--layer-floating)',
        left: point.left,
        top: point.top,
        maxHeight: point.maxHeight,
        width: matchAnchorWidth ? point.anchorWidth : undefined,
        minWidth: minAnchorWidth ? Math.max(point.anchorWidth, 190) : undefined,
      }}
    >
      {children}
    </div>,
    overlayRoot(),
  );
}

export function ModalOverlay({ children, onDismiss, labelledBy, describedBy, className = '', initialFocusRef }: { children: ReactNode; onDismiss: () => void; labelledBy: string; describedBy?: string; className?: string; initialFocusRef?: RefObject<HTMLElement | null> }) {
  const dialogRef = useRef<HTMLElement>(null);
  const backdropRef = useRef<HTMLDivElement>(null);
  const activeElement = useRef<HTMLElement | null>(null);
  const restoreFocus = useRef(true);
  const onDismissRef = useRef(onDismiss);
  useExternalDismiss((reason) => {
    restoreFocus.current = reason !== 'route-transition' && reason !== 'viewer-transition';
    onDismissRef.current();
  });

  useEffect(() => {
    onDismissRef.current = onDismiss;
  }, [onDismiss]);

  useEffect(() => {
    activeElement.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = dialogRef.current;
    const backdrop = backdropRef.current;
    if (!dialog || !backdrop) return;
    acquireModal(backdrop);
    const first = initialFocusRef?.current ?? focusable(dialog)[0] ?? dialog;
    const focusFrame = window.requestAnimationFrame(() => {
      if (isTopModal(backdrop)) first.focus();
    });

    const onKeyDown = (event: KeyboardEvent) => {
      if (!isTopModal(backdrop)) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        onDismissRef.current();
        return;
      }
      if (event.key !== 'Tab' || !dialog) return;
      const items = focusable(dialog);
      if (!items.length) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const firstItem = items[0];
      const lastItem = items[items.length - 1];
      if (event.shiftKey && document.activeElement === firstItem) {
        event.preventDefault();
        lastItem.focus();
      } else if (!event.shiftKey && document.activeElement === lastItem) {
        event.preventDefault();
        firstItem.focus();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.cancelAnimationFrame(focusFrame);
      window.removeEventListener('keydown', onKeyDown);
      releaseModal(backdrop);
      const restoreTarget = activeElement.current;
      if (!restoreFocus.current) return;
      window.requestAnimationFrame(() => {
        const current = document.activeElement;
        const focusWasLost = !current || current === document.body || !(current as Node).isConnected;
        if (focusWasLost && restoreTarget?.isConnected && !restoreTarget.closest('[inert]')) restoreTarget.focus({ preventScroll: true });
      });
    };
  }, [initialFocusRef]);

  return createPortal(
    <div ref={backdropRef} className="modal-backdrop" data-portico-modal-root="true" onPointerDown={(event) => event.target === event.currentTarget && isTopModal(backdropRef.current) && onDismiss()}>
      <section ref={dialogRef} className={className} role="dialog" aria-modal="true" aria-labelledby={labelledBy} aria-describedby={describedBy} tabIndex={-1}>
        {children}
      </section>
    </div>,
    overlayRoot(),
  );
}
