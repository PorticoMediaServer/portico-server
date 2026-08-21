/**
 * React Native and other native TypeScript client entrypoint.
 *
 * Native applications provide secure credentials, fetch/event-stream,
 * cryptography, timers, and LAN discovery through the exported adapter
 * contracts. Browser-only playback capability detection is not exported here.
 */
export * from "./core.js";
export * from "./client.js";
export * from "./hostedConnectionRuntime.js";
export * from "./hostedServerConnection.js";
export * from "./localDiscovery.js";
export * from "./trustedServerConnection.js";
