import {useCallback, useRef} from 'react';
import {useRouter} from '@tanstack/react-router';

/**
 * A page's Back button. It goes back only while the previous entry is a Portico page opened in
 * this tab; otherwise (the page was opened from a link elsewhere, in a new tab, or by typing its
 * address) it goes to the page's own parent, `fallback`, instead of leaving Portico or doing
 * nothing. Every page with a Back button uses this, so they all behave the same way.
 */
export function useBack(fallback: () => void): () => void {
  const router = useRouter();
  const parent = useRef(fallback);
  parent.current = fallback;
  return useCallback(() => {
    if (router.history.canGoBack()) router.history.back();
    else parent.current();
  }, [router]);
}
