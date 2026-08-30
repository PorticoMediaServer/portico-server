import { AccountProfileIcon, StatusErrorIcon, StatusEmptyIcon, NavigationLibraryIcon, StatusLoadingIcon, NavigationSearchIcon, DeviceOfflineIcon, StatusWarningIcon } from '#portico-icons';
import {
  productMessage,
  resolveProductProblem,
  semanticIcon,
  type ProductMessageId,
  type ProductMessagePresentation,
  type SemanticIconId,
} from '@porticomediaserver/client-core';

type StructuredProblem = Error & {
  code?: string;
  messageId?: string;
  status?: number;
  details?: Readonly<Record<string, unknown>>;
};

export function productLanguageProblem(reason: unknown, fallback: ProductMessageId = 'problem.request-failed') {
  if (!(reason instanceof Error)) return productMessage(fallback);
  const problem = reason as StructuredProblem;
  if (!problem.code && !problem.messageId && !problem.status) return productMessage(fallback);
  return resolveProductProblem(problem);
}

export function ProductLanguageIcon({ presentation }: { presentation: ProductMessagePresentation }) {
  if (!presentation.icon) return null;
  const glyph = semanticIcon(presentation.icon as SemanticIconId).glyph;
  if (glyph === 'CircleX') return <StatusErrorIcon aria-hidden="true" />;
  if (glyph === 'Inbox') return <StatusEmptyIcon aria-hidden="true" />;
  if (glyph === 'LibraryBig') return <NavigationLibraryIcon aria-hidden="true" />;
  if (glyph === 'LoaderCircle') return <StatusLoadingIcon className="state-spinner" aria-hidden="true" />;
  if (glyph === 'Search') return <NavigationSearchIcon aria-hidden="true" />;
  if (glyph === 'ServerOff') return <DeviceOfflineIcon aria-hidden="true" />;
  if (glyph === 'TriangleAlert') return <StatusWarningIcon aria-hidden="true" />;
  if (glyph === 'WifiOff') return <DeviceOfflineIcon aria-hidden="true" />;
  if (glyph === 'CircleUserRound') return <AccountProfileIcon aria-hidden="true" />;
  return null;
}
