import type {HttpLocalApi, ServerPin} from '@core/index.ts';
import {claimErrorMessage, useClaimConnect} from '../../app/claim-connect';
import {errorText} from '../../app/errors';
import {useI18n} from '../../app/i18n';
import {Button, Notice, Text} from '../../ui';

/**
 * The server's "Connect" for a Portico Account (Server › General). No sign-in
 * here (BE-hosted d37e441): it opens the approval page, where the person signs in and approves,
 * and waits for the decision; `onDone` refreshes the caller.
 */
export function ClaimConnectButton({api, pin, hostedOrigin, onDone, size = 'sm'}: {api: HttpLocalApi; pin: ServerPin | undefined; hostedOrigin: string; onDone: () => void; size?: 'sm' | 'md'}) {
  const i18n = useI18n();
  const connector = useClaimConnect(api, pin, hostedOrigin, onDone);
  const e = connector.error;
  const known = e ? claimErrorMessage(e) : undefined;
  const message = !e ? '' : known ? i18n.t(known) : errorText(e, 'claim', 'action');
  return (
    <>
      <Button size={size} variant="primary" label={i18n.t('web.claim.connect')} iconAfter="external" loading={connector.busy} disabled={!pin} onClick={() => void connector.connect()} />
      {connector.phase === 'waiting' ? <Text variant="caption" tone="secondary">{i18n.t('web.claim.waiting')}</Text> : null}
      {message ? <Notice tone="error" compact>{message}</Notice> : null}
    </>
  );
}
