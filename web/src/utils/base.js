/** 部署 / 代理路径前缀，始终以 / 结尾（Vite BASE_URL）。 */
export const BASE_URL = import.meta.env.BASE_URL || '/'

function stripTrailingSlash(s) {
  return String(s || '').replace(/\/+$/, '')
}

function isPrivateHost(hostname) {
  const host = String(hostname || '')
    .toLowerCase()
    .replace(/^\[|\]$/g, '')
  if (host === 'localhost' || host === '127.0.0.1' || host === '::1' || host.endsWith('.local')) {
    return true
  }
  if (/^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)) return true
  if (/^192\.168\.\d{1,3}\.\d{1,3}$/.test(host)) return true
  if (/^172\.(1[6-9]|2\d|3[0-1])\.\d{1,3}\.\d{1,3}$/.test(host)) return true
  return false
}

/**
 * 公网页面请求内网 API 会被浏览器 Private Network Access 拦截，axios 表现为 Network Error。
 * 此时改走当前站点同源，由 nginx 反代 /api。
 */
function isPublicPageToPrivateApi(base) {
  if (typeof window === 'undefined') return false
  try {
    const apiHost = new URL(base, window.location.href).hostname
    return !isPrivateHost(window.location.hostname) && isPrivateHost(apiHost)
  } catch {
    return false
  }
}

/**
 * API 根路径（无尾斜杠）。
 * 设置了 VITE_API_BASE 时用之（可含协议）；否则跟站点前缀走同源代理。
 */
export function getApiBase() {
  const explicit = import.meta.env.VITE_API_BASE
  if (explicit != null && String(explicit) !== '') {
    const base = stripTrailingSlash(explicit)
    if (isPublicPageToPrivateApi(base)) return stripTrailingSlash(BASE_URL)
    return base
  }
  return stripTrailingSlash(BASE_URL)
}

/** 拼站点前缀：`login` → `/rag/login` 或 `/login`。 */
export function withBase(path) {
  const rel = String(path || '').replace(/^\//, '')
  return `${BASE_URL}${rel}`
}

/** 当前浏览器地址对应的应用内路径（不含代理前缀）+ search。 */
export function appLocation() {
  const prefix = stripTrailingSlash(BASE_URL)
  let path = window.location.pathname
  if (prefix && (path === prefix || path.startsWith(`${prefix}/`))) {
    path = path.slice(prefix.length) || '/'
  }
  return path + window.location.search
}
