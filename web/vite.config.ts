import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import {fileURLToPath} from 'node:url';
const at = (p: string) => fileURLToPath(new URL(p, import.meta.url));
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {'@core': at('../packages/client-core/src'), '@design': at('../packages/design/src/index.ts'), '@i18n': at('../packages/i18n/src/index.ts'), '@': at('./src')},
    dedupe: ['react', 'react-dom'],
  },
  server: {
    strictPort: true,
    fs: {allow: ['../..']},
    proxy: {
      '/v1': {target: process.env.PORTICO_DEV_SERVER_URL ?? 'http://127.0.0.1:32500', changeOrigin: false},
      '/v2': {target: process.env.PORTICO_DEV_SERVER_URL ?? 'http://127.0.0.1:32500', changeOrigin: false},
    },
  },
  build: {sourcemap: true},
});
