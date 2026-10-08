import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from 'react-router';
import { router } from './routes';
import { PreferencesBridge } from './Shell';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 10 * 60_000,
      retry: (count, err) => count < 1 && !(err instanceof Error && 'status' in err && (err as { status: number }).status === 404),
      refetchOnWindowFocus: false,
    },
  },
});

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <PreferencesBridge />
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
