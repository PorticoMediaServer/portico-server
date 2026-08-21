export type RuntimeMode = "bundled" | "hosted";
export type WebUiVariant = "v3";

export interface RuntimeCapabilities {
  mode: RuntimeMode;
  canUsePorticoAccountAuth: boolean;
  canUseLocalAuth: boolean;
  canRunFirstRunSetup: boolean;
  canUseLocalOnlyMode: boolean;
  canManagePorticoInvites: boolean;
  canManageLocalUsers: boolean;
  canClaimServer: boolean;
  canUseLANDiscovery: boolean;
  canOpenLocalRecovery: boolean;
  canUseDirectPorticoRoutes: boolean;
}

export function runtimeCapabilitiesForMode(mode: RuntimeMode): RuntimeCapabilities {
  if (mode === "hosted") {
    return {
      mode,
      canUsePorticoAccountAuth: true,
      canUseLocalAuth: false,
      canRunFirstRunSetup: false,
      canUseLocalOnlyMode: false,
      canManagePorticoInvites: true,
      canManageLocalUsers: false,
      canClaimServer: true,
      canUseLANDiscovery: false,
      canOpenLocalRecovery: false,
      canUseDirectPorticoRoutes: true
    };
  }
  return {
    mode,
    canUsePorticoAccountAuth: true,
    canUseLocalAuth: true,
    canRunFirstRunSetup: true,
    canUseLocalOnlyMode: true,
    canManagePorticoInvites: true,
    canManageLocalUsers: true,
    canClaimServer: true,
    canUseLANDiscovery: true,
    canOpenLocalRecovery: true,
    canUseDirectPorticoRoutes: false
  };
}

export function hostedDirectRouteAllowed(rawURL: string, capabilities: Pick<RuntimeCapabilities, "canUseDirectPorticoRoutes"> = { canUseDirectPorticoRoutes: true }): boolean {
  if (!capabilities.canUseDirectPorticoRoutes) return true;
  try {
    const url = new URL(rawURL);
    if (url.protocol !== "https:") return false;
    if (isIPAddress(url.hostname)) return false;
    return url.hostname.endsWith(".direct.getportico.tv");
  } catch {
    return false;
  }
}

export function isIPAddress(hostname: string): boolean {
  return /^\d{1,3}(?:\.\d{1,3}){3}$/.test(hostname) || hostname.includes(":");
}
