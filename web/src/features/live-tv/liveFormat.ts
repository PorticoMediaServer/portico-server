import { productMessage, type ProductMessageId, type ProductMessageVariables } from '@porticomediaserver/client-core';
import { productProblem } from '../../components/ProductLanguage';

export function productState(id: ProductMessageId, variables: ProductMessageVariables = {}) {
  const presentation = productMessage(id, variables);
  return {
    title: presentation.title ?? presentation.text ?? '',
    message: presentation.body ?? presentation.text ?? presentation.title ?? '',
  };
}

export function localDay(date = new Date()) {
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 10);
}

export function initialGuideStart(date = new Date()) {
  const start = new Date(date);
  start.setMinutes(Math.floor(start.getMinutes() / 30) * 30, 0, 0);
  return start;
}

export function timeLabel(value: string | Date) {
  return new Date(value).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

export function dateLabel(value: string | Date) {
  return new Date(value).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' });
}

export function dateTimeLabel(value: string) {
  return new Date(value).toLocaleString([], { weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
}

export function formatBytes(value: number) {
  if (!value) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount >= 10 || unit === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[unit]}`;
}

export function requestError(reason: unknown, fallbackId: ProductMessageId, variables: ProductMessageVariables = {}) {
  const presentation = productProblem(reason, fallbackId, variables);
  const fallback = productMessage(fallbackId, variables);
  return presentation.body ?? presentation.title ?? presentation.text ?? fallback.body ?? fallback.title ?? fallback.text ?? '';
}

export function isConflictError(reason: unknown) {
  const error = reason as { status?: number; code?: string } | undefined;
  return error?.status === 409 || error?.code === 'conflict';
}
