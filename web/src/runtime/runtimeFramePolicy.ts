import type { RuntimeState } from './runtimeMachine';

/**
 * Identity/setup flows own the whole viewport. Ordinary connection work and
 * server recovery belong inside the signed-in product frame. This is a
 * presentation decision only; it never treats cached chrome as authorization.
 */
export function runtimeUsesProductFrame(state: RuntimeState): boolean {
  switch (state.id) {
    case 'checking-local-server':
    case 'hosted-account-session':
    case 'server-memberships':
    case 'route-discovery':
      return true;
    case 'runtime-recovery':
      return !['route-security', 'session-expired'].includes(state.classification);
    default:
      return false;
  }
}

export function runtimeFrameServerName(state: RuntimeState): string | undefined {
  if (state.id === 'checking-local-server') return state.serverName;
  if (state.id === 'route-discovery' || state.id === 'profile-selection') return state.selectedServer.name;
  if (state.id === 'runtime-recovery') return state.serverName ?? state.selectedServer?.name;
  if (state.id === 'server-ready') return state.serverName;
  return undefined;
}
