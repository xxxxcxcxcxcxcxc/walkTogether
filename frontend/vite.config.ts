import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', 'VITE_')
  const tunnelHost = env.VITE_TUNNEL_HOST?.trim()

  return {
    plugins: [react()],
    server: {
      port: 3000,
      host: true,
      allowedHosts: tunnelHost ? [tunnelHost] : [],
      proxy: {
        '/api': {
          target: 'http://127.0.0.1:8080',
          changeOrigin: true,
        },
      },
    },
    test: {
      environment: 'jsdom',
      include: ['src/**/*.test.{ts,tsx}'],
    },
  }
})