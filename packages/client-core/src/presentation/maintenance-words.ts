/**
 * Storage care categories and maintenance day presets in the viewer's language. The server sends
 * each with an id and US English words; the catalogue words the ones it knows by id, and an id this
 * client does not know yet keeps the server's words.
 */
import type {I18n, MessageId} from '../../../i18n/src/index.ts';

type Words = Pick<I18n, 't' | 'has'>;

export function storageCategoryWords(i18n: Words, category: Readonly<{id: string; name: string; description?: string}>): {name: string; description: string} {
  const id = `server.storage.category.${category.id}`;
  if (!i18n.has(id)) return {name: category.name, description: category.description ?? ''};
  return {name: i18n.t(id as MessageId), description: i18n.t(`${id}.description` as MessageId)};
}

export function maintenanceCadenceName(i18n: Words, cadence: Readonly<{id: string; name: string}>): string {
  const id = `server.maintenance.cadence.${cadence.id}`;
  return i18n.has(id) ? i18n.t(id as MessageId) : cadence.name;
}
