const TOKEN_KEY = 'vdcat_token'

export const tokenStore = {
  get: () => localStorage.getItem(TOKEN_KEY) || '',
  set: (t) => localStorage.setItem(TOKEN_KEY, t),
  clear: () => localStorage.removeItem(TOKEN_KEY),
}

async function request(path, options = {}) {
  const headers = { 'Content-Type': 'application/json' }
  const tok = tokenStore.get()
  if (tok) headers['Authorization'] = `Bearer ${tok}`

  const res = await fetch(path, { headers, ...options })
  let body
  try {
    body = await res.json()
  } catch {
    if (res.status === 401) {
      tokenStore.clear()
      throw new Error('未授权或登录已过期')
    }
    throw new Error('服务器响应异常')
  }
  if (res.status === 401) {
    tokenStore.clear()
    throw new Error(body.error || '未授权或登录已过期')
  }
  if (!res.ok || !body.ok) {
    throw new Error(body.error || `请求失败 (${res.status})`)
  }
  return body.data
}

// 远程视频/图片直链转同源代理地址（抖音/快手 CDN 在浏览器直连会被防盗链拦截）。
// <video>/<img> 无法携带 Authorization 头，通过 query token 鉴权。
export function mediaSrc(remoteUrl) {
  if (!remoteUrl) return ''
  // 已是本站相对地址则不再代理
  if (remoteUrl.startsWith('/')) return remoteUrl
  return `/api/media?url=${encodeURIComponent(remoteUrl)}&token=${encodeURIComponent(tokenStore.get())}`
}

export const api = {
  health: () => request('/api/health'),
  login: (data) => request('/api/login', { method: 'POST', body: JSON.stringify(data) }),
  getConfig: () => request('/api/config'),
  saveConfig: (data) => request('/api/config', { method: 'POST', body: JSON.stringify(data) }),
  parse: (url, platform) =>
    request('/api/parse', { method: 'POST', body: JSON.stringify({ url, platform }) }),
  download: (data) => request('/api/download', { method: 'POST', body: JSON.stringify(data) }),
  listCookies: (platform) =>
    request(`/api/cookies${platform ? `?platform=${encodeURIComponent(platform)}` : ''}`),
  createCookie: (data) => request('/api/cookies', { method: 'POST', body: JSON.stringify(data) }),
  updateCookie: (id, data) => request(`/api/cookies/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  toggleCookie: (id, enabled) =>
    request(`/api/cookies/${id}/toggle`, { method: 'POST', body: JSON.stringify({ enabled }) }),
  deleteCookie: (id) => request(`/api/cookies/${id}`, { method: 'DELETE' }),
  downloadDirect: (url, referer) =>
    request('/api/download-direct', { method: 'POST', body: JSON.stringify({ url, referer }) }),
  activeTasks: () => request('/api/tasks/active'),
  history: (page = 1, size = 10, filters = {}) => {
    const q = new URLSearchParams({ page, size })
    if (filters.platform) q.set('platform', filters.platform)
    if (filters.group_id) q.set('group_id', filters.group_id)
    return request(`/api/history?${q.toString()}`)
  },
  historyGroups: (platform = '') =>
    request(`/api/history/groups${platform ? `?platform=${encodeURIComponent(platform)}` : ''}`),
  deleteGroup: (id, withFile) =>
    request(`/api/groups/${id}${withFile ? '?file=1' : ''}`, { method: 'DELETE' }),
  cancel: (id) => request(`/api/tasks/${id}/cancel`, { method: 'POST' }),
  retry: (id) => request(`/api/tasks/${id}/retry`, { method: 'POST' }),
  deleteTask: (id, withFile) =>
    request(`/api/tasks/${id}${withFile ? '?file=1' : ''}`, { method: 'DELETE' }),
  files: () => request('/api/files'),
  deleteFile: (path) =>
    request(`/api/files?path=${encodeURIComponent(path)}`, { method: 'DELETE' }),
  rawURL: (path) => `/api/files/raw?path=${encodeURIComponent(path)}&token=${encodeURIComponent(tokenStore.get())}`,

  // API Key（浏览器扩展接入）管理
  listAPIKeys: () => request('/api/api-keys'),
  createAPIKey: (name) =>
    request('/api/api-keys', { method: 'POST', body: JSON.stringify({ name }) }),
  toggleAPIKey: (id, enabled) =>
    request(`/api/api-keys/${id}/toggle`, { method: 'POST', body: JSON.stringify({ enabled }) }),
  rotateAPIKey: (id) =>
    request(`/api/api-keys/${id}/rotate`, { method: 'POST' }),
  deleteAPIKey: (id) =>
    request(`/api/api-keys/${id}`, { method: 'DELETE' }),
}
