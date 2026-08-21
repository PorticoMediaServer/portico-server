import {
  CircleUserRound,
  CircleX,
  Inbox,
  LibraryBig,
  LoaderCircle,
  Search,
  ServerOff,
  TriangleAlert,
  WifiOff,
} from '#portico-icons';
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
  if (glyph === 'CircleX') return <CircleX aria-hidden="true" />;
  if (glyph === 'Inbox') return <Inbox aria-hidden="true" />;
  if (glyph === 'LibraryBig') return <LibraryBig aria-hidden="true" />;
  if (glyph === 'LoaderCircle') return <LoaderCircle className="state-spinner" aria-hidden="true" />;
  if (glyph === 'Search') return <Search aria-hidden="true" />;
  if (glyph === 'ServerOff') return <ServerOff aria-hidden="true" />;
  if (glyph === 'TriangleAlert') return <TriangleAlert aria-hidden="true" />;
  if (glyph === 'WifiOff') return <WifiOff aria-hidden="true" />;
  if (glyph === 'CircleUserRound') return <CircleUserRound aria-hidden="true" />;
  return null;
}
