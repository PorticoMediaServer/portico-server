/** Both existing explicit build switches are required. Host names never enable a different app. */
export function servicesBuildMode(env: {VITE_HOSTED_WEB?: string; VITE_ACCOUNT_WORKSPACE?: string}) {
  return env.VITE_HOSTED_WEB === 'true' && env.VITE_ACCOUNT_WORKSPACE === 'true';
}
export const servicesPaths = ['/', '/account', '/device', '/reset', '/reset-password', '/verify-email', '/claim', '/join'] as const;
