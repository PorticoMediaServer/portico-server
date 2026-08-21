import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ProductMessageIcon, productProblem, productProblemText, reviewedProductError } from './ProductLanguage';

describe('Web Product Language boundary', () => {
  it('suppresses unknown raw error details in visible copy', () => {
    const raw = new Error('sqlite /private/media/path secret');
    expect(productProblemText(raw)).toBe('Something interrupted the request before it could finish. Try again.');
    expect(productProblemText(raw)).not.toMatch(/sqlite|private|secret/i);
  });

  it('maps structured account deletion problems and semantic icons', () => {
    const presentation = productProblem({ status: 409, code: 'owned_servers_require_action', message: 'raw Cloud detail' });
    expect(presentation.id).toBe('account.delete-owned-servers');
    expect(presentation.body).toMatch(/disconnect every server/i);
    render(<ProductMessageIcon presentation={presentation} />);
    expect(document.querySelector('svg')).not.toBeNull();
    expect(screen.queryByText(/raw Cloud detail/i)).not.toBeInTheDocument();
  });

  it('uses contextual reviewed copy for an unstructured Web failure', () => {
    const presentation = reviewedProductError(new Error('database host and private table name'), 'settings.action-failed', { actionName: 'save these settings' });
    expect(presentation.id).toBe('settings.action-failed');
    expect(presentation.body).toBe("Portico couldn't save these settings. Nothing was changed. Try again.");
    expect(presentation.body).not.toMatch(/database|private/i);
    expect(presentation.icon).toBe('status.error');
  });
});
