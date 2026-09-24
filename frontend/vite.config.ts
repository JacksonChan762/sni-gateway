import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import thirdPartyNotices from './build/third-party-notices.mjs'

export default defineConfig({
  plugins: [react(), thirdPartyNotices()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      }
    }
  },
  build: {
    outDir: '../backend/public',
    emptyOutDir: true,
  }
})
