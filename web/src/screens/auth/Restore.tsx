import {useSession} from '../../app/session';
import {currentI18n} from '../../app/i18n';
import {StatusScreen} from '../../ui';
import {AuthFrame} from './AuthFrame';

/** Opening and reconnecting share one calm status; the wordmark stays above it. */
export function RestoreScreen() {
  const session = useSession();
  const t = currentI18n().t;
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
