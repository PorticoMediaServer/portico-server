import {useRouter, type ErrorComponentProps} from '@tanstack/react-router';
import {Page} from '../ui/Layout';
import {StateView} from '../ui/Feedback';

export function NotFound() {
  const router = useRouter();
  return (
    <Page title="Not found">
      <StateView icon="search" title="There’s nothing here" body="The link may be old, or the item was removed from the server." action={{label: 'Go home', icon: 'home', onClick: () => void router.navigate({to: '/'})}} secondary={{label: 'Search instead', onClick: () => void router.navigate({to: '/search'})}} />
    </Page>
  );
}

export function RouteError({error, reset}: ErrorComponentProps) {
  const router = useRouter();
  if (import.meta.env.DEV) console.warn(error);
  return (
    <Page title="Something went wrong">
      <StateView icon="alert" tone="error" title="This screen stopped working" body="Portico hit a problem showing it. Going back usually helps; your library and settings are fine." action={{label: 'Try again', icon: 'refresh', onClick: () => { reset(); void router.invalidate(); }}} secondary={{label: 'Go home', onClick: () => void router.navigate({to: '/'})}} />
    </Page>
  );
}
