/// <reference types="vite/client" />

interface Window {
  __PORTICO_CONFIG__?: import('./runtime/runtimeMachine').RuntimeBootstrapConfig;
}
