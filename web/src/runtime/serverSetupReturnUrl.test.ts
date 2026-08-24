import { describe, expect, it } from 'vitest';
import { validServerSetupReturnUrl } from './serverSetupReturnUrl';

describe('validServerSetupReturnUrl', () => {
  it.each([
    'http://localhost:32500/?porticoSetup=continue',
    'http://127.0.0.1:32500/?porticoSetup=continue',
    'http://[::1]:32500/?porticoSetup=continue',
    'https://192.168.1.20:32500/?porticoSetup=continue',
  ])('accepts an exact local setup continuation: %s', (value) => {
    expect(validServerSetupReturnUrl(value)).toBe(true);
  });

  it.each([
    'https://web.getportico.tv/?porticoSetup=continue',
    'http://localhost:32500/admin?porticoSetup=continue',
    'http://localhost:32500/?porticoSetup=continue&next=https://example.com',
    'http://user:password@localhost:32500/?porticoSetup=continue',
    'http://localhost:32500/?porticoSetup=continue#fragment',
  ])('rejects an unsafe or non-exact setup continuation: %s', (value) => {
    expect(validServerSetupReturnUrl(value)).toBe(false);
  });
});
