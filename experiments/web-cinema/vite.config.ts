import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {port: 5402, strictPort: true, host: '127.0.0.1'},
  preview: {port: 5412, strictPort: true, host: '127.0.0.1'},
  // The main chunk carries the fixture catalogue (~450 kB of JSON); real services remove it.
  build: {target: 'es2022', chunkSizeWarningLimit: 1000},
});
