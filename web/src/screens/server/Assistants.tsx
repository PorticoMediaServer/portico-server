import {useState} from 'react';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, SettingsGroup, SettingsRow, Text} from '../../ui';
import {useOpenApiKeys} from './ApiAccess';

/** Where an assistant connects: this server's address with the path every Portico server uses. */
export function assistantAddress(serverBase: string): string {
  return new URL('/v1/mcp', serverBase).toString();
}

/**
 * Integrations › Assistants: what it is, the address to give an assistant, and the way to the
 * key it needs. There is nothing to turn on: the address answers only to an API key or a
 * personal access token, and an assistant can do only what that key allows.
 */
export function AssistantsPanel() {
  const {t} = useI18n();
  const {api} = useSession();
  const openKeys = useOpenApiKeys();
  const [copied, setCopied] = useState(false);
  const address = assistantAddress(api.baseUrl);
  return (
    <SettingsGroup title={t('api.mcp.title')} description={t('api.mcp.lede')}>
      <SettingsRow label={t('api.mcp.address')} help={t('api.mcp.addressHelp')} stack start
        control={<div style={{display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap'}}>
          <Text variant="mono" style={{fontSize: 13, overflowWrap: 'anywhere'}}>{address}</Text>
          <Button size="sm" variant="secondary" icon="copy" label={t('web.twoStep.copy')} onClick={() => void navigator.clipboard?.writeText(address).then(() => setCopied(true), () => {})} />
          <span role="status">{copied ? <Text variant="caption" tone="tertiary">{t('api.credential.copied')}</Text> : null}</span>
        </div>} />
      <SettingsRow label={t('api.mcp.key')} help={t('api.mcp.keyHelp')} control={<Button size="sm" variant="secondary" label={t('web.access.createKey')} onClick={openKeys} />} />
    </SettingsGroup>
  );
}
