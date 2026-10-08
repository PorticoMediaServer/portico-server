import {EmptyState} from '@/ui/Bits';
import {useTitle} from '@/ui/hooks';

export default function NotFound() {
  useTitle('Not found');
  return (
    <div style={{minHeight: '70vh', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
      <EmptyState icon="discover" title="There’s nothing at this address" body="The link may be old, or the page may have moved. Home has everything else." actions={[{label: 'Go to Home', icon: 'home', to: '/'}, {label: 'Search', icon: 'search', to: '/search', variant: 'secondary'}]} />
    </div>
  );
}
