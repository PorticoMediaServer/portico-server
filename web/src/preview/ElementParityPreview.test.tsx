import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ElementParityPreview } from './ElementParityPreview';

describe('Web element parity preview', () => {
  it('shows branded providers, labeled fields, a Portico action, and legal links on the auth view', () => {
    window.location.hash = '#auth';
    render(<ElementParityPreview />);

    expect(screen.getByRole('button', { name: 'Sign in with Google' })).toHaveClass('google');
    expect(screen.getByRole('button', { name: 'Sign in with Apple' })).toHaveClass('apple');
    expect(screen.getByLabelText('Username or email')).toHaveAttribute('placeholder', 'Username or email');
    expect(screen.getByLabelText('Password')).toHaveAttribute('placeholder', 'Password');
    expect(screen.getByRole('button', { name: 'Sign in' })).toHaveClass('parity-portico-action');
    expect(screen.getByRole('link', { name: 'Terms of Use' })).toHaveAttribute('href', 'https://getportico.tv/terms/');
    expect(screen.getByRole('link', { name: 'Privacy Policy' })).toHaveAttribute('href', 'https://getportico.tv/privacy/');
  });

  it('exposes the explicit field and action states in the gallery', () => {
    window.location.hash = '#states';
    render(<ElementParityPreview />);

    expect(screen.getByRole('heading', { name: 'Component states' })).toBeInTheDocument();
    expect(screen.getByLabelText('Invalid')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByLabelText('Disabled')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Focus action' })).toHaveClass('is-focus');
    expect(screen.getByRole('button', { name: 'Disabled action' })).toBeDisabled();

    window.location.hash = '#auth';
    fireEvent(window, new HashChangeEvent('hashchange'));
    expect(screen.getByRole('heading', { name: 'Sign in to Portico' })).toBeInTheDocument();
  });
});
