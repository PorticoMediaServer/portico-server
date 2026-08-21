import {
  productMessage,
  type ProductMessageId,
  type ProductMessageVariables,
} from '@porticomediaserver/client-core';
import { SecondaryButton } from '../components/controls/Buttons';
import { ProductMessageIcon, reviewedProductError } from '../components/ProductLanguage';

export type ProductProblemSpec = {
  reason?: unknown;
  fallbackId: ProductMessageId;
  variables?: ProductMessageVariables;
};

export function ProductProblemMessage({
  problem,
  reason,
  fallbackId = 'problem.request-failed',
  variables = {},
  className,
  actionHandlers = {},
  additionalActions = [],
  showIcon = true,
  showTitle = true,
}: {
  problem?: ProductProblemSpec;
  reason?: unknown;
  fallbackId?: ProductMessageId;
  variables?: ProductMessageVariables;
  className?: string;
  actionHandlers?: Partial<Record<ProductMessageId, () => void>>;
  additionalActions?: ProductMessageId[];
  showIcon?: boolean;
  showTitle?: boolean;
}) {
  const presentation = reviewedProductError(
    problem?.reason ?? reason,
    problem?.fallbackId ?? fallbackId,
    problem?.variables ?? variables,
  );
  const declared = new Set(presentation.actions.map((action) => action.id));
  const actions = [
    ...presentation.actions,
    ...additionalActions
      .filter((id) => !declared.has(id))
      .map((id) => ({ id, label: productMessage(id).text ?? id })),
  ].filter((action) => actionHandlers[action.id]);

  return <div
    className={className}
    role="alert"
    data-product-message-id={presentation.id}
    data-product-message-tone={presentation.tone}
    data-semantic-icon={presentation.icon}
  >
    {showIcon && <ProductMessageIcon presentation={presentation} />}
    {showTitle && presentation.title && <strong>{presentation.title}</strong>}
    {(presentation.body || presentation.text) && <p>{presentation.body ?? presentation.text}</p>}
    {actions.map((action) => <SecondaryButton key={action.id} onClick={actionHandlers[action.id]!}>{action.label}</SecondaryButton>)}
  </div>;
}
