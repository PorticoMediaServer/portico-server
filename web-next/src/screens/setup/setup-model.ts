import {defaultI18n, type MessageId} from '@i18n';
import {passwordStrength} from '@core/password-strength.ts';
import {presentError} from '@core/presentation/errors.ts';

export type SetupMode = 'hosted' | 'local';
export type SetupLibraryKind = 'movie' | 'tv' | 'anime' | 'music' | 'audiobook' | 'book' | 'comic';

type SetupKindIcon = 'i-film' | 'i-tvlib' | 'i-category' | 'i-music' | 'i-headph' | 'i-sbook' | 'i-book';

/** The seven drawn library kinds (owner-libraries#13), in frame order, with the
 * frame's own name and one-line description for each. */
export const setupLibraryKinds: readonly Readonly<{id: SetupLibraryKind; label: MessageId; description: MessageId; icon: SetupKindIcon}>[] = Object.freeze([
  {id: 'movie', label: 'next.ownerLibraries.kind.movie', description: 'next.ownerLibraries.kindDescription.movie', icon: 'i-film'},
  {id: 'tv', label: 'next.ownerLibraries.kind.tv', description: 'next.ownerLibraries.kindDescription.tv', icon: 'i-tvlib'},
  {id: 'anime', label: 'next.ownerLibraries.kind.anime', description: 'next.ownerLibraries.kindDescription.anime', icon: 'i-category'},
  {id: 'music', label: 'next.ownerLibraries.kind.music', description: 'next.ownerLibraries.kindDescription.music', icon: 'i-music'},
  {id: 'audiobook', label: 'next.ownerLibraries.kind.audiobook', description: 'next.ownerLibraries.kindDescription.audiobook', icon: 'i-headph'},
  {id: 'book', label: 'next.ownerLibraries.kind.book', description: 'next.ownerLibraries.kindDescription.book', icon: 'i-sbook'},
  {id: 'comic', label: 'next.ownerLibraries.kind.comic', description: 'next.ownerLibraries.kindDescription.comic', icon: 'i-book'},
]);

const kindMessages: Readonly<Record<SetupLibraryKind, Readonly<{label: MessageId; description: MessageId}>>> = Object.freeze(
  Object.fromEntries(setupLibraryKinds.map(kind => [kind.id, {label: kind.label, description: kind.description}])) as Record<SetupLibraryKind, Readonly<{label: MessageId; description: MessageId}>>);

export function libraryKindLabel(kind: SetupLibraryKind): string {
  return defaultI18n.t(kindMessages[kind].label);
}

/** The chosen kind's own line under the tiles (owner-libraries#13). */
export function libraryKindDescription(kind: SetupLibraryKind): string {
  return defaultI18n.t(kindMessages[kind].description);
}

export function defaultLibraryName(kind: SetupLibraryKind): string {
  return libraryKindLabel(kind);
}

/** A server left named "Portico" is indistinguishable from every other one; suggest the host. */
export function defaultSetupServerName(current: string, hostname: string): string {
  if (current && current !== 'Portico') return current;
  if (!hostname || /^(localhost|\d{1,3}(\.\d{1,3}){3}|\[.*\])$/.test(hostname)) return defaultI18n.t('web.setup.defaultServerName');
  return hostname.replace(/\.(local|lan|home)$/i, '').replace(/[-_]+/g, ' ');
}

/** The drawn local owner: 3 to 64 characters, no spaces or @. */
export function validLocalUsername(value: string): boolean {
  return /^[^\s@]{3,64}$/.test(value.trim());
}

export function setupNameValid(value: string): boolean {
  const name = value.trim();
  return name.length > 0 && name.length <= 80;
}

export function localPasswordValid(password: string, confirm: string): boolean {
  return passwordStrength(password).acceptable && password === confirm;
}

/** A setup that cannot be authorized at this address answers 401/403; it is
 * never "sign in again" because no account exists yet. */
export function setupErrorMessage(error: unknown): string {
  const status = (error as {status?: number} | undefined)?.status;
  if (status === 401 || status === 403) return defaultI18n.t('web.setup.notOnNetwork');
  return presentError(error, 'sign-in', {operation: 'action', i18n: defaultI18n}).body;
}

export type SetupStep = Readonly<{id: 'server' | 'account' | 'library'; state: 'done' | 'current' | 'next'; title: string; body: string}>;

/** Steps follow the drawn two-step local setup and three-step Portico setup. */
export function setupSteps(input: Readonly<{serverName: string; mode: SetupMode; phase: 'form' | 'library'; local: boolean}>): readonly SetupStep[] {
  const server = {id: 'server' as const, state: input.phase === 'library' ? 'done' as const : 'current' as const,
    title: defaultI18n.t('web.setup.stepServer'),
    body: input.phase === 'library' ? input.serverName : input.local ? defaultI18n.t('web.setup.stepServerLocalBody') : defaultI18n.t('web.setup.stepServerBody')};
  if (input.mode === 'local') return [server, {id: 'library', state: input.phase === 'library' ? 'current' : 'next',
    title: defaultI18n.t('web.setup.stepLibrary'), body: input.phase === 'library' ? defaultI18n.t('web.setup.stepLibraryLater') : defaultI18n.t('web.setup.stepLibraryBody')}];
  return [server,
    {id: 'account', state: input.phase === 'library' ? 'done' : 'next', title: defaultI18n.t('web.setup.stepAccount'),
      body: input.phase === 'library' ? defaultI18n.t('web.setup.ownsServer') : defaultI18n.t('web.setup.stepAccountBody')},
    {id: 'library', state: input.phase === 'library' ? 'current' : 'next', title: defaultI18n.t('web.setup.stepLibrary'),
      body: input.phase === 'library' ? defaultI18n.t('web.setup.stepLibraryLater') : defaultI18n.t('web.setup.stepLibraryBody')}];
}
