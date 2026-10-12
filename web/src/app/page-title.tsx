import React, {useEffect, useRef, useState} from 'react';
import {useRouterState} from '@tanstack/react-router';
import {currentI18n} from './i18n';
import {useViewerScope} from './viewer-scope';

/** What names the page at this moment: the element, and the name it gives (none while it is a stand-in or still empty). */
export type PageHeading = Readonly<{element: unknown; name: string}>;

/**
 * The name of the page that is open: its heading, or the name a page gives itself with
 * `data-page-title` when its heading is something else (Home's is the featured title). Only the
 * main region counts, so the player's and a dialog's headings never name the page. A heading
 * marked `data-provisional` is a stand-in shown before the real name is known, and names nothing.
 */
export function pageName(root: {querySelector(selector: string): {getAttribute(name: string): string | null; textContent: string | null} | null}): PageHeading | undefined {
  const found = root.querySelector('main [data-page-title], main h1');
  if (!found) return undefined;
  const text = found.getAttribute('data-provisional') !== null ? '' : found.getAttribute('data-page-title') ?? found.textContent;
  return {element: found, name: (text ?? '').replace(/\s+/g, ' ').trim().slice(0, 200)};
}

/**
 * Where the announcer is: the viewer and address it is on, whether that destination is still to
 * be announced, what the previous destination left on screen (`stale`), and what was last seen.
 */
export type PageTitleState = Readonly<{scope: string; pathname: string; pending: boolean; stale?: PageHeading; last?: PageHeading}>;
const same = (a: PageHeading | undefined, b: PageHeading | undefined) => !!a && !!b && a.element === b.element && a.name === b.name;

/**
 * The address or the viewer changed; `heading` is what is on screen at that moment. A new address
 * is a destination to announce once it has a name of its own. The first page is not announced
 * (the browser reads the document title on load). A change of viewer on the same address is not
 * a navigation: it announces nothing and drops anything not yet announced. What the previous
 * destination left on screen is remembered so that it is never taken for the new one's name: the
 * same heading the last address showed, or, when the viewer changed, whatever is showing at all.
 */
export function pageTitleMoved(previous: PageTitleState | undefined, scope: string, pathname: string, heading: PageHeading | undefined): PageTitleState {
  if (!previous) return {scope, pathname, pending: false, last: heading};
  const stale = heading && (previous.scope !== scope || same(heading, previous.last)) ? heading : undefined;
  return {scope, pathname, pending: previous.pathname !== pathname, stale, last: heading};
}

/**
 * The page was looked at again. Returns the name to announce now, if any: the destination's own
 * name the first time it has one. Nothing that happens afterwards on the same address is
 * announced: a refresh, a renamed item, a tab kept in the query.
 */
export function pageTitleRead(state: PageTitleState, heading: PageHeading | undefined): {state: PageTitleState; announce: string} {
  const name = heading && !same(heading, state.stale) ? heading.name : '';
  const announce = state.pending ? name : '';
  return {state: {...state, pending: state.pending && !announce, stale: name ? undefined : state.stale, last: heading}, announce};
}

/** The browser tab's title: the page, then the product; the product alone while a page has no name yet. */
export function documentTitle(name: string, product: string, t: (id: 'web.shell.pageTitle', values: {page: string; product: string}) => string): string {
  return name ? t('web.shell.pageTitle', {page: name, product}) : product;
}

/** The product's own title, as the document carries it before any page has named itself. */
const product = typeof document === 'undefined' ? '' : document.title;

/**
 * Keeps the document title on the page that is open and tells assistive technology when the
 * destination changes. A page's heading arrives after the address does (a split screen, then its
 * data), so the name is read from the page as it settles rather than declared per route; a
 * heading is only taken for the destination's name once it is the destination's own (see
 * `pageTitleMoved`), however long that takes.
 */
export function PageTitle() {
  const pathname = useRouterState({select: state => (state.resolvedLocation ?? state.location).pathname});
  const viewer = useViewerScope();
  const scope = viewer.serverId + '/' + viewer.viewerId;
  const [announcement, setAnnouncement] = useState('');
  const state = useRef<PageTitleState | undefined>(undefined);
  useEffect(() => {
    // Nothing from the address or viewer just left stays in the live region or waiting to enter it.
    setAnnouncement('');
    state.current = pageTitleMoved(state.current, scope, pathname, pageName(document));
    let timer: ReturnType<typeof setTimeout> | undefined;
    const read = () => {
      timer = undefined;
      const heading = pageName(document);
      const step = pageTitleRead(state.current!, heading);
      state.current = step.state;
      const title = documentTitle(heading?.name ?? '', product, currentI18n().t);
      if (document.title !== title) document.title = title;
      if (step.announce) setAnnouncement(step.announce);
    };
    // One look a quarter of a second after something on the page changes, so at most four a
    // second however busy the page is. The timer belongs to this address and viewer: leaving
    // either cancels it.
    const look = () => { timer ??= setTimeout(read, 250); };
    const observer = new MutationObserver(look);
    observer.observe(document.body, {childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ['data-page-title', 'data-provisional']});
    look();
    return () => { observer.disconnect(); if (timer !== undefined) clearTimeout(timer); };
  }, [pathname, scope]);
  return <div className="visually-hidden" role="status" aria-live="polite" aria-atomic="true">{announcement}</div>;
}
