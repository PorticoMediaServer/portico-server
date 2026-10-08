import type { PorticoApi } from './api';
import { fixtureApi } from './fixture/api';
import { createHttpApi } from './http';

/**
 * The active backend. Set VITE_PORTICO_SERVER=https://your-server:32500 to talk to a
 * real Portico server over HTTP; otherwise the bundled fixture household is used.
 */
const server = import.meta.env.VITE_PORTICO_SERVER as string | undefined;
export const api: PorticoApi = server ? createHttpApi(server) : fixtureApi;
export const usingFixtures = !server;
