import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

/** 规范化为 Vite base：始终以 / 开头并以 / 结尾。 */
function normalizeBase(raw) {
  let b = String(raw ?? '/').trim()
  if (!b) b = '/'
  if (!b.startsWith('/')) b = `/${b}`
  if (!b.endsWith('/')) b += '/'
  return b.replace(/\/{2,}/g, '/')
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const base = normalizeBase(
    process.env.VITE_BASE ||
      env.VITE_BASE ||
      process.env.VITE_PROXY_PREFIX ||
      env.VITE_PROXY_PREFIX ||
      '/',
  )
  const prefix = base === '/' ? '' : base.slice(0, -1)
  const rewrite = prefix
    ? (path) => (path.startsWith(prefix) ? path.slice(prefix.length) || '/' : path)
    : undefined

  const proxy = {
    [`${prefix}/api`]: {
      target: 'http://localhost:8080',
      changeOrigin: true,
      timeout: 120000,
      proxyTimeout: 120000,
      rewrite,
    },
    [`${prefix}/health`]: {
      target: 'http://localhost:8080',
      changeOrigin: true,
      rewrite,
    },
  }

  return {
    base,
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: true,
      port: 5173,
      allowedHosts: true,
      proxy,
    },
    preview: {
      proxy,
    },
  }
})
