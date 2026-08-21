import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { UnexpectedErrorBoundary } from './UnexpectedErrorBoundary';

afterEach(() => vi.restoreAllMocks());

test('contains unexpected render failures without exposing their details', async () => {
  vi.spyOn(console, 'error').mockImplementation(() => undefined);
  let shouldFail = true;
  function Child() {
    if (shouldFail) throw new Error('/private/media/secret.mkv');
    return <p>Recovered content</p>;
  }

  render(<UnexpectedErrorBoundary><Child /></UnexpectedErrorBoundary>);

  const heading = screen.getByRole('heading', { name: "Portico couldn't complete this request" });
  await waitFor(() => expect(heading).toHaveFocus());
  expect(screen.queryByText(/secret\.mkv/)).not.toBeInTheDocument();
  expect(screen.getByText(/Something interrupted the request before it could finish/i)).toBeInTheDocument();

  shouldFail = false;
  fireEvent.click(screen.getByRole('button', { name: /Try again/ }));
  expect(screen.getByText('Recovered content')).toBeInTheDocument();
});
