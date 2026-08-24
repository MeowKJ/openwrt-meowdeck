import { defineConfig } from 'vite'

export default defineConfig({
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:9080',
      '/healthz': 'http://127.0.0.1:9080',
    },
  },
})
