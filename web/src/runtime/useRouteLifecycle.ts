import { type RefObject, useEffect, useRef, useState } from 'react';
import { type Location, type NavigationType } from 'react-router-dom';
import { dismissOverlays, VIEWER_TRANSITION_EVENT } from '../components/overlay/OverlayPortal';

type ScrollPosition = { x: number; y: number };

const scrollPositions = new Map<string, ScrollPosition>();
const maximumRememberedRoutes = 80;

function rememberScroll(key: string, position: ScrollPosition) {
  scrollPositions.delete(key);
  scrollPositions.set(key, position);
  if (scrollPositions.size > maximumRememberedRoutes) {
    const oldest = scrollPositions.keys().next().value;
    if (oldest) scrollPositions.delete(oldest);
  }
}

export function useRouteLifecycle(
  location: Location,
  navigationType: NavigationType,
  mainContent: RefObject<HTMLElement | null>,
) {
  const mounted = useRef(false);
  const previousPath = useRef(location.pathname);
  const generation = useRef(0);
  const [transitioning, setTransitioning] = useState(false);

  useEffect(() => {
    const previousRestoration = window.history.scrollRestoration;
    window.history.scrollRestoration = 'manual';
    return () => {
      window.history.scrollRestoration = previousRestoration;
    };
  }, []);

  useEffect(() => {
    const clearViewerHistory = () => {
      scrollPositions.clear();
      generation.current += 1;
    };
    window.addEventListener(VIEWER_TRANSITION_EVENT, clearViewerHistory);
    return () => window.removeEventListener(VIEWER_TRANSITION_EVENT, clearViewerHistory);
  }, []);

  useEffect(() => {
    const routeKey = location.key;
    let frozen = scrollPositions.has(routeKey);
    let releaseTimer = 0;
    const capture = () => {
      if (!frozen) rememberScroll(routeKey, { x: window.scrollX, y: window.scrollY });
    };
    const release = () => {
      frozen = false;
      window.clearTimeout(releaseTimer);
      capture();
    };
    const freezeForLink = (event: MouseEvent) => {
      const link = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>('a[href]') : null;
      if (!link || link.target || link.download) return;
      const destination = new URL(link.href, window.location.href);
      if (destination.origin !== window.location.origin) return;
      if (`${destination.pathname}${destination.search}` === `${location.pathname}${location.search}`) return;
      capture();
      frozen = true;
    };
    const freezeForHistory = () => {
      capture();
      frozen = true;
    };
    const freezeForViewer = () => {
      frozen = true;
    };
    if (frozen) releaseTimer = window.setTimeout(release, 1_100);
    else capture();
    window.addEventListener('scroll', capture, { passive: true });
    window.addEventListener('wheel', release, { passive: true, once: true });
    window.addEventListener('touchstart', release, { passive: true, once: true });
    window.addEventListener('pointerdown', release, { passive: true, once: true });
    window.addEventListener('keydown', release, { once: true });
    document.addEventListener('click', freezeForLink, true);
    window.addEventListener('popstate', freezeForHistory);
    window.addEventListener(VIEWER_TRANSITION_EVENT, freezeForViewer);
    return () => {
      window.removeEventListener('scroll', capture);
      window.clearTimeout(releaseTimer);
      window.removeEventListener('wheel', release);
      window.removeEventListener('touchstart', release);
      window.removeEventListener('pointerdown', release);
      window.removeEventListener('keydown', release);
      document.removeEventListener('click', freezeForLink, true);
      window.removeEventListener('popstate', freezeForHistory);
      window.removeEventListener(VIEWER_TRANSITION_EVENT, freezeForViewer);
    };
  }, [location.key, location.pathname, location.search]);

  useEffect(() => {
    const pathChanged = previousPath.current !== location.pathname;
    previousPath.current = location.pathname;
    if (!mounted.current) {
      mounted.current = true;
      return;
    }

    const currentGeneration = ++generation.current;
    dismissOverlays('route-transition');
    setTransitioning(pathChanged);

    const stored = navigationType === 'POP' ? scrollPositions.get(location.key) : undefined;
    let secondFrame = 0;
    let restoreTimer = 0;
    let transitionTimer = 0;
    let restoreAttempts = 0;
    let restorationInterrupted = false;
    const interruptRestoration = () => {
      restorationInterrupted = true;
      window.clearTimeout(restoreTimer);
    };
    const restoreStoredPosition = () => {
      if (!stored || restorationInterrupted || generation.current !== currentGeneration) return;
      window.scrollTo({ top: stored.y, left: stored.x, behavior: 'auto' });
      const maximumY = Math.max(0, document.documentElement.scrollHeight - window.innerHeight);
      if (maximumY + 1 < stored.y && restoreAttempts < 20) {
        restoreAttempts += 1;
        restoreTimer = window.setTimeout(restoreStoredPosition, 50);
      }
    };
    window.addEventListener('wheel', interruptRestoration, { passive: true, once: true });
    window.addEventListener('touchstart', interruptRestoration, { passive: true, once: true });
    window.addEventListener('pointerdown', interruptRestoration, { passive: true, once: true });
    window.addEventListener('keydown', interruptRestoration, { once: true });
    const firstFrame = window.requestAnimationFrame(() => {
      secondFrame = window.requestAnimationFrame(() => {
        if (generation.current !== currentGeneration) return;
        if (stored) restoreStoredPosition();
        else if (pathChanged) window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
        if (pathChanged && !stored) mainContent.current?.focus({ preventScroll: true });
        const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
          || document.documentElement.dataset.porticoReduceMotion === 'true';
        if (reducedMotion || !pathChanged) setTransitioning(false);
        else transitionTimer = window.setTimeout(() => {
          if (generation.current === currentGeneration) setTransitioning(false);
        }, 160);
      });
    });

    return () => {
      window.cancelAnimationFrame(firstFrame);
      window.cancelAnimationFrame(secondFrame);
      window.clearTimeout(restoreTimer);
      window.clearTimeout(transitionTimer);
      window.removeEventListener('wheel', interruptRestoration);
      window.removeEventListener('touchstart', interruptRestoration);
      window.removeEventListener('pointerdown', interruptRestoration);
      window.removeEventListener('keydown', interruptRestoration);
    };
  }, [location.key, location.pathname, mainContent, navigationType]);

  return transitioning;
}
