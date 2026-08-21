import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { PasswordInput, PasswordRequirements, porticoPasswordStrength, validPorticoPassword } from './PasswordInput';

describe('PasswordInput', () => {
  it('reveals and hides the password without changing its value', () => {
    render(<PasswordInput aria-label="Account password" value="Portico8!" readOnly />);
    const input = screen.getByLabelText('Account password');
    expect(input).toHaveAttribute('type', 'password');

    fireEvent.click(screen.getByRole('button', { name: 'Show password: Account password' }));
    expect(input).toHaveAttribute('type', 'text');
    expect(input).toHaveValue('Portico8!');

    fireEvent.click(screen.getByRole('button', { name: 'Hide password: Account password' }));
    expect(input).toHaveAttribute('type', 'password');
  });

  it('reports each creation requirement and validates the same policy', () => {
    const { rerender } = render(<PasswordRequirements value="Portico" />);
    expect(screen.getByText(/At least 8 characters/).closest('li')).not.toHaveClass('met');
    expect(validPorticoPassword('Portico')).toBe(false);

    rerender(<PasswordRequirements value="Portico8" />);
    expect(screen.getByText('Medium')).toBeInTheDocument();
    expect(validPorticoPassword('Portico8')).toBe(true);
    expect(porticoPasswordStrength('Portico8')).toBe('Medium');
    expect(porticoPasswordStrength('Long Portico Passphrase 8!')).toBe('Strong');
  });
});
