import {defaultI18n, formatMessage, type MessageValues} from '@i18n';
import {selectionMessages} from '../../../../packages/i18n/src/catalog/en-US/next/selection';

export type SelectionMessage = keyof typeof selectionMessages;

export function selectionText(key: SelectionMessage, values?: MessageValues): string {
  return formatMessage(selectionMessages[key], values, {locale: defaultI18n.region.formatLocale});
}
