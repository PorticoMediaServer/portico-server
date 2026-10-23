import {routeBytes, routeOrigin, serverPin, webRouteCrypto, type RouteCrypto} from './route-identity.ts';
import {validInstallationId} from './installation.ts';
import {currentUTCTimestamp} from './local-session.ts';

export type RegisteredSignInContext = Readonly<{payload: string; signature: string}>;
const fields = ['kind', 'version', 'serverId', 'publicKey', 'fingerprint', 'serverName', 'registrationId', 'registrationOrigin', 'hostedOrigin', 'claimOperationId', 'challenge', 'nonce', 'installationId', 'issuedAt', 'expiresAt'] as const;
export type RegisteredSignInContextFacts = Readonly<Record<typeof fields[number], string>>;
function invalid(): never {throw new TypeError('Invalid registered sign-in context.');}
function object(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value) && [Object.prototype, null].includes(Object.getPrototypeOf(value));
}
function goJSON(value: unknown): string {return JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'));}

/** Every consumer, including Hosted before rendering, verifies the original
 * bytes. A signed display name is never a sign-in grant or a consent gesture.
 * Browser initiation must additionally compare the fresh nonce and pinned key;
 * Hosted must compare every corresponding wrapper/start/registration field. */
export async function verifyRegisteredSignInContext(value: unknown, now = Date.now(), crypto: RouteCrypto = webRouteCrypto): Promise<RegisteredSignInContextFacts> {
  if (!Number.isSafeInteger(now) || !object(value) || Object.keys(value).sort().join(',') !== 'payload,signature') invalid();
  const raw = routeBytes(value.payload, undefined, 6144), signature = routeBytes(value.signature, 64);
  let decoded: unknown;
  try {decoded = JSON.parse(new TextDecoder('utf-8', {fatal: true}).decode(raw));} catch {invalid();}
  if (!object(decoded) || Object.keys(decoded).join(',') !== fields.join(',') || fields.some(field => typeof decoded[field] !== 'string')) invalid();
  const facts = decoded as Record<typeof fields[number], string>;
  if (goJSON(facts) !== new TextDecoder().decode(raw) || facts.kind !== 'portico.registered-sign-in-prepare' || facts.version !== '1' ||
    !/^srv_[A-Za-z0-9_-]{43}$/.test(facts.serverId) || !/^ret_[A-Za-z0-9_-]{43}$/.test(facts.registrationId) ||
    !/^[A-Za-z0-9_-]{1,128}$/.test(facts.claimOperationId) || !/^[A-Za-z0-9_-]{32,128}$/.test(facts.challenge) ||
    !validInstallationId(facts.installationId) || !facts.serverName || new TextEncoder().encode(facts.serverName).length > 120 ||
    /[\p{Cc}\p{Cf}]/u.test(facts.serverName) || !facts.serverName.trim()) invalid();
  routeBytes(facts.registrationId.slice(4), 32); routeBytes(facts.nonce, 32);
  for (const origin of [facts.registrationOrigin, facts.hostedOrigin]) if (!origin.startsWith('https://') || routeOrigin(origin) !== origin) invalid();
  const utcNano = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{0,8}[1-9])?Z$/;
  if (!currentUTCTimestamp(facts.issuedAt) || !currentUTCTimestamp(facts.expiresAt) || !utcNano.test(facts.issuedAt) || !utcNano.test(facts.expiresAt)) invalid();
  const issued = Date.parse(facts.issuedAt), expires = Date.parse(facts.expiresAt);
  if (issued > now + 60000 || expires <= now || expires <= issued || expires - issued > 300000) invalid();
  await serverPin(facts.serverId, facts.publicKey, facts.fingerprint, crypto);
  if (!await crypto.verify(routeBytes(facts.publicKey, 32), signature, raw)) invalid();
  return Object.freeze({...facts});
}
