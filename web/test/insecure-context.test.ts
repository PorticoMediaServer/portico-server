import test from 'node:test';
import assert from 'node:assert/strict';
import {ed25519} from '@noble/curves/ed25519.js';
import {installInsecureContextFallbacks} from '../src/app/polyfills.ts';
import {webRouteCrypto, setRouteCryptoFallback} from '@core/route-identity.ts';

// A page served over plain HTTP from a LAN address: getRandomValues exists; randomUUID, Web
// Locks and crypto.subtle don't.
function insecurePage() {
  return {crypto: {getRandomValues: (b: Uint8Array) => globalThis.crypto.getRandomValues(b)} as unknown as Crypto, navigator: {} as Navigator};
}

test('ids, locks and server verification work on a plain-HTTP page', async () => {
  const page = insecurePage();
  installInsecureContextFallbacks(page);
  const id = (page.crypto as Crypto & {randomUUID: () => string}).randomUUID();
  assert.match(id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  // The page installs randomId as crypto.randomUUID; randomId must not then call itself.
  const {randomId} = await import('@core/random-id.ts');
  const native = Object.getOwnPropertyDescriptor(globalThis.crypto, 'randomUUID') ?? Object.getOwnPropertyDescriptor(Object.getPrototypeOf(globalThis.crypto), 'randomUUID');
  Object.defineProperty(globalThis.crypto, 'randomUUID', {configurable: true, value: randomId});
  try { assert.match(randomId(), /^[0-9a-f-]{36}$/); } finally {
    delete (globalThis.crypto as {randomUUID?: unknown}).randomUUID;
    if (native && !Object.getOwnPropertyDescriptor(Object.getPrototypeOf(globalThis.crypto), 'randomUUID')) Object.defineProperty(globalThis.crypto, 'randomUUID', native);
  }

  // Work under one lock name runs one at a time, in order.
  const locks = (page.navigator as Navigator & {locks: {request: <T>(n: string, w: () => Promise<T>) => Promise<T>}}).locks;
  const order: string[] = [];
  const slow = locks.request('a', async () => { order.push('first start'); await new Promise(r => setTimeout(r, 20)); order.push('first end'); });
  const next = locks.request('a', async () => { order.push('second'); });
  await Promise.all([slow, next]);
  assert.deepEqual(order, ['first start', 'first end', 'second']);
});

test('the JavaScript fallback verifies exactly what crypto.subtle would', async () => {
  const subtle = Object.getOwnPropertyDescriptor(globalThis.crypto, 'subtle');
  const page = insecurePage();
  installInsecureContextFallbacks(page);
  // Pretend this process is the insecure page for webRouteCrypto.
  Object.defineProperty(globalThis.crypto, 'subtle', {configurable: true, get: () => undefined});
  try {
    const secret = ed25519.utils.randomSecretKey();
    const key = ed25519.getPublicKey(secret);
    const message = new TextEncoder().encode('portico.route.proof');
    const signature = ed25519.sign(message, secret);
    assert.equal(await webRouteCrypto.verify(key, signature, message), true);
    const tampered = message.slice(); tampered[0] ^= 1;
    assert.equal(await webRouteCrypto.verify(key, signature, tampered), false);
    const digest = await webRouteCrypto.sha256(new TextEncoder().encode('abc'));
    assert.equal(Buffer.from(digest).toString('hex'), 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad');
  } finally {
    if (subtle) Object.defineProperty(globalThis.crypto, 'subtle', subtle); else delete (globalThis.crypto as {subtle?: unknown}).subtle;
    setRouteCryptoFallback(undefined as never);
  }
});
