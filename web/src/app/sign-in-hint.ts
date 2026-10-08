import type {LocalSession} from '@core/index.ts';

/**
 * A sign-in that has ended (renewal refused, credential revoked, another server at the saved
 * address) opens the sign-in screen, not the shell: a direct sign-in on the direct form for the
 * same server with the username filled in, a Portico Account on account sign-in and its server
 * chooser. An unreachable server is different: the shell opens with a banner and keeps retrying.
 */
export type SignInHint = {authority: 'local' | 'hosted'; serverId: string; serverUrl: string; accountId: string; username?: string};

/** `portico`: a Portico Account member (a server-local session reached through the account). */
export function signInHintFor(serverUrl: string, session: Pick<LocalSession, 'viewer'>, username?: string, portico = false): SignInHint {
  const v = session.viewer;
  return {authority: portico || v.authority === 'hosted' ? 'hosted' : 'local', serverId: v.serverId, serverUrl, accountId: v.accountId, ...(username ? {username} : {})};
}
