import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/ping': 'http://localhost:8080',
    },
  },
  resolve: {
    // Forces Vite to prioritize modern browser module formats
    mainFields: ['module', 'main']
  }
})
