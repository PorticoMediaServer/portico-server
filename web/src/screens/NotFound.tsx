import {useEffect} from 'react';
import {StateView} from '../ui';
import {useI18n} from '../app/i18n';
import {useNavigate} from '@tanstack/react-router';

export function NotFoundScreen() {
  return <NotFoundView />;
}

function NotFoundView({inApp}: {inApp?: boolean}) {
  const {t} = useI18n();
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

/**
 * A split screen's file could not be loaded: most often a tab that was open across a server
 * upgrade asking for a file name from the build before it. Each browser words the failed import
 * its own way (Chromium, Firefox, Safari), and the bundler's preload has a fourth.
 */
export function screenFileFailed(error: unknown): boolean {
  const message = error instanceof Error ? error.message : '';
  return /dynamically imported module|Importing a module script failed|Unable to preload CSS/i.test(message);
}

/** Raw errors never reach the page; the person gets a way back. */
/** Route crash boundary: catalogue copy only; the raw error goes to the console, never to the page. */
export function RouteErrorScreen({error, reset}: {error?: unknown; reset?: () => void}) {
  const {t} = useI18n();
  const navigate = useNavigate();
  useEffect(() => { if (error) console.error('[route error]', error); }, [error]);
  // Resetting the boundary re-renders the same failed import, which never loads again in this
  // document; only a fresh document asks the server for the current files.
  const stale = screenFileFailed(error);
  return (
    <div style={{minHeight: '60vh', display: 'grid', placeItems: 'center'}}>
      <StateView icon="warning" title={t('route.errorTitle')} body={t('route.errorBody')} action={{label: t('action.tryAgain'), onClick: () => (reset && !stale ? reset() : location.reload())}} secondaryAction={{label: t('route.goHome'), onClick: () => void navigate({to: '/'})}} />
    </div>
  );
}
