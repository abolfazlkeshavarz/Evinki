import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development the API runs on :8080; in production nginx serves both under one origin.
export default defineConfig({
  plugins: [react()],
  server: { port: 5173, proxy: { '/api': process.env.API_URL || 'http://localhost:8080' } },
  build: { sourcemap: false, chunkSizeWarningLimit: 900 },
})
