const TOKEN_KEY = 'eino_rag_token'
const USER_KEY = 'eino_rag_auth_user'
const TENANT_KEY = 'eino_rag_auth_tenant'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setAuth(payload) {
  if (payload?.token) {
    localStorage.setItem(TOKEN_KEY, payload.token)
  }
  if (payload?.user) {
    localStorage.setItem(USER_KEY, JSON.stringify(payload.user))
  }
  if (payload?.tenant) {
    localStorage.setItem(TENANT_KEY, JSON.stringify(payload.tenant))
  }
}

export function clearAuth() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
  localStorage.removeItem(TENANT_KEY)
}

export function getAuthUser() {
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || 'null')
  } catch {
    return null
  }
}

export function getAuthTenant() {
  try {
    return JSON.parse(localStorage.getItem(TENANT_KEY) || 'null')
  } catch {
    return null
  }
}

export function isLoggedIn() {
  return !!getToken()
}

/** 仅 default 租户下的 admin 可创建租户 */
export function isPlatformAdmin() {
  const user = getAuthUser()
  const tenant = getAuthTenant()
  return tenant?.code === 'default' && user?.username === 'admin'
}

/** 当前登录租户下用户名为 admin 的账号 */
export function isTenantAdmin() {
  return getAuthUser()?.username === 'admin'
}
