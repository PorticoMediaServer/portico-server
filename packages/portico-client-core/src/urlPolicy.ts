export type PorticoUrlPurpose = "trusted-server-route" | "lan-server-route" | "download-grant";

export type PorticoRouteTransport = "https" | "lan-http";

export type PorticoUrlPolicyOptions = {
  /** Trusted origin used to resolve a route-bound relative download URL. */
  trustedOrigin?: string;
};

const serverFingerprintPattern = /^sha256:[A-Za-z0-9_-]{43}$/u;

/**
 * Central URL boundary for security-sensitive URLs received from Portico services.
 * Route URLs are absolute HTTPS origins. Download grants may be relative, but an
 * absolute grant must remain on the already verified route origin.
 */
export function validatePorticoUrl(raw: string, purpose: PorticoUrlPurpose, options: PorticoUrlPolicyOptions = {}): string {
  if (typeof raw !== "string" || raw !== raw.trim() || !raw || /[\u0000-\u001f\u007f]/u.test(raw)) {
    throw new TypeError(`The ${purpose} URL is invalid.`);
  }
  if (raw.startsWith("//") || raw.includes("\\")) throw new TypeError(`The ${purpose} URL is unsafe.`);

  if (purpose === "trusted-server-route" || purpose === "lan-server-route") {
    const url = parseAbsolute(raw, purpose);
    const validScheme = url.protocol === "https:" || purpose === "lan-server-route" && url.protocol === "http:" && isLocalNetworkHost(url.hostname);
    if (!validScheme || url.username || url.password || url.search || url.hash || !url.hostname) {
      throw new TypeError(purpose === "lan-server-route" ? "A LAN server route must be a clean HTTP or HTTPS URL." : "A trusted server route must be a clean HTTPS URL.");
    }
    return url.href.replace(/\/$/, "");
  }

  const trustedOrigin = options.trustedOrigin ? grantBindingOrigin(options.trustedOrigin) : undefined;
  const absolute = /^[a-z][a-z0-9+.-]*:/iu.test(raw);
  if (!absolute && !raw.startsWith("/")) throw new TypeError("A download grant URL must be absolute or root-relative.");
  if (!absolute && !trustedOrigin) {
    const relative = new URL(raw, "https://portico.invalid");
    if (relative.hash) throw new TypeError("The download grant URL is unsafe.");
    return `${relative.pathname}${relative.search}`;
  }
  const url = new URL(raw, trustedOrigin);
  const allowedProtocol = trustedOrigin ? new URL(trustedOrigin).protocol : "https:";
  if (url.protocol !== allowedProtocol || url.username || url.password || url.hash ||
      url.protocol === "http:" && !isLocalNetworkHost(url.hostname)) {
    throw new TypeError("The download grant URL is unsafe.");
  }
  if (trustedOrigin && url.origin !== trustedOrigin) throw new TypeError("The download grant URL is not bound to the trusted server route.");
  return absolute ? url.href : `${url.pathname}${url.search}`;
}

/** Returns the transport class accepted for a server route. */
export function porticoRouteTransport(
  raw: string,
  purpose: Exclude<PorticoUrlPurpose, "download-grant">
): PorticoRouteTransport {
  const normalized = validatePorticoUrl(raw, purpose);
  return new URL(normalized).protocol === "http:" ? "lan-http" : "https";
}

export function isValidPorticoServerPublicKeyFingerprint(value: unknown): value is string {
  return typeof value === "string" && serverFingerprintPattern.test(value.trim());
}

function isLocalNetworkHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  if (host === "localhost" || host.endsWith(".local") || host === "::1" || host.startsWith("fe80:")) return true;
  const octets = host.split(".").map(Number);
  if (octets.length !== 4 || octets.some(value => !Number.isInteger(value) || value < 0 || value > 255)) return false;
  return octets[0] === 10 || octets[0] === 127 || octets[0] === 169 && octets[1] === 254 ||
    octets[0] === 192 && octets[1] === 168 || octets[0] === 172 && octets[1] >= 16 && octets[1] <= 31;
}

export function trustedRouteOrigin(raw: string): string {
  return new URL(validatePorticoUrl(raw, "trusted-server-route")).origin;
}

function grantBindingOrigin(raw: string): string {
  const route = parseAbsolute(raw, "download-grant");
  if (route.username || route.password || route.search || route.hash || !route.hostname) {
    throw new TypeError("The trusted server route is not a clean origin.");
  }
  if (route.protocol !== "https:" && !(route.protocol === "http:" && isLocalNetworkHost(route.hostname))) {
    throw new TypeError("A download grant must be bound to HTTPS or a local LAN route.");
  }
  return route.origin;
}

function parseAbsolute(raw: string, purpose: PorticoUrlPurpose): URL {
  try {
    const url = new URL(raw);
    if (!url.origin || url.origin === "null") throw new TypeError();
    return url;
  } catch {
    throw new TypeError(`The ${purpose} URL is invalid.`);
  }
}
