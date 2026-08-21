import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, beforeEach } from 'vitest';

const lockTails = new Map<string, Promise<void>>();

const testLockManager = {
  request: <T,>(name: string, _options: LockOptions, callback: () => Promise<T> | T): Promise<T> => {
    const predecessor = lockTails.get(name) ?? Promise.resolve();
    let release!: () => void;
    const reservation = new Promise<void>((resolve) => { release = resolve; });
    const tail = predecessor.catch(() => undefined).then(() => reservation);
    lockTails.set(name, tail);
    return predecessor.catch(() => undefined).then(callback).finally(() => {
      release();
      if (lockTails.get(name) === tail) lockTails.delete(name);
    });
  },
};

beforeEach(() => {
  lockTails.clear();
  Object.defineProperty(navigator, 'locks', {configurable: true, value: testLockManager});
  const overlays = document.createElement('div');
  overlays.id = 'portico-overlays';
  document.body.append(overlays);
});

afterEach(() => {
  lockTails.clear();
  cleanup();
  document.getElementById('portico-overlays')?.remove();
});

Object.defineProperty(window, 'scrollTo', { value: () => undefined, writable: true });
Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  writable: true,
  value: (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => true,
  }),
});

if (!HTMLElement.prototype.scrollIntoView) {
  HTMLElement.prototype.scrollIntoView = () => undefined;
}
