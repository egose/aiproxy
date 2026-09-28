import path from 'path';

import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  base: '/',
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/_internal/dashboard': {
        target: process.env.AIPROXY_UPSTREAM ?? 'http://localhost:8080',
        changeOrigin: true,
      },
      '/_internal/admin': {
        target: process.env.AIPROXY_UPSTREAM ?? 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  preview: {
    port: 5173,
  },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
  },
});
