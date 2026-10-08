import { Compass } from 'lucide-react';
import { Button, EmptyState } from '../../components/ui';

export function NotFound() {
  return (
    <div className="page">
      <EmptyState
        icon={<Compass />}
        title="This page wandered off"
        body="The link may be old, or the item was removed from Harbor. Everything else is right where you left it."
        actions={
          <>
            <Button variant="accent" to="/">
              Go Home
            </Button>
            <Button variant="plain" to="/search">
              Search
            </Button>
          </>
        }
      />
    </div>
  );
}
