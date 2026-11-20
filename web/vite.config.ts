import { defineConfig, loadEnv } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = { ...loadEnv(mode, process.cwd(), 'VITE_'), ...process.env }
  const apiProxy = env.VITE_API_PROXY ?? 'http://127.0.0.1:8000'
  // Large uploads may need a host that bypasses the API proxy's body limit.
  const uploadProxy = env.VITE_UPLOAD_PROXY ?? apiProxy
  // changeOrigin sends the target's Host header so a remote reverse proxy matches its site.
  const proxyTo = (target: string) => ({ target, changeOrigin: true })

  return {
    plugins: [
      react(),
      babel({ presets: [reactCompilerPreset()] })
    ],
    server: {
      proxy: {
        '/api/v1/videos/upload': proxyTo(uploadProxy),
        '/api': proxyTo(apiProxy),
        '/media': proxyTo(apiProxy),
      },
    },
  }
})
