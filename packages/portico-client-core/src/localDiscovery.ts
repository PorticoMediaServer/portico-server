import { isValidPorticoServerPublicKeyFingerprint, validatePorticoUrl } from "./urlPolicy.js";

export const PORTICO_LAN_SERVICE_TYPE = "_portico._tcp" as const;
export const PORTICO_LAN_TXT_VERSION = 1 as const;
export const DEFAULT_PORTICO_SERVICE_PORT = 32500;

const defaultTTLSeconds = 120;
const minimumTTLSeconds = 15;
const maximumTTLSeconds = 600;
const serverIDPattern = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;

export type NativeDiscoveryTXT = Readonly<Record<string, string>> | readonly string[];

export type LocalServerRouteAddressClass = "localhost" | "loopback" | "rfc1918-lan" | "not-local";

/** Classifies the selected route address only; it is never viewer/network locality evidence. */
export function localServerRouteAddressClass(rawURL: string): LocalServerRouteAddressClass {
  let url: URL;
  try {
    url = new URL(rawURL);
  } catch {
    return "not-local";
  }
  if (url.protocol !== "http:" || url.port !== String(DEFAULT_PORTICO_SERVICE_PORT) ||
      url.username || url.password || (url.pathname !== "/" && url.pathname !== "") || url.search || url.hash) {
    return "not-local";
  }
  try {
    validatePorticoUrl(rawURL, "lan-server-route");
  } catch {
    return "not-local";
  }
  const hostname = url.hostname.toLowerCase();
  if (hostname === "localhost") return "localhost";
  if (hostname === "127.0.0.1") return "loopback";
  return isRFC1918IPv4(hostname) ? "rfc1918-lan" : "not-local";
}

export function isValidLocalServerRouteURL(rawURL: string): boolean {
  return localServerRouteAddressClass(rawURL) !== "not-local";
}

/** Raw, platform-neutral output expected from Bonjour/Network.framework, Android NSD, or another native mDNS provider. */
export interface NativePorticoDiscoveryRecord {
  serviceType: string;
  instanceName: string;
  hostname?: string;
  port: number;
  addresses?: readonly string[];
  txt: NativeDiscoveryTXT;
  observedAt?: Date | string | number;
  ttlSeconds?: number;
  interfaceName?: string;
}

export interface PorticoLANDiscoverySubscription {
  stop(): void | Promise<void>;
}

export type PorticoLANDiscoveryEvent =
  | { type: "found" | "updated"; record: NativePorticoDiscoveryRecord }
  | { type: "lost"; serviceType: string; instanceName: string; interfaceName?: string };

/** Native shells implement this boundary. Client Core intentionally does not select an mDNS library. */
export interface PorticoLANDiscoveryProvider {
  subscribe(
    listener: (event: PorticoLANDiscoveryEvent) => void,
    options?: { signal?: AbortSignal }
  ): PorticoLANDiscoverySubscription | Promise<PorticoLANDiscoverySubscription>;
}

export interface PorticoLANRouteCandidate {
  type: "lan";
  url: string;
  address: string;
  port: number;
  serverId?: string;
  serverPublicKeyFingerprint: string;
}

export interface NormalizedPorticoDiscoveryRecord {
  serviceType: typeof PORTICO_LAN_SERVICE_TYPE;
  instanceName: string;
  displayName: string;
  hostname?: string;
  port: number;
  path: string;
  serverId?: string;
  serverPublicKeyFingerprint: string;
  addresses: string[];
  interfaceNames: string[];
  observedAt: string;
  expiresAt: string;
  stale: boolean;
  identityConflict: boolean;
  routes: PorticoLANRouteCandidate[];
}

export type PorticoLANTrustState = "trusted" | "unverified" | "stale" | "rejected" | "identity-conflict";

export interface PorticoLANTrustExpectation {
  expectedServerId?: string;
  /** Fingerprint from a signed Hosted route document or a prior explicit local pairing. */
  expectedServerPublicKeyFingerprint?: string;
}

export class PorticoLANDiscoveryRecordError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PorticoLANDiscoveryRecordError";
  }
}

export function normalizePorticoDiscoveryRecord(
  raw: NativePorticoDiscoveryRecord,
  now: Date = new Date()
): NormalizedPorticoDiscoveryRecord {
  const serviceType = normalizeServiceType(raw.serviceType);
  if (serviceType !== PORTICO_LAN_SERVICE_TYPE) {
    throw new PorticoLANDiscoveryRecordError(`Unsupported discovery service ${raw.serviceType}.`);
  }
  const instanceName = cleanLabel(raw.instanceName, "instance name");
  const txt = parsePorticoDiscoveryTXT(raw.txt);
  if (txt.txtversion !== String(PORTICO_LAN_TXT_VERSION)) {
    throw new PorticoLANDiscoveryRecordError("The Portico discovery TXT version is not supported.");
  }
  if (txt.scheme !== "http") {
    throw new PorticoLANDiscoveryRecordError("LAN discovery must advertise local HTTP.");
  }
  const port = normalizePort(raw.port);
  const path = normalizePath(txt.path ?? "/");
  const fingerprint = normalizeFingerprint(txt.fingerprint);
  const serverId = normalizeServerID(txt.serverid);
  const hostname = normalizeHostname(raw.hostname);
  const addresses = uniqueSortedAddresses(raw.addresses ?? []);
  if (!hostname && addresses.length === 0) {
    throw new PorticoLANDiscoveryRecordError("A discovery record must include a local hostname or address.");
  }
  const observedAt = parseObservedAt(raw.observedAt, now);
  const ttlSeconds = normalizeTTL(raw.ttlSeconds);
  const expiresAt = new Date(observedAt.getTime() + ttlSeconds * 1000);
  const normalized: NormalizedPorticoDiscoveryRecord = {
    serviceType,
    instanceName,
    displayName: cleanOptionalLabel(txt.name) ?? instanceName,
    ...(hostname ? { hostname } : {}),
    port,
    path,
    ...(serverId ? { serverId } : {}),
    serverPublicKeyFingerprint: fingerprint,
    addresses,
    interfaceNames: raw.interfaceName ? [cleanLabel(raw.interfaceName, "interface name")] : [],
    observedAt: observedAt.toISOString(),
    expiresAt: expiresAt.toISOString(),
    stale: now.getTime() > expiresAt.getTime(),
    identityConflict: false,
    routes: []
  };
  normalized.routes = normalized.stale ? [] : porticoLANRouteCandidates(normalized);
  return normalized;
}

export function parsePorticoDiscoveryTXT(raw: NativeDiscoveryTXT): Record<string, string> {
  const result: Record<string, string> = {};
  const entries = Array.isArray(raw)
    ? raw.map((entry) => {
        const separator = entry.indexOf("=");
        return separator < 1 ? [entry, ""] as const : [entry.slice(0, separator), entry.slice(separator + 1)] as const;
      })
    : Object.entries(raw);
  for (const [rawKey, rawValue] of entries) {
    const key = rawKey.trim().toLowerCase();
    if (!/^[a-z][a-z0-9]{0,31}$/.test(key)) {
      throw new PorticoLANDiscoveryRecordError("A discovery TXT key is invalid.");
    }
    if (Object.hasOwn(result, key)) {
      throw new PorticoLANDiscoveryRecordError(`Discovery TXT key ${key} is duplicated.`);
    }
    result[key] = rawValue.trim();
  }
  return result;
}

export function porticoLANRouteCandidates(
  record: Pick<NormalizedPorticoDiscoveryRecord, "hostname" | "addresses" | "path" | "port" | "serverId" | "serverPublicKeyFingerprint" | "stale" | "identityConflict">
): PorticoLANRouteCandidate[] {
  if (record.stale || record.identityConflict) return [];
  const hosts = [...record.addresses];
  if (record.hostname) hosts.push(record.hostname);
  const uniqueHosts = [...new Set(hosts.map(normalizeRouteHost))].sort(compareRouteHosts);
  return uniqueHosts.map((host) => ({
    type: "lan",
    url: `http://${formatURLHost(host)}:${record.port}${record.path === "/" ? "" : record.path}`,
    address: host,
    port: record.port,
    ...(record.serverId ? { serverId: record.serverId } : {}),
    serverPublicKeyFingerprint: record.serverPublicKeyFingerprint
  }));
}

/** Collapses duplicate interface announcements by persistent server fingerprint. */
export function dedupePorticoDiscoveryRecords(records: readonly NormalizedPorticoDiscoveryRecord[], now = new Date()): NormalizedPorticoDiscoveryRecord[] {
  const groups = new Map<string, NormalizedPorticoDiscoveryRecord[]>();
  for (const record of records) {
    const group = groups.get(record.serverPublicKeyFingerprint) ?? [];
    group.push(record);
    groups.set(record.serverPublicKeyFingerprint, group);
  }
  return [...groups.values()].map((group) => mergeDiscoveryGroup(group, now))
    .sort((left, right) => left.displayName.localeCompare(right.displayName) || left.serverPublicKeyFingerprint.localeCompare(right.serverPublicKeyFingerprint));
}

export function porticoLANTrustState(record: NormalizedPorticoDiscoveryRecord, expectation: PorticoLANTrustExpectation = {}): PorticoLANTrustState {
  if (record.identityConflict) return "identity-conflict";
  if (record.stale) return "stale";
  const expectedServerId = normalizeServerID(expectation.expectedServerId);
  if (expectedServerId && record.serverId !== expectedServerId) return "rejected";
  const expectedFingerprint = expectation.expectedServerPublicKeyFingerprint
    ? normalizeFingerprint(expectation.expectedServerPublicKeyFingerprint)
    : undefined;
  if (expectedFingerprint && record.serverPublicKeyFingerprint !== expectedFingerprint) return "rejected";
  return expectedFingerprint ? "trusted" : "unverified";
}

function mergeDiscoveryGroup(group: readonly NormalizedPorticoDiscoveryRecord[], now: Date): NormalizedPorticoDiscoveryRecord {
  const active = group.filter((record) => Date.parse(record.expiresAt) >= now.getTime());
  const contributors = active.length > 0 ? active : group;
  const newest = [...contributors].sort((left, right) => Date.parse(right.observedAt) - Date.parse(left.observedAt))[0];
  if (!newest) throw new PorticoLANDiscoveryRecordError("Cannot merge an empty discovery group.");
  const serverIds = [...new Set(contributors.map((record) => record.serverId).filter((value): value is string => Boolean(value)))];
  const ports = [...new Set(contributors.map((record) => record.port))];
  const paths = [...new Set(contributors.map((record) => record.path))];
  const identityConflict = serverIds.length > 1 || ports.length > 1 || paths.length > 1;
  const expiresAt = new Date(Math.max(...contributors.map((record) => Date.parse(record.expiresAt))));
  const merged: NormalizedPorticoDiscoveryRecord = {
    ...newest,
    ...(serverIds.length === 1 ? { serverId: serverIds[0] } : { serverId: undefined }),
    addresses: uniqueSortedAddresses(contributors.flatMap((record) => record.addresses)),
    interfaceNames: [...new Set(contributors.flatMap((record) => record.interfaceNames))].sort(),
    expiresAt: expiresAt.toISOString(),
    stale: now.getTime() > expiresAt.getTime(),
    identityConflict,
    routes: []
  };
  merged.routes = porticoLANRouteCandidates(merged);
  return merged;
}

function normalizeServiceType(value: string): string {
  return value.trim().toLowerCase().replace(/\.local\.?$/, "").replace(/\.$/, "");
}

function normalizePort(value: number): number {
  if (!Number.isInteger(value) || value < 1 || value > 65535) {
    throw new PorticoLANDiscoveryRecordError("The advertised service port is invalid.");
  }
  return value;
}

function normalizeTTL(value: number | undefined): number {
  if (value === undefined) return defaultTTLSeconds;
  if (!Number.isFinite(value) || value <= 0) throw new PorticoLANDiscoveryRecordError("The discovery TTL is invalid.");
  return Math.min(maximumTTLSeconds, Math.max(minimumTTLSeconds, Math.round(value)));
}

function parseObservedAt(value: Date | string | number | undefined, fallback: Date): Date {
  const date = value === undefined ? new Date(fallback) : new Date(value);
  if (!Number.isFinite(date.getTime())) throw new PorticoLANDiscoveryRecordError("The discovery observation time is invalid.");
  return date;
}

function normalizeFingerprint(value: string | undefined): string {
  const fingerprint = value?.trim() ?? "";
  if (!isValidPorticoServerPublicKeyFingerprint(fingerprint)) {
    throw new PorticoLANDiscoveryRecordError("The advertised server fingerprint is invalid.");
  }
  return fingerprint;
}

function normalizeServerID(value: string | undefined): string | undefined {
  const serverId = value?.trim();
  if (!serverId) return undefined;
  if (!serverIDPattern.test(serverId)) throw new PorticoLANDiscoveryRecordError("The advertised server ID is invalid.");
  return serverId;
}

function normalizePath(value: string): string {
  const path = value.trim();
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("?") || path.includes("#") || path.includes("\\")) {
    throw new PorticoLANDiscoveryRecordError("The advertised service path is invalid.");
  }
  return path === "/" ? path : path.replace(/\/+$/, "");
}

function normalizeHostname(value: string | undefined): string | undefined {
  const hostname = value?.trim().replace(/\.$/, "").toLowerCase();
  if (!hostname) return undefined;
  if (!hostname.endsWith(".local") || !/^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?\.local$/.test(hostname)) {
    throw new PorticoLANDiscoveryRecordError("The advertised mDNS hostname is invalid.");
  }
  return hostname;
}

function uniqueSortedAddresses(values: readonly string[]): string[] {
  return [...new Set(values.map(normalizeLANAddress))].sort(compareRouteHosts);
}

function normalizeLANAddress(value: string): string {
  const address = normalizeRouteHost(value);
  if (isPrivateIPv4(address) || isLocalIPv6(address)) return address;
  throw new PorticoLANDiscoveryRecordError("Discovery records may only advertise local network addresses.");
}

function normalizeRouteHost(value: string): string {
  let host = value.trim().toLowerCase();
  if (host.startsWith("[") && host.endsWith("]")) host = host.slice(1, -1);
  if (!host || /[\s/?#]/.test(host)) throw new PorticoLANDiscoveryRecordError("A discovery route host is invalid.");
  return host;
}

function isPrivateIPv4(value: string): boolean {
  const parts = value.split(".");
  if (parts.length !== 4 || parts.some((part) => !/^\d{1,3}$/.test(part) || Number(part) > 255)) return false;
  const [a, b] = parts.map(Number);
  return a === 10 || a === 127 || (a === 192 && b === 168) || (a === 172 && b >= 16 && b <= 31) || (a === 169 && b === 254);
}

function isRFC1918IPv4(value: string): boolean {
  const parts = value.split(".");
  if (parts.length !== 4 || parts.some((part) => !/^\d{1,3}$/.test(part) || Number(part) > 255)) return false;
  const [a, b] = parts.map(Number);
  return a === 10 || (a === 192 && b === 168) || (a === 172 && b >= 16 && b <= 31);
}

function isLocalIPv6(value: string): boolean {
  const [address, scope, ...extra] = value.split("%");
  if (extra.length > 0 || (scope !== undefined && !/^[A-Za-z0-9_.-]+$/.test(scope))) return false;
  if (!validIPv6Address(address)) return false;
  return address === "::1" || /^f[cd][0-9a-f]{2}:/i.test(address) || /^fe[89ab][0-9a-f]:/i.test(address);
}

function formatURLHost(host: string): string {
  return host.includes(":") ? `[${host.replace("%", "%25")}]` : host;
}

function validIPv6Address(value: string): boolean {
  if (!/^[0-9a-f:]+$/i.test(value) || value.includes(":::")) return false;
  const compressed = value.includes("::");
  if (compressed && value.indexOf("::") !== value.lastIndexOf("::")) return false;
  const groups = value.split(":").filter(Boolean);
  if (groups.some((group) => group.length > 4)) return false;
  return compressed ? groups.length < 8 : groups.length === 8;
}

function compareRouteHosts(left: string, right: string): number {
  const rank = (host: string) => host.includes(":") ? 1 : host.endsWith(".local") ? 2 : 0;
  return rank(left) - rank(right) || left.localeCompare(right);
}

function cleanLabel(value: string, field: string): string {
  const cleaned = cleanOptionalLabel(value);
  if (!cleaned) throw new PorticoLANDiscoveryRecordError(`The discovery ${field} is required.`);
  return cleaned;
}

function cleanOptionalLabel(value: string | undefined): string | undefined {
  const cleaned = value?.trim().replace(/[\u0000-\u001f\u007f]/g, "").slice(0, 128);
  return cleaned || undefined;
}
