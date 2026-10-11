/* VDCat 扩展设置页：配置服务地址与 API Key，并支持连通性测试。 */
'use strict'

const DEFAULTS = { baseUrl: 'http://127.0.0.1:8626', apiKey: '' }

const el = {
  form: document.getElementById('form'),
  baseUrl: document.getElementById('base-url'),
  apiKey: document.getElementById('api-key'),
  btnTest: document.getElementById('btn-test'),
  msg: document.getElementById('msg'),
}

function normalize(u) {
  return (u || '').trim().replace(/\/+$/, '')
}

function setMsg(text, ok) {
  el.msg.textContent = text || ''
  el.msg.className = 'msg' + (ok === true ? ' ok' : ok === false ? ' err' : '')
}

async function load() {
  const cfg = await chrome.storage.sync.get(DEFAULTS)
  el.baseUrl.value = cfg.baseUrl || ''
  el.apiKey.value = cfg.apiKey || ''
}

async function save() {
  const baseUrl = normalize(el.baseUrl.value)
  const apiKey = el.apiKey.value.trim()
  if (!baseUrl || !/^https?:\/\//i.test(baseUrl)) {
    setMsg('请输入合法的服务地址（http:// 或 https:// 开头）', false)
    return null
  }
  if (!apiKey) {
    setMsg('请输入 API Key', false)
    return null
  }
  await chrome.storage.sync.set({ baseUrl, apiKey })
  return { baseUrl, apiKey }
}

async function testConnection(cfg) {
  setMsg('正在测试…')
  let health
  try {
    const res = await fetch(cfg.baseUrl + '/api/health', { method: 'GET' })
    health = await res.json()
  } catch (err) {
    setMsg('无法连接服务：' + (err.message || '网络错误'), false)
    return
  }
  // Key 无效时外部任务接口返回 401
  try {
    const res = await fetch(cfg.baseUrl + '/api/external/tasks', {
      method: 'GET',
      headers: { 'X-Api-Key': cfg.apiKey },
    })
    const body = await res.json().catch(() => null)
    if (res.ok && body && body.ok) {
      setMsg('连接成功，API Key 有效 ✓', true)
    } else {
      setMsg((body && body.error) || 'API Key 无效或已停用（HTTP ' + res.status + '）', false)
    }
  } catch (err) {
    setMsg('服务可达，但鉴权请求失败：' + err.message, false)
  }
}

el.form.addEventListener('submit', async (e) => {
  e.preventDefault()
  const cfg = await save()
  if (cfg) setMsg('已保存 ✓', true)
})

el.btnTest.addEventListener('click', async () => {
  const cfg = await save()
  if (cfg) testConnection(cfg)
})

load()
