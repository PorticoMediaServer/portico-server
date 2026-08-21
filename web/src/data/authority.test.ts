import { describe, expect, it } from 'vitest';
import { canManageLibraries, canManageServer } from './authority';

describe('server authority projection', () => {
  it('treats the owner role as server authority without synthetic permission keys', () => {
    const owner = { role: 'owner' as const, permissions: { playMedia: true } };
    expect(canManageServer(owner)).toBe(true);
    expect(canManageLibraries(owner)).toBe(true);
  });

  it('does not expose server administration to an ordinary account', () => {
    const user = { role: 'user' as const, permissions: { playMedia: true } };
    expect(canManageServer(user)).toBe(false);
    expect(canManageLibraries(user)).toBe(false);
  });
});
