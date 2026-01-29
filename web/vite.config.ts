import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/stream': {
        target: 'http://localhost:2020',
        changeOrigin: true,
      },
      '/start': {
        target: 'http://localhost:2020',
        changeOrigin: true,
      },
      '/message': {
        target: 'http://localhost:2020',
        changeOrigin: true,
      },
    },
  },
})
