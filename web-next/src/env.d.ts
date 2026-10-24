/// <reference types="vite/client" />
interface ImportMetaEnv {
  readonly VITE_HOSTED_WEB?: string;
  readonly VITE_ACCOUNT_WORKSPACE?: string;
  readonly VITE_HOSTED_URL?: string;
  readonly VITE_APP_VERSION?: string;
  readonly VITE_CAST_APPLICATION_ID?: string;
  readonly VITE_SERVER_URL?: string;
}
declare module 'virtual:portico-kit-families' {
  const loaders: readonly ((() => Promise<{default: import('./dev/gallery-registry').GalleryFamily}>) & {readonly familyId: string})[];
  export default loaders;
}
