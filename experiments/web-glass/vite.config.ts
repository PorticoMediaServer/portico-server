import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: { port: 5404, strictPort: true },
  preview: { port: 5414, strictPort: true },
  build: {
    target: 'es2022',
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('react-router')) return 'router';
            if (id.includes('@tanstack')) return 'query';
            if (id.includes('lucide')) return 'icons';
            if (id.includes('react')) return 'react';
          }
          if (id.includes('/src/data/fixtures/')) return 'fixtures';
          return undefined;
        },
      },
    },
  },
});
