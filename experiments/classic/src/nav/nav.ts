import {createNavigationContainerRef, StackActions, CommonActions} from '@react-navigation/native';
import type {AppRouteName, AppRoutes, RootRoutes} from './routes';
import type {Entry} from '../data/types';
import {Platform} from 'react-native';
import {requestContentFocus} from '../ui/focusMemory';

/** The root container (auth screens and the Main shell). */
export const rootNav = createNavigationContainerRef<RootRoutes>();

type ContentNav = {
  push: (name: string, params?: object) => void;
  navigate: (name: string, params?: object) => void;
  goBack: () => void;
  canGoBack: () => boolean;
  dispatch: (a: unknown) => void;
  getState: () => {index: number; routes: {name: string; params?: object}[]};
};
let content: ContentNav | undefined;
const listeners = new Set<() => void>();

/** Called by every content screen so global UI (rail, player, deep links) can drive the stack. */
export function registerContentNav(n: ContentNav) {
  content = n;
}
export function contentNav() {
  return content;
}

export function push<K extends AppRouteName>(name: K, params?: AppRoutes[K]) {
  if (Platform.isTV) requestContentFocus();
  if (content) content.dispatch(StackActions.push(name, params as object));
  else if (rootNav.isReady()) (rootNav.navigate as (a: string, b: object) => void)('Main', {screen: name, params});
}

/** Switches the root of the content stack (rail and tab destinations). */
export function root<K extends AppRouteName>(name: K, params?: AppRoutes[K]) {
  if (Platform.isTV) requestContentFocus();
  if (content) content.dispatch(CommonActions.reset({index: 0, routes: [{name, params: params as object}]}));
  listeners.forEach(fn => fn());
}

/**
 * Opens a destination from the iPhone header or profile menu. It is pushed on
 * top of the current tab, so that tab stays selected and Back returns to it.
 * Elsewhere (rail, sidebar) it becomes the new root.
 */
export function openFromChrome<K extends AppRouteName>(name: K, params?: AppRoutes[K], phone = false) {
  if (!phone) return root(name, params);
  const cur = currentRoute();
  if (cur?.name === name && JSON.stringify(cur.params ?? {}) === JSON.stringify(params ?? {})) return;
  push(name, params);
}

export function back() {
  if (content?.canGoBack()) content.goBack();
}

export function currentRoute(): {name: string; params?: object} | undefined {
  const s = content?.getState();
  return s ? s.routes[s.index] : undefined;
}

export function onRootChange(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

/** Where an entry opens. */
export function openEntry(e: Pick<Entry, 'id' | 'kind' | 'libraryId' | 'title' | 'parentId'>) {
  switch (e.kind) {
    case 'movie': return e.id.includes('-trailer') || e.id.includes('-bts') || e.id.includes('-scene') ? undefined : push('Movie', {id: e.id});
    case 'homevideo': return push('HomeVideo', {id: e.id});
    case 'show': return push('Show', {id: e.id});
    case 'episode': return push('Episode', {id: e.id});
    case 'season': return push('Show', {id: e.parentId!});
    case 'artist': return push('Artist', {id: e.id});
    case 'album': return push('Album', {id: e.id});
    case 'track': return push('Album', {id: e.parentId!});
    case 'audiobook': return push('Audiobook', {id: e.id});
    case 'photoAlbum': return push('PhotoAlbum', {id: e.id});
    case 'person': return push('Person', {id: e.id});
    case 'collection': return e.id.startsWith('col-') ? push('Entity', {libraryId: e.libraryId ?? 'lib-movies', kind: 'collection', id: e.id, title: e.title}) : push('Resource', {kind: 'collection', id: e.id});
    case 'playlist': return push('Resource', {kind: 'playlist', id: e.id});
    case 'view': return push('Resource', {kind: 'view', id: e.id});
    case 'author': return push('Entity', {libraryId: 'lib-audiobooks', kind: 'author', id: e.id.replace('author:', ''), title: e.title});
    case 'series': return push('Entity', {libraryId: 'lib-audiobooks', kind: 'series', id: e.id.replace('series:', ''), title: e.title});
    case 'genre': return push('Entity', {libraryId: e.libraryId ?? 'lib-movies', kind: 'genre', id: e.title, title: e.title});
    default: return undefined;
  }
}
