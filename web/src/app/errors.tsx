import {presentError, type ErrorContext, type ErrorOperation, type PresentedError} from '@core/presentation/index.ts';
import {Notice, StateView} from '../ui';
import type {I18n} from '@i18n';

/** The viewer's catalogue, set by `app/i18n.ts` (no runtime import, so this module stays standalone). */
let errorI18n: I18n | undefined;
export function setErrorI18n(i18n: I18n): void {
  errorI18n = i18n;
}

/**
 * Web bindings for the shared error presenter (X-04). Every consumer error
 * goes through `presentError`: the words come from the catalogue by code and
 * status, never from `error.message`, and the button offered is the one
 * recovery that's actually true (Try again, Refresh, or none).
 */
export function present(error: unknown, context: ErrorContext, operation: ErrorOperation = 'load', retrying = false): PresentedError {
  return presentError(error, context, {operation, retrying, deviceOnline: typeof navigator === 'undefined' ? undefined : navigator.onLine, ...(errorI18n ? {i18n: errorI18n} : {})});
}

/** Body copy only, for an inline message inside a dialog or form that already names the task. */
export function errorText(error: unknown, context: ErrorContext, operation: ErrorOperation = 'action'): string {
  const p = present(error, context, operation);
  return p.silent ? '' : p.body;
}

type Recover = {retry?: () => void; refresh?: () => void};

export function recoveryAction(p: PresentedError, on: Recover): {label: string; onClick: () => void} | undefined {
  if (!p.actionLabel) return undefined;
  if (p.action === 'try-again' && on.retry) return {label: p.actionLabel, onClick: on.retry};
  const refresh = on.refresh ?? on.retry;
  if (p.action === 'refresh' && refresh) return {label: p.actionLabel, onClick: refresh};
  return undefined;
}

/** Full-page failure: what failed, what to do, and the one truthful button. */
export function ErrorState({error, context, retry, refresh}: {error: unknown; context: ErrorContext} & Recover) {
  const p = present(error, context);
  if (p.silent) return null;
  return <StateView icon="warning" title={p.title} body={p.body} action={recoveryAction(p, {retry, refresh})} />;
}

/** Inline failure above content that's still shown. `changed` presents as a warning to refresh. */
export function ErrorNotice({error, context, operation = 'load', retry, refresh, compact}: {error: unknown; context: ErrorContext; operation?: ErrorOperation; compact?: boolean} & Recover) {
  const p = present(error, context, operation);
  if (p.silent) return null;
  return <Notice tone={p.category === 'changed' ? 'warning' : 'error'} title={p.category === 'changed' ? undefined : p.title} compact={compact} action={recoveryAction(p, {retry, refresh})}>{p.body}</Notice>;
}

/** A `refresh-required` snapshot may carry no error; present it as a change. */
export const changedError = {code: 'refresh_required'} as const;
