import {
  Archive,
  ArrowLeft,
  ArrowDown,
  ArrowRight,
  ArrowUp,
  BadgeCheck,
  Bell,
  CheckCheck,
  CircleCheck,
  CircleCheckBig,
  CircleHelp,
  CircleUserRound,
  CircleX,
  Flag,
  Inbox,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  LogOut,
  Mail,
  MessageSquare,
  MessageSquareWarning,
  RefreshCw,
  RotateCw,
  Search,
  ServerOff,
  SlidersHorizontal,
  Timer,
  Trash2,
  TriangleAlert,
  WifiOff,
  X,
  type LucideIcon,
} from '#portico-icons';
import {
  productMessage,
  resolveProductProblem,
  semanticIcon,
  type ProductMessageId,
  type ProductMessagePresentation,
  type ProductMessageVariables,
  type SemanticIconId,
} from '@porticomediaserver/client-core';

const glyphs: Readonly<Record<string, LucideIcon>> = {
  Archive,
  ArrowLeft,
  ArrowDown,
  ArrowRight,
  ArrowUp,
  BadgeCheck,
  Bell,
  CheckCheck,
  CircleCheck,
  CircleCheckBig,
  CircleUserRound,
  CircleX,
  Flag,
  Inbox,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  LogOut,
  Mail,
  MessageSquare,
  MessageSquareWarning,
  RefreshCw,
  RotateCw,
  Search,
  ServerOff,
  SlidersHorizontal,
  Timer,
  Trash2,
  TriangleAlert,
  WifiOff,
  X,
};

export function SemanticProductIcon({ id, className }: { id: SemanticIconId; className?: string }) {
  const definition = semanticIcon(id);
  const Icon = glyphs[definition.glyph] ?? CircleHelp;
  return <Icon className={className} aria-hidden="true" />;
}

export function ProductMessageIcon({ presentation, className }: { presentation: ProductMessagePresentation; className?: string }) {
  return presentation.icon ? <SemanticProductIcon id={presentation.icon} className={className} /> : null;
}

export function productText(id: ProductMessageId, variables: ProductMessageVariables = {}): string {
  const presentation = productMessage(id, variables);
  return presentation.text ?? presentation.title ?? presentation.body ?? '';
}

function structuredProblem(reason: unknown): { code?: string; messageId?: string; status?: number; details?: Readonly<Record<string, unknown>> } {
  if (!reason || typeof reason !== 'object') return {};
  const value = reason as Record<string, unknown>;
  return {
    code: typeof value.code === 'string' ? value.code : undefined,
    messageId: typeof value.messageId === 'string' ? value.messageId : undefined,
    status: typeof value.status === 'number' ? value.status : undefined,
    details: value.details && typeof value.details === 'object' && !Array.isArray(value.details)
      ? value.details as Readonly<Record<string, unknown>>
      : undefined,
  };
}

/** Returns only catalog-owned copy. Error.message/detail remains diagnostic data. */
export function productProblem(
  reason: unknown,
  fallbackId: ProductMessageId = 'problem.request-failed',
  variables: ProductMessageVariables = {},
): ProductMessagePresentation {
  const problem = structuredProblem(reason);
  if (problem.code || problem.messageId || problem.status) return resolveProductProblem(problem, variables);
  return productMessage(fallbackId, variables);
}

export function productProblemText(
  reason: unknown,
  fallbackId: ProductMessageId = 'problem.request-failed',
  variables: ProductMessageVariables = {},
): string {
  const presentation = productProblem(reason, fallbackId, variables);
  return presentation.body ?? presentation.title ?? productText(fallbackId, variables);
}

/**
 * Uses structured API metadata when available and otherwise falls back to
 * reviewed catalog copy. Error.message and response detail remain diagnostic.
 */
export function reviewedProductError(
  reason: unknown,
  fallbackId: ProductMessageId,
  variables: ProductMessageVariables = {},
): ProductMessagePresentation {
  const problem = structuredProblem(reason);
  if (problem.code || problem.messageId || problem.status) return resolveProductProblem(problem, variables);
  return productMessage(fallbackId, variables);
}

export function reviewedProductErrorText(
  reason: unknown,
  fallbackId: ProductMessageId,
  variables: ProductMessageVariables = {},
): string {
  const presentation = reviewedProductError(reason, fallbackId, variables);
  return presentation.body ?? presentation.title ?? productText(fallbackId, variables);
}
