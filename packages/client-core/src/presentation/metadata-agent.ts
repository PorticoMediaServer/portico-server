/**
 * A library's metadata sources in the viewer's language. The server sends each source's id with a
 * US English name and description; the two sources it has today are worded by the catalogue (the
 * online one per kind of library), and a source this client does not know keeps the server's words.
 */
import type {I18n, MessageId} from '../../../i18n/src/index.ts';

export type MetadataAgentLike = Readonly<{id: string; name: string; description?: string}>;

const onlineKinds = new Set(['movie', 'tv', 'anime', 'audiobook', 'music']);

export function metadataAgentWords(i18n: Pick<I18n, 't'>, agent: MetadataAgentLike, kind: string): {name: string; description: string} {
  if (agent.id === 'local') return {name: i18n.t('web.metadataSource.localName'), description: i18n.t('web.metadataSource.localDescription')};
  if (agent.id === 'online') {
    const description = onlineKinds.has(kind) ? i18n.t(`web.metadataSource.onlineDescription.${kind}` as MessageId) : agent.description ?? '';
    return {name: i18n.t('web.metadataSource.onlineName'), description};
  }
  return {name: agent.name, description: agent.description ?? ''};
}
