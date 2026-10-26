import {useEffect, useLayoutEffect, useRef, useState, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';
import type {OverlayChange, OverlayDismissReason} from './types';

export const classes = (...values: (string | undefined | false)[]) => values.filter(Boolean).join(' ');
export const focusableSelector = 'button:not([disabled]),[href],input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
export function focusable(root: ParentNode): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(focusableSelector)).filter(element => !element.closest('[inert],[hidden],[aria-hidden="true"]') && element.getClientRects().length > 0);
}
export function focusElement(element: HTMLElement | null | undefined) { if (element?.isConnected) element.focus({preventScroll: true}); }

/** Portalled child surfaces must paint above the dialog that contains their trigger. */
export function useInDialog(open: boolean, anchor: RefObject<HTMLElement | null>) {
  const [inside, setInside] = useState(false);
  useLayoutEffect(() => { setInside(!!anchor.current?.closest('[role="dialog"],[role="alertdialog"]')); }, [open, anchor]);
  return inside;
}

export function usePhone() {
  const [phone, setPhone] = useState(() => typeof window !== 'undefined' && window.matchMedia('(max-width: 599px)').matches);
  useEffect(() => {
    const media = window.matchMedia('(max-width: 599px)'), update = () => setPhone(media.matches);
    media.addEventListener('change', update); update();
    return () => media.removeEventListener('change', update);
  }, []);
  return phone;
}

export function useReducedMotion(elementRef?: RefObject<HTMLElement | null>) {
  const [reduced, setReduced] = useState(false);
  useLayoutEffect(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => setReduced(media.matches || !!(elementRef?.current ?? document.documentElement).closest('[data-reduce-motion="true"]') || document.documentElement.dataset.reduceMotion === 'true' || document.body.dataset.reduceMotion === 'true');
    const observer = new MutationObserver(update);
    observer.observe(document.documentElement, {attributes: true, subtree: true, attributeFilter: ['data-reduce-motion']});
    media.addEventListener('change', update); update();
    return () => { observer.disconnect(); media.removeEventListener('change', update); };
  }, [elementRef]);
  return reduced;
}

const stack: symbol[] = [];
/** Back/Menu keys dismiss only the highest overlay. Escape stays with Radix except for SidePanel. */
export function useOverlayKeys(open: boolean, onOpenChange: OverlayChange, escape = false, dismissOnBack = true) {
  const callback = useRef(onOpenChange); callback.current = onOpenChange;
  const identity = useRef(Symbol());
  useLayoutEffect(() => {
    if (!open) return;
    const id = identity.current; stack.push(id);
    const keydown = (event: KeyboardEvent) => {
      if (stack.at(-1) !== id || event.defaultPrevented) return;
      const isEscape = escape && event.key === 'Escape';
      const isBack = ['BrowserBack', 'GoBack', 'Back', 'Menu', 'ContextMenu'].includes(event.key);
      if (!isEscape && !isBack) return;
      event.preventDefault(); event.stopPropagation();
      if (isEscape || dismissOnBack) callback.current(false, isEscape ? 'escape' : 'back');
    };
    document.addEventListener('keydown', keydown);
    return () => { const index = stack.indexOf(id); if (index >= 0) stack.splice(index, 1); document.removeEventListener('keydown', keydown); };
  }, [open, escape, dismissOnBack]);
  return () => stack.at(-1) === identity.current;
}

export function useSurfaceFocus(open: boolean, initialFocusRef?: RefObject<HTMLElement | null>, returnFocusRef?: RefObject<HTMLElement | null>) {
  const opener = useRef<HTMLElement | null>(null);
  const suppressRestore = useRef(false);
  useLayoutEffect(() => { if (open) { opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; suppressRestore.current = false; } }, [open]);
  return {
    suppressRestore,
    onOpenAutoFocus(event: Event) { if (initialFocusRef?.current) { event.preventDefault(); focusElement(initialFocusRef.current); } },
    onCloseAutoFocus(event: Event) { event.preventDefault(); if (!suppressRestore.current) focusElement(returnFocusRef?.current ?? opener.current); },
  };
}

const anchoredOpenEvent = 'portico:anchored-overlay-open';
export function useExclusiveAnchor(open: boolean, change: OverlayChange) {
  const callback = useRef(change); callback.current = change;
  const [superseded, setSuperseded] = useState(false);
  useLayoutEffect(() => {
    if (!open) return;
    setSuperseded(false);
    const id = Symbol();
    document.dispatchEvent(new CustomEvent(anchoredOpenEvent, {detail: id}));
    const receive = (event: Event) => { if ((event as CustomEvent<symbol>).detail !== id) { setSuperseded(true); callback.current(false, 'action'); } };
    document.addEventListener(anchoredOpenEvent, receive);
    return () => document.removeEventListener(anchoredOpenEvent, receive);
  }, [open]);
  return superseded;
}

export function useDismissReason() {
  const reason = useRef<OverlayDismissReason>('action');
  return reason;
}

export function usePresence(open: boolean, milliseconds: number) {
  const [held, setHeld] = useState(open);
  useEffect(() => {
    if (open) { setHeld(true); return; }
    const timer = window.setTimeout(() => setHeld(false), milliseconds);
    return () => window.clearTimeout(timer);
  }, [open, milliseconds]);
  return open || held;
}

/** Cancel and retarget from the painted value, including a reversal during entry. */
export function useSurfaceMotion(open: boolean, kind: 'dialog' | 'menu' | 'panel' | 'fade' | 'toast' | 'banner', exitTransform?: RefObject<string | null>) {
  const [element, setElement] = useState<HTMLElement | null>(null);
  const animation = useRef<Animation | null>(null);
  const motionElement = useRef<HTMLElement | null>(null); motionElement.current = element;
  const prefersReduced = useReducedMotion(motionElement);
  const first = useRef(true);
  const previousElement = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    if (!element) return;
    if (previousElement.current !== element) { first.current = true; previousElement.current = element; }
    const reduced = prefersReduced || window.matchMedia('(prefers-reduced-motion: reduce)').matches || !!element.closest('[data-reduce-motion="true"]') || document.documentElement.dataset.reduceMotion === 'true';
    const style = getComputedStyle(element), entering = open;
    const initialTransform = kind === 'dialog' ? 'scale(.98)' : kind === 'menu' ? 'translateY(-4px) scale(.98)' : kind === 'panel' ? 'translateX(24px)' : kind === 'toast' ? 'translateY(24px)' : 'none';
    const from = {opacity: first.current && entering ? 0 : Number(style.opacity), transform: reduced ? 'none' : first.current && entering ? initialTransform : style.transform};
    animation.current?.cancel(); first.current = false;
    const targetTransform = reduced || entering ? 'none' : exitTransform?.current ?? (kind === 'panel' ? initialTransform : from.transform);
    const current = element.animate([from, {opacity: entering ? 1 : 0, transform: targetTransform}], {duration: reduced ? reducedMotion.durationMs : entering ? motion.standard : motion.quick, easing: entering ? easings.out : easings.in, fill: 'both'});
    animation.current = current;
    current.onfinish = () => {
      if (animation.current !== current) return;
      element.style.opacity = entering ? '1' : '0'; element.style.transform = targetTransform;
      current.cancel(); animation.current = null;
    };
    return () => { /* The next run reads the painted value before cancelling. */ };
  }, [element, open, kind, exitTransform, prefersReduced]);
  useEffect(() => () => animation.current?.cancel(), []);
  return setElement;
}
