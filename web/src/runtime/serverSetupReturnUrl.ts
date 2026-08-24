function loopbackHostname(hostname: string): boolean {
  const normalized = hostname.toLowerCase().replace(/^\[|\]$/g, '');
  return normalized === 'localhost' || normalized === '127.0.0.1' || normalized === '::1';
}

export function localHTTPHostname(hostname: string): boolean {
  if (loopbackHostname(hostname)) return true;
  const octets = hostname.split('.');
  if (octets.length !== 4 || octets.some((octet) => !/^\d{1,3}$/.test(octet))) return false;
  const values = octets.map(Number);
  if (values.some((octet) => octet < 0 || octet > 255)) return false;
  return values[0] === 10
    || (values[0] === 172 && values[1]! >= 16 && values[1]! <= 31)
    || (values[0] === 192 && values[1] === 168);
}

export function validServerSetupReturnUrl(value: unknown): value is string {
  if (typeof value !== 'string' || !value.trim() || value.length > 2048) return false;
  try {
    const target = new URL(value);
    if (target.username || target.password || target.hash || !localHTTPHostname(target.hostname)) return false;
    if (target.protocol !== 'http:' && target.protocol !== 'https:') return false;
    return target.pathname === '/'
      && [...target.searchParams.keys()].length === 1
      && target.searchParams.get('porticoSetup') === 'continue';
  } catch {
    return false;
  }
}
