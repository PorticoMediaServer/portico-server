/**
 * What the server says about the connection this app is using to reach it
 * (`GET /v1/networking/connection`, open to anyone who can reach the server). The server alone
 * decides what counts as outside the home: the owner's home networks setting is the one
 * definition, and the app never guesses from an address.
 */
export type ConnectionFacts = Readonly<{
  /** The request arrived encrypted. */
  secure: boolean;
  /** The request came from outside the owner's home networks. */
  remote: boolean;
  /** Plain HTTP from outside the home networks: worth one quiet line on the sign-in screen, never a block. */
  plainHttpOutsideHome: boolean;
}>;

export interface ConnectionApi { request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> }

/** Reads the facts tolerantly: a field that is missing or not a yes/no reads as "nothing to say". */
export function parseConnectionFacts(value: unknown): ConnectionFacts {
  const v = typeof value === 'object' && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : {};
  const secure = v.secure === true;
  const remote = v.remote === true;
  return Object.freeze({secure, remote, plainHttpOutsideHome: v.plainHttpOutsideHome === true && !secure});
}

/**
 * Whether to show the quiet plain-HTTP line. It is only ever a note: any failure to find out
 * (an older server, a timeout, an unreadable answer) means no note, and nothing waits on it.
 */
export async function plainHttpOutsideHome(api: ConnectionApi, signal?: AbortSignal): Promise<boolean> {
  try {
    return parseConnectionFacts(await api.request<unknown>('/v1/networking/connection', 'GET', undefined, signal)).plainHttpOutsideHome;
  } catch {
    return false;
  }
}

/** Where the note's link goes: how an owner turns on an encrypted connection. */
export const SECURE_CONNECTION_HELP_URL = 'https://getportico.tv/docs/remote/enable-remote-access';
