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
    // Array form, because order matters: the bare '@' alias is a prefix of
    // '@unownhash/...' and would otherwise claim it first. Listing the
    // package name ahead of it keeps the workspace resolving base-ui by the
    // same specifier a consumer installing it from the registry would use --
    // for stylesheets as well as modules, which the tsconfig paths plugin
    // does not cover.
    alias: [
      {
        find: /^@unownhash\/rotom-base-ui$/,
        replacement: path.resolve(__dirname, '../../libs/base-ui/src/index.ts'),
      },
      {
        find: /^@unownhash\/rotom-base-ui\//,
        replacement: path.resolve(__dirname, '../../libs/base-ui/src') + '/',
      },
      { find: /^@\//, replacement: path.resolve(__dirname, '../../libs/base-ui/src') + '/' },
      // Both the bare specifier and its subpaths (react-dom/client,
      // react/jsx-runtime), so every import lands in the one copy.
      { find: /^react$/, replacement: path.resolve(__dirname, '../../node_modules/react') },
      { find: /^react\//, replacement: path.resolve(__dirname, '../../node_modules/react') + '/' },
      { find: /^react-dom$/, replacement: path.resolve(__dirname, '../../node_modules/react-dom') },
      {
        find: /^react-dom\//,
        replacement: path.resolve(__dirname, '../../node_modules/react-dom') + '/',
      },
    ],
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
