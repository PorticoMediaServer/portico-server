import {useSession} from '../../app/session';
import {StatusScreen} from '../../ui';
import {AuthFrame} from './AuthFrame';
import {useI18n} from '../../app/i18n';

/** Opening and reconnecting share one calm status; the wordmark stays above it. */
export function RestoreScreen() {
  const {t} = useI18n();
  const session = useSession();
  const reconnecting = !!session.restoreMessage;
  return (
    <AuthFrame legal={false}>
      <StatusScreen
        title={reconnecting ? t('launch.reconnectingTitle') : t('launch.title')}
        body={reconnecting ? t('web.restore.reconnectingBody') : t('web.restore.checking')}
        action={reconnecting ? {label: t('web.chooser.checkNow'), onClick: session.retryRestore} : undefined}
        secondaryAction={reconnecting ? {label: t('web.restore.useAnother'), onClick: session.disconnectServer} : session.busy ? {label: t('action.cancel'), onClick: session.cancelRestore} : undefined}
      />
    </AuthFrame>
  );
}
