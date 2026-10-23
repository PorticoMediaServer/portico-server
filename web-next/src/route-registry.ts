import type {ComponentType} from 'react';
import type {ShellPlaybackBinding} from './shell/Playback';

/** Stable destinations. Screen owners supply their own component in *.route.tsx. */
export const webScreenPaths = Object.freeze({
  home: '/', search: '/search', libraries: '/libraries', library: '/library/$libraryId',
  live: '/live', saved: '/saved', item: '/item/$itemId', homeRow: '/home/rows/$rowId',
  player: '/player', settings: '/settings', dashboard: '/dashboard', notifications: '/notifications',
  profiles: '/profiles', servers: '/servers', account: '/account', signIn: '/sign-in',
  together: '/watch-together', feedback: '/feedback', help: '/help',
} as const);
export type WebScreenId = keyof typeof webScreenPaths;
export type WebScreenChrome = 'shell' | 'standalone' | 'player';
export type WebScreenDefinition = Readonly<{
  id: WebScreenId;
  component: ComponentType;
  chrome?: WebScreenChrome;
  /** The player module's one persistent media owner, mounted outside route outlets. */
  playbackHost?: ComponentType<{binding: ShellPlaybackBinding | undefined}>;
}>;
export function defineWebScreen<T extends WebScreenDefinition>(definition: T): Readonly<T> {
  if (!Object.hasOwn(webScreenPaths, definition.id)) throw new Error('unknown-web-screen');
  if (definition.playbackHost && definition.id !== 'player') throw new Error('playback-host-must-belong-to-player');
  return Object.freeze({...definition});
}
export type WebScreenModule = Readonly<{default: WebScreenDefinition | readonly WebScreenDefinition[]}>;
/** Duplicate owners fail at startup rather than silently replacing a registered screen. */
export function createWebScreenRegistry(modules: Readonly<Record<string, WebScreenModule>>) {
  const definitions = new Map<WebScreenId, WebScreenDefinition>();
  for (const filename of Object.keys(modules).sort()) {
    const exported = modules[filename].default;
    for (const value of Array.isArray(exported) ? exported : [exported]) {
      const definition = defineWebScreen(value);
      if (definitions.has(definition.id)) throw new Error(`duplicate-web-screen:${definition.id}`);
      definitions.set(definition.id, definition);
    }
  }
  return Object.freeze({
    get: (id: WebScreenId) => definitions.get(id),
    missing: () => (Object.keys(webScreenPaths) as WebScreenId[]).filter(id => !definitions.has(id)),
    chrome: (id: WebScreenId): WebScreenChrome => definitions.get(id)?.chrome
      ?? (['signIn', 'profiles', 'servers', 'account'].includes(id) ? 'standalone' : id === 'player' ? 'player' : 'shell'),
  });
}
export function webScreenIdForRoute(routeId: string): WebScreenId | undefined {
  return (Object.keys(webScreenPaths) as WebScreenId[]).find(id => webScreenPaths[id] === routeId);
}
