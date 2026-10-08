import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import {fileURLToPath} from 'node:url';

export default defineConfig({
  plugins: [react()],
  resolve: {alias: {'@': fileURLToPath(new URL('./src', import.meta.url))}},
  server: {strictPort: true},
  build: {
    sourcemap: true,
    // The fixture catalogue (about 1.5 MB, 200 kB gzipped) is one chunk, loaded behind the start-up screen.
    chunkSizeWarningLimit: 1600,
  },
});
