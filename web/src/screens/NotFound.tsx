import {useEffect} from 'react';
import {StateView} from '../ui';
import {currentI18n} from '../app/i18n';

const t = (id: Parameters<ReturnType<typeof currentI18n>['t']>[0]) => currentI18n().t(id);
import {useNavigate} from '@tanstack/react-router';

export function NotFoundScreen() {
  return <NotFoundView />;
}

function NotFoundView({inApp}: {inApp?: boolean}) {
  const navigate = useNavigate();
  return (
    <div style={{minHeight: inApp ? '60vh' : '100vh', display: 'grid', placeItems: 'center'}}>
      <StateView icon="info" title={t('route.notFoundTitle')} body={t('route.notFoundBody')} action={{label: t('route.goHome'), onClick: () => void navigate({to: '/'})}} secondaryAction={inApp ? {label: t('route.search'), onClick: () => void navigate({to: '/search'})} : undefined} />
    </div>
  );
}

/** WEB-SHELL-01: an unknown address inside the app keeps the app's frame (rail, player). */
export function InAppNotFound() {
  return <NotFoundView inApp />;
}

/** Raw errors never reach the page; the person gets a way back. */
/** Route crash boundary: catalogue copy only; the raw error goes to the console, never to the page. */
export function RouteErrorScreen({error, reset}: {error?: unknown; reset?: () => void}) {
  const navigate = useNavigate();
  useEffect(() => { if (error) console.error('[route error]', error); }, [error]);
  return (
    <div style={{minHeight: '60vh', display: 'grid', placeItems: 'center'}}>
      <StateView icon="warning" title={t('route.errorTitle')} body={t('route.errorBody')} action={{label: t('action.tryAgain'), onClick: () => (reset ? reset() : location.reload())}} secondaryAction={{label: t('route.goHome'), onClick: () => void navigate({to: '/'})}} />
    </div>
  );
}
