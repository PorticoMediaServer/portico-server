import {useCallback} from 'react';
import {useNavigate, useRouter} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import {usePlayerActions} from '../player/PlayerContext';
import type {EntryOrigin} from '../player/engine';
import type {MenuAnchor} from '../ui/Menu';

export type OpenIntent = 'open' | 'play' | 'more';

/**
 * Resolves what activating an entry means. The server names the destination
 * (`navigation`) and whether it is directly playable (`playback`); this only
 * translates that into a route or a player intent.
 */
/** Where an entry's page is, as router options; nothing when the entry only plays. One rule for navigating and for a card's link. */
export function entryRoute(entry: ContentEntry, libraryId?: string) {
  const library = entry.libraryId ?? libraryId ?? '';
  const target = entry.navigation;
  if (!target) return null;
  if (target.view === 'item') return {to: '/media/$itemId', params: {itemId: target.entityId ?? entry.id}, search: {library}} as const;
  if (target.view === 'show' || target.view === 'season') {
    const id = target.entityId ?? entry.id;
    return {to: '/show/$showId', params: {showId: target.view === 'show' ? id : 'season'}, search: target.view === 'show' ? {library, title: entry.title} : {library, season: id, title: entry.title}} as const;
  }
  if (target.category !== undefined || target.view === 'browse' || target.view === 'discover' || target.view === 'collections' || target.view === 'categories') {
    return {to: '/library/$libraryId', params: {libraryId: library}, search: {view: target.view === 'discover' || target.view === 'collections' ? target.view : 'browse', category: target.category}} as const;
  }
  return {to: '/library/$libraryId/$view/$entityId', params: {libraryId: library, view: target.view, entityId: target.entityId ?? entry.id}, search: {title: entry.title}} as const;
}

/** A person's page as a URL (the app is served from the root). */
export const personHref = (personId: string) => `/person/${encodeURIComponent(personId)}`;

/** An entry's page as a URL, for the card's own link (middle-click, open in a new tab, copy link). */
export function useEntryHref() {
  const router = useRouter();
  return useCallback((entry: ContentEntry, libraryId?: string): string | undefined => {
    const route = entryRoute(entry, libraryId);
    if (!route) return undefined;
    try { return router.buildLocation(route as never).href; } catch { return undefined; }
  }, [router]);
}

export function useOpenEntry() {
  const navigate = useNavigate();
  const player = usePlayerActions();
  return useCallback(
    (entry: ContentEntry, intent: OpenIntent = 'open', libraryId?: string, anchor?: MenuAnchor, origin?: EntryOrigin) => {
      if (intent === 'play' && entry.playback) {
        player.play(entry.playback.itemId, entry.playback.startSeconds ?? entry.progressSeconds ?? 0, entry);
        return;
      }
      if (intent === 'more') {
        player.more(entry, anchor, origin);
        return;
      }
      const route = entryRoute(entry, libraryId);
      if (!route) {
        if (entry.playback) player.play(entry.playback.itemId, entry.playback.startSeconds ?? 0, entry);
        return;
      }
      void navigate(route as never);
    },
    [navigate, player],
  );
}
