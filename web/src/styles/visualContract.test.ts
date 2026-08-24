import { describe, expect, it } from 'vitest';
import app from './app.css?raw';
import runtime from './runtime.css?raw';
import tokens from './tokens.css?raw';

describe('canonical Web element contract', () => {
  it('defines reusable native-parity control and action tokens', () => {
    expect(tokens).toContain('--portico-control-fill: var(--slate)');
    expect(tokens).toContain('--portico-action-fill: var(--screen-blue)');
    expect(tokens).toContain('--portico-provider-google-fill: #fff');
    expect(tokens).toContain('--portico-provider-apple-fill: #000');
    expect(tokens).toContain('--portico-action-focus-halo: 0 0 0 3px var(--portico-control-focus)');
  });

  it('keeps product actions solid and gives them one focus halo', () => {
    expect(app).toContain('background: var(--portico-action-fill)');
    expect(app).toContain('box-shadow: var(--portico-action-focus-halo)');
    expect(app).not.toMatch(/\.button\.primary[^}]*linear-gradient/);
  });

  it('keeps Apple and Google visibly separate and legal links blue', () => {
    expect(runtime).toContain('background: var(--portico-provider-google-fill)');
    expect(runtime).toContain('background: var(--portico-provider-apple-fill)');
    expect(runtime).toContain('color: var(--portico-link)');
  });
});
