import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    // Forces Vite to prioritize modern browser module formats
    mainFields: ['module', 'main']
  }
})
