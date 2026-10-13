import {createAccessToken, listAccessTokens, revokeAccessToken} from '@core/api-access.ts';
import {useCursorList} from '../../admin/cursor-read';
import {QUIET_READ_EVERY_MS} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Notice} from '../../ui';
import {CredentialList, credentialPage, useApiAccessOptions} from '../server/ApiAccess';
import s from './Settings.module.css';

/**
 * Profile & account › Personal access tokens: a member's own tokens for their own scripts and
 * apps. A token signs in as the member and can do only what was ticked when it was made, never
 * more than the member. When the owner has turned tokens off, the ones that exist are still
 * listed and can still be revoked.
 */
export default function AccessTokensPanel() {
  const {t} = useI18n();
  const {api} = useSession();
  const options = useApiAccessOptions();
  const read = useCursorList(cursor => credentialPage(listAccessTokens(api, cursor)), [api], {every: QUIET_READ_EVERY_MS});
  const limit = options.data ? `${t('api.credential.limitNote', {count: options.data.requestsPerMinute})} ` : '';
  return (
    <>
      <p className={s.sectionLede}>{t('api.tokens.lede')}</p>
      {options.data && !options.data.tokensAllowed ? <Notice tone="info" compact>{t('api.tokens.off')}</Notice> : null}
      <CredentialList kind="token" read={read} options={options.data} create={input => createAccessToken(api, input)} revoke={id => revokeAccessToken(api, id)} footnote={limit + t('api.tokens.lifetime')} />
    </>
  );
}
