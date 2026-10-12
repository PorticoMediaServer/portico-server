import React, {useEffect, useRef, useState} from 'react';
import {useRouterState} from '@tanstack/react-router';
import {currentI18n} from './i18n';

/**
 * The name of the page that is open: its heading, or the name a page gives itself with
 * `data-page-title` when its heading is something else (Home's is the featured title). Only the
 * main region counts, so the player's and a dialog's headings never name the page.
 */
export function pageName(root: {querySelector(selector: string): {getAttribute(name: string): string | null; textContent: string | null} | null}): string {
  const found = root.querySelector('main [data-page-title], main h1');
  const text = found ? found.getAttribute('data-page-title') ?? found.textContent : '';
  return (text ?? '').replace(/\s+/g, ' ').trim().slice(0, 200);
}

export type PageTitleState = Readonly<{pathname: string; announced: boolean}>;

/**
 * Whether to announce the page now. A destination is announced once, when it first has a name
 * after the address moved to it. The first page is not announced (the browser reads the document
 * title on load), and neither is anything that happens while the address stays the same: a
 * refresh, a renamed item, a tab kept in the query.
 */
export function pageTitleStep(previous: PageTitleState | undefined, pathname: string, name: string): {state: PageTitleState; announce: boolean} {
  const pending = previous ? (previous.pathname === pathname ? !previous.announced : true) : false;
  const announce = pending && name !== '';
  return {state: {pathname, announced: !pending || announce}, announce};
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
 * data), so the name is read from the page as it settles rather than declared per route.
 */
export function PageTitle() {
  const pathname = useRouterState({select: state => (state.resolvedLocation ?? state.location).pathname});
  const [announcement, setAnnouncement] = useState('');
  const state = useRef<PageTitleState | undefined>(undefined);
  useEffect(() => {
    // Cleared first, so a destination with the same name as the last one is still spoken.
    setAnnouncement('');
    let timer: ReturnType<typeof setTimeout> | undefined;
    const read = () => {
      timer = undefined;
      const name = pageName(document);
      const step = pageTitleStep(state.current, pathname, name);
      state.current = step.state;
      const title = documentTitle(name, product, currentI18n().t);
      if (document.title !== title) document.title = title;
      if (step.announce) setAnnouncement(name);
    };
    // Read a moment after the page last changed, and at most a few times a second however busy it is.
    const settle = () => { timer ??= setTimeout(read, 250); };
    const observer = new MutationObserver(settle);
    observer.observe(document.body, {childList: true, subtree: true, characterData: true});
    settle();
    return () => { observer.disconnect(); if (timer !== undefined) clearTimeout(timer); };
  }, [pathname]);
  return <div className="visually-hidden" role="status" aria-live="polite" aria-atomic="true">{announcement}</div>;
}
