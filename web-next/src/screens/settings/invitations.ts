import {PeopleInvitationsClient, type PeopleInvitationPage, type PeopleInvitationQuery, type ServerInvitation} from '@core/direct-account.ts';
import {parseJoinLink} from '@core/join-link.ts';
import type {SettingsPeopleApi} from '@core/settings-people.ts';
import type {TableQuery} from '../../kit/settings/table-model';

export function invitationQuery(query: TableQuery): PeopleInvitationQuery {
  const status = query.filters.status;
  if (!['all', 'waiting', 'used', 'expired'].includes(status) || !['sent', 'status'].includes(query.sort.by)) throw new Error('invalid_invitation_query');
  return {q: query.search, ...(status === 'all' ? {} : {state: status === 'waiting' ? 'pending' as const : status === 'used' ? 'accepted' as const : 'expired' as const}),
    sort: query.sort.by as 'sent' | 'status', direction: query.sort.direction, limit: 40};
}
export function invitationShareLink(serverOrigin: string, serverId: string, code: string): string {
  const link = 'https://web.getportico.tv/join#' + new URLSearchParams({code, server: serverOrigin, id: serverId});
  const parsed = parseJoinLink(link);
  if (!parsed || parsed.server !== serverOrigin || parsed.id !== serverId || parsed.code !== code) throw new Error('invalid_invitation_link');
  return link;
}
/** Captured private UI adapter over the actual counted producer. Revisions,
 * totals, search/filtering and keyset order always stay with that producer. */
export class SettingsInvitations {
  readonly client: PeopleInvitationsClient;
  private readonly lifetime = new AbortController();
  private closed = false;
  constructor(api: SettingsPeopleApi, private readonly isCurrent: () => boolean, readonly origin: string, readonly serverId: string) {
    this.client = new PeopleInvitationsClient({request: async <T,>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> => {
      this.assertCurrent();
      const currentSignal = signal ? AbortSignal.any([signal, this.lifetime.signal]) : this.lifetime.signal;
      currentSignal.throwIfAborted();
      const reply = await api.request<T>(path, method, body, currentSignal);
      this.assertCurrent(); currentSignal.throwIfAborted(); return reply;
    }});
  }
  private assertCurrent() {if (this.closed || !this.isCurrent()) throw new DOMException('Invitation authority retired', 'AbortError');}
  read(query: TableQuery, cursor: string, signal: AbortSignal): Promise<PeopleInvitationPage> {
    return this.client.list({...invitationQuery(query), ...(cursor ? {cursor} : {})}, signal);
  }
  async link(invitation: ServerInvitation, signal: AbortSignal): Promise<string> {
    const link = await this.client.link(invitation, signal);
    if (link.serverId !== undefined && link.serverId !== this.serverId) throw new Error('invitation_server_changed');
    this.assertCurrent(); signal.throwIfAborted();
    return invitationShareLink(this.origin, this.serverId, link.code);
  }
  dispose() {this.closed = true; this.lifetime.abort();}
}
