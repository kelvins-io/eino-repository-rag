/** 部署 / 代理路径前缀，始终以 / 结尾（Vite BASE_URL）。 */
export const BASE_URL = import.meta.env.BASE_URL || '/'

function stripTrailingSlash(s) {
  return String(s || '').replace(/\/+$/, '')
}

/**
 * API 根路径（无尾斜杠）。
 * 设置了 VITE_API_BASE 时用之（可含协议）；否则跟站点前缀走同源代理。
 */
export function getApiBase() {
  const explicit = import.meta.env.VITE_API_BASE
  if (explicit != null && String(explicit) !== '') {
    return stripTrailingSlash(explicit)
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
