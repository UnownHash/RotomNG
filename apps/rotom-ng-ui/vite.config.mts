/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import tailwindcss from '@tailwindcss/vite';
import { nxViteTsPaths } from '@nx/vite/plugins/nx-tsconfig-paths.plugin';
import * as path from 'node:path';

export default defineConfig({
  root: __dirname,
  cacheDir: '../../node_modules/.vite/apps/rotom-ng-ui',
  plugins: [react(), tailwindcss(), nxViteTsPaths()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '../../libs/base-ui/src'),
      react: path.resolve(__dirname, '../../node_modules/react'),
      'react-dom': path.resolve(__dirname, '../../node_modules/react-dom'),
    },
    dedupe: ['react', 'react-dom'],
  },
  optimizeDeps: {
    include: ['react', 'react-dom', 'react/jsx-runtime'],
  },
  server: {
    port: 4201,
    host: '0.0.0.0',
    fs: {
      allow: [path.resolve(__dirname, '../..')],
    },
    proxy: {
      '/api': {
        target: 'http://localhost:7072',
        changeOrigin: true,
        secure: false,
      },
    },
  },
  preview: {
    port: 4203,
    host: '0.0.0.0',
  },
  build: {
    outDir: '../../libs/rotom_ui/static',
    emptyOutDir: true,
    sourcemap: false,
    reportCompressedSize: false,
  },
});
