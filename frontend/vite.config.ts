import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development the API runs on :8080; in production nginx serves both under one origin.
export default defineConfig({
  plugins: [react()],
  server: { port: 5173, proxy: { '/api': process.env.API_URL || 'http://localhost:8080' } },
  // onnxruntime-web finds its .wasm next to its own file; pre-bundling would move the JS away from it
  optimizeDeps: { exclude: ['onnxruntime-web', 'onnxruntime-web/wasm'] },
  // the face engine (TensorFlow + ONNX runtime) is large by nature and only loads on the enrolment / kiosk screens
  build: { sourcemap: false, chunkSizeWarningLimit: 1600 },
})
