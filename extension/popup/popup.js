/* VDCat 浏览器扩展 —— 弹出面板
 * 对接自托管 VDCat 服务的外部接口（X-Api-Key 鉴权）：
 *   POST /api/external/parse    解析
 *   POST /api/external/download 解析并直接入队
 *   GET  /api/external/tasks    活动任务列表
 *   GET  /api/health            服务连通性
 */
'use strict'

const $ = (sel) => document.querySelector(sel)

const els = {
  setupTip: $('#setup-tip'),
  panel: $('#panel'),
  btnOptions: $('#btn-options'),
  btnGoOptions: $('#btn-go-options'),
  input: $('#input-url'),
  platform: $('#sel-platform'),
  cover: $('#chk-cover'),
  segDest: $('#seg-dest'),
  homeMode: $('#chk-home-mode'),
  btnPaste: $('#btn-paste'),
  btnTab: $('#btn-tab'),
  btnClear: $('#btn-clear'),
  btnParse: $('#btn-parse'),
  btnQuick: $('#btn-quick-download'),
  btnBatchPage: $('#btn-batch-page'),
  batchHint: $('#batch-hint'),
  resultCard: $('#result-card'),
  rPlatform: $('#result-platform'),
  rCookie: $('#result-cookie'),
  video: $('#video-preview'),
  img: $('#cover-preview'),
  title: $('#result-title'),
  author: $('#result-author'),
  duration: $('#result-duration'),
  page: $('#result-page'),
  btnDownload: $('#btn-download'),
  btnCopy: $('#btn-copy-video'),
  btnRefresh: $('#btn-refresh-tasks'),
  taskList: $('#task-list'),
  toast: $('#toast'),
}

// ---------- 配置 ----------
const DEFAULTS = { baseUrl: 'http://127.0.0.1:8626', apiKey: '' }

async function getConfig() {
  const obj = await chrome.storage.sync.get(DEFAULTS)
  obj.baseUrl = (obj.baseUrl || '').replace(/\/+$/, '')
  return obj
}

// 下载方式偏好（前端本地持久化）：server=后端队列 local=浏览器本地下载
const DEST_KEY = 'vdcat_dest'
function getDest() {
  const checked = els.segDest.querySelector('input[name="dest"]:checked')
  return checked ? checked.value : 'server'
}
function setDest(value) {
  const radio = els.segDest.querySelector(`input[name="dest"][value="${value}"]`)
  if (radio) radio.checked = true
}

// 经后端媒体代理构造可在浏览器直接下载的地址（CDN 直连有防盗链），
// 携带 query api_key 鉴权（chrome.downloads 无法加自定义头）。
function mediaProxyUrl(cfg, remoteUrl) {
  if (!remoteUrl) return ''
  if (/^https?:\/\//i.test(remoteUrl)) {
    return (
      cfg.baseUrl +
      '/api/media?url=' + encodeURIComponent(remoteUrl) +
      '&api_key=' + encodeURIComponent(cfg.apiKey)
    )
  }
  return cfg.baseUrl + remoteUrl
}

// 从 URL 路径推断扩展名（抖音/快手直链常见 .mp4/.m4s，封面 .jpeg/.webp），兜底 mp4/jpg
function guessExt(url, kind) {
  try {
    const path = new URL(url).pathname.toLowerCase()
    const m = path.match(/\.([a-z0-9]{2,4})(?:$|\?)/)
    if (m && ['mp4', 'm4s', 'mov', 'webm', 'mkv', 'jpg', 'jpeg', 'png', 'webp', 'gif'].includes(m[1])) {
      return m[1] === 'jpeg' ? 'jpg' : m[1]
    }
  } catch {}
  return kind === 'cover' ? 'jpg' : 'mp4'
}

function sanitizeFileName(name) {
  const cleaned = (name || '')
    .replace(/[\\/:*?"<>|\r\n\t]+/g, '_') // 文件系统非法字符
    .replace(/\s+/g, ' ')
    .trim()
  // 按字符截断，避免中文标题超长（与服务端 80 字以内策略一致）
  const r = Array.from(cleaned)
  return (r.length > 80 ? r.slice(0, 80).join('') : cleaned) || 'video'
}

// 文件主名与服务端下载器对齐：作者-标题，标题缺失回退 ID
function fileBaseName(info) {
  const title = (info.title || '').trim()
  const author = (info.author || '').trim()
  let stem = ''
  if (author || title) {
    stem = (author ? author + '-' : '') + (title || info.id || '')
  } else {
    stem = info.id || 'video'
  }
  return sanitizeFileName(stem)
}

// 触发浏览器本地下载（视频，可选封面）
async function downloadLocally(cfg, data) {
  const info = data.info || {}
  if (!info.video_url) throw new Error('缺少视频直链')
  const base = fileBaseName(info)
  const tasks = [
    {
      url: mediaProxyUrl(cfg, info.video_url),
      filename: `VDCat/${base}.${guessExt(info.video_url, 'video')}`,
    },
  ]
  if (els.cover.checked && info.cover_url) {
    tasks.push({
      url: mediaProxyUrl(cfg, info.cover_url),
      filename: `VDCat/${base}_cover.${guessExt(info.cover_url, 'cover')}`,
    })
  }
  // chrome.downloads 需在 manifest 声明 downloads 权限并重新加载扩展后才存在
  if (!chrome.downloads || typeof chrome.downloads.download !== 'function') {
    throw new Error('浏览器下载接口不可用，请在扩展管理页重新加载扩展后重试')
  }
  for (const t of tasks) {
    await new Promise((resolve, reject) => {
      chrome.downloads.download(
        { url: t.url, filename: t.filename, saveAs: false, conflictAction: 'uniquify' },
        (id) => {
          if (chrome.runtime.lastError || id === undefined) {
            reject(new Error(chrome.runtime.lastError ? chrome.runtime.lastError.message : '下载被取消'))
          } else {
            resolve(id)
          }
        }
      )
    })
  }
  return tasks.length
}

// ---------- 通用请求 ----------
async function apiRequest(cfg, path, options = {}) {
  if (!cfg.baseUrl || !cfg.apiKey) {
    throw new Error('未配置服务地址或 API Key')
  }
  const res = await fetch(cfg.baseUrl + path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'X-Api-Key': cfg.apiKey,
      ...(options.headers || {}),
    },
  })
  let body = null
  try {
    body = await res.json()
  } catch {
    throw new Error(`服务器响应异常 (${res.status})`)
  }
  if (!res.ok || !body.ok) {
    const err = new Error(body.error || `请求失败 (${res.status})`)
    err.status = res.status
    throw err
  }
  return body.data
}

// 与后端 platform.ExtractURLs 同规则：仅提取 https 链接，去标点粘连
const HTTPS_RE = /https:\/\/[^\s，。、；“”"'<>（）()[\]{}]+/g
function extractFirstURL(text) {
  if (!text) return ''
  const matches = text.match(HTTPS_RE) || []
  if (!matches.length) return ''
  return matches[0].replace(/[.,;:!?，。、；！？]+$/, '')
}

// 平台域名识别，规则与后端 platform.Detect 对齐
const DOUYIN_HOST_RE = /(?:^|[\./])(douyin|iesdouyin|amemv)\.(?:com|cn)/
const KUAISHOU_HOST_RE = /(?:^|[\./])(kuaishou|chenzhongtech|gifshow)\.(?:com|cn)/

function detectPlatform(url) {
  if (!url) return ''
  if (DOUYIN_HOST_RE.test(url.toLowerCase())) return 'douyin'
  if (KUAISHOU_HOST_RE.test(url.toLowerCase())) return 'kuaishou'
  return ''
}

// 是否快手作者主页：仅当路径为 /profile/<id>。
// 注意：source=PROFILE 查询参数在分享短链/视频页也会出现，不能作为判据。
function isKuaishouProfile(rawUrl) {
  if (!rawUrl) return false
  let u
  try {
    u = new URL(rawUrl)
  } catch {
    return false
  }
  if (!/(^|\.)kuaishou\.com$/.test(u.hostname.toLowerCase())) return false
  return /^\/profile\/[^/]+\/?$/i.test(u.pathname)
}

// 根据输入框当前内容实时判定模式：
//  - 内容提取出的链接是快手作者主页 → 整页批量模式（禁用解析，显示批量按钮）
//  - 其他任何内容（含视频链接）→ 普通解析模式
// 输入变化立即重算，不锁死，因此在主页标签下粘贴视频链接可立即恢复解析。
function refreshMode() {
  const raw = els.input.value.trim()
  const url = extractFirstURL(raw)
  const on = !!url && isKuaishouProfile(url)
  els.btnParse.disabled = on
  els.btnQuick.disabled = on
  els.btnBatchPage.classList.toggle('hidden', !on)
  els.batchHint.classList.toggle('hidden', !on)
  return on
}

// 读取当前活动标签页；无法访问时（如浏览器内置页）返回 null
async function getActiveTab() {
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
    return tab || null
  } catch {
    return null
  }
}

// 打开面板时自动读取当前标签页：仅负责「自动填入」，模式由输入内容经 refreshMode 决定。
// 主页链接或视频链接都填入输入框；用户已有输入则不覆盖。
async function autoFillFromActiveTab() {
  const tab = await getActiveTab()
  if (!tab || !tab.url) return
  if (els.input.value.trim()) {
    refreshMode()
    return
  }
  const url = extractFirstURL(tab.url) || tab.url
  if (!detectPlatform(url) && !isKuaishouProfile(url)) return
  els.input.value = url
  if (!isKuaishouProfile(url)) {
    els.platform.value = detectPlatform(url) || 'auto'
    if (detectPlatform(url)) toast(`已识别当前页为${platformLabel(detectPlatform(url))}，链接自动填入`)
  }
  refreshMode()
}

// ---------- Toast ----------
let toastTimer = null
function toast(msg, type = '') {
  els.toast.textContent = msg
  els.toast.className = 'toast' + (type ? ' ' + type : '')
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => els.toast.classList.add('hidden'), 4000)
  els.toast.classList.remove('hidden')
}

// ---------- 状态 ----------
let lastResult = null // 最近一次解析结果
let taskPollTimer = null
let renderCfg = { baseUrl: '', apiKey: '' } // 供预览/本地下载拼代理地址

function platformLabel(p) {
  return p === 'douyin' ? '抖音' : p === 'kuaishou' ? '快手' : (p || '未知')
}

function fmtDuration(ms) {
  if (!ms || ms <= 0) return ''
  const s = Math.round(ms / 1000)
  const m = Math.floor(s / 60)
  const r = s % 60
  return m > 0 ? `时长 ${m}:${String(r).padStart(2, '0')}` : `时长 ${r}秒`
}

function fmtSpeed(n) {
  if (!n) return ''
  if (n >= 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB/s'
  if (n >= 1024) return (n / 1024).toFixed(0) + ' KB/s'
  return n + ' B/s'
}

function renderResult(data) {
  lastResult = data
  const info = data.info || {}
  els.resultCard.classList.remove('hidden')
  els.rPlatform.textContent = platformLabel(data.platform || info.platform)
  els.rCookie.textContent =
    data.cookie_count > 0
      ? `Cookie ${data.cookie_used || 0}/${data.cookie_count}`
      : '免 Cookie'
  els.title.textContent = info.title || '（无标题）'
  els.author.textContent = info.author ? '作者：' + info.author : ''
  els.duration.textContent = fmtDuration(info.duration)
  if (info.page_url) {
    els.page.textContent = info.page_url
    els.page.href = info.page_url
  } else {
    els.page.textContent = ''
  }
  // 预览：CDN 直连有防盗链，统一经后端媒体代理（renderResult 时 cfg 已可用）
  const coverProxy = info.cover_url ? mediaProxyUrl(renderCfg, info.cover_url) : ''
  if (coverProxy) {
    els.img.src = coverProxy
    els.img.classList.remove('hidden')
  } else {
    els.img.removeAttribute('src')
    els.img.classList.add('hidden')
  }
  els.video.poster = coverProxy || ''
  els.video.src = info.video_url ? mediaProxyUrl(renderCfg, info.video_url) : ''
  syncDownloadUI()
}

// 根据下载方式同步按钮文案
function syncDownloadUI() {
  const local = getDest() === 'local'
  els.btnDownload.textContent = local ? '本地下载' : '加入下载队列'
  els.btnQuick.textContent = local ? '解析并本地下载' : '解析并下载'
  els.btnQuick.title = local ? '解析成功后由浏览器下载到本机' : '解析成功后发送到后端下载队列'
}

function clearResult() {
  lastResult = null
  els.resultCard.classList.add('hidden')
  els.video.pause()
  els.video.removeAttribute('src')
  els.video.load()
  els.img.removeAttribute('src')
}

// ---------- 任务列表 ----------
const STATUS_TEXT = {
  pending: '等待中',
  downloading: '下载中',
  paused: '已暂停',
  done: '已完成',
  failed: '失败',
  canceled: '已取消',
}

async function loadTasks(cfg) {
  let list
  try {
    list = await apiRequest(cfg, '/api/external/tasks')
  } catch (err) {
    // 401（API Key 无效/停用）交给调用方提示并引导设置，不在此吞掉
    if (err && err.status === 401) throw err
    els.taskList.innerHTML = '<li class="empty muted">任务加载失败</li>'
    return false
  }
  renderTasks(Array.isArray(list) ? list : [])
  const active = list.some((t) => t.status === 'downloading' || t.status === 'pending')
  return active
}

function renderTasks(tasks) {
  if (!tasks.length) {
    els.taskList.innerHTML = '<li class="empty muted">暂无活动任务</li>'
    return
  }
  els.taskList.innerHTML = ''
  for (const t of tasks) {
    const li = document.createElement('li')
    li.className = 'task-item'

    const line = document.createElement('div')
    line.className = 'task-line'
    const name = document.createElement('span')
    name.className = 'task-name'
    name.textContent = t.title || t.file_path || t.url || '任务 ' + t.id
    name.title = name.textContent
    const pf = document.createElement('span')
    pf.className = 'task-platform'
    pf.textContent = platformLabel(t.platform)
    line.append(name, pf)

    const statusLine = document.createElement('div')
    statusLine.className = 'task-status' + (t.status === 'failed' ? ' err' : '')
    const pct = Math.max(0, Math.min(100, Math.round((t.progress || 0) * 100)))
    let st = STATUS_TEXT[t.status] || t.status
    if (t.status === 'downloading') {
      st += ` ${pct}%`
      if (t.speed) st += ' · ' + fmtSpeed(t.speed)
    }
    if (t.status === 'failed' && t.error) st += '：' + t.error
    statusLine.textContent = st

    const bar = document.createElement('div')
    bar.className = 'progress'
    const inner = document.createElement('i')
    inner.style.width = pct + '%'
    bar.appendChild(inner)

    li.append(line, statusLine, bar)

    if (t.status === 'failed' && t.group_name) {
      const act = document.createElement('div')
      act.className = 'task-actions'
      const hint = document.createElement('span')
      hint.className = 'muted'
      hint.textContent = '可在 Web 端重试'
      act.appendChild(hint)
      li.appendChild(act)
    }
    els.taskList.appendChild(li)
  }
}

async function startTaskPolling(cfg) {
  clearInterval(taskPollTimer)
  const tick = async () => {
    let active
    try {
      active = await loadTasks(cfg)
    } catch (err) {
      // 401 等鉴权错误：停止轮询，避免每 2 秒重复报错
      clearInterval(taskPollTimer)
      taskPollTimer = null
      return
    }
    if (!active) {
      clearInterval(taskPollTimer)
      taskPollTimer = null
    }
  }
  await tick()
  if (!taskPollTimer) taskPollTimer = setInterval(tick, 2000)
}

// ---------- 操作 ----------
async function doParse(cfg) {
  const raw = els.input.value.trim()
  const url = extractFirstURL(raw)
  if (!url) {
    toast('未发现有效的 https 链接', 'error')
    return null
  }
  els.btnParse.disabled = true
  els.btnQuick.disabled = true
  try {
    const data = await apiRequest(cfg, '/api/external/parse', {
      method: 'POST',
      body: JSON.stringify({ url, platform: els.platform.value }),
    })
    renderResult(data)
    toast('解析成功', 'success')
    return data
  } catch (err) {
    toast(err.message || '解析失败', 'error')
    return null
  } finally {
    els.btnParse.disabled = false
    els.btnQuick.disabled = false
  }
}

async function doQuickDownload(cfg) {
  const raw = els.input.value.trim()
  const url = extractFirstURL(raw)
  if (!url) {
    toast('未发现有效的 https 链接', 'error')
    return
  }
  els.btnQuick.disabled = true
  try {
    // 本地下载：先解析拿直链，再走浏览器下载
    if (getDest() === 'local') {
      const data = await apiRequest(cfg, '/api/external/parse', {
        method: 'POST',
        body: JSON.stringify({ url, platform: els.platform.value }),
      })
      renderResult(data)
      const n = await downloadLocally(cfg, data)
      toast(`已触发本地下载 ${n} 个文件`, 'success')
      return
    }
    // 后端队列
    const data = await apiRequest(cfg, '/api/external/download', {
      method: 'POST',
      body: JSON.stringify({
        url,
        platform: els.platform.value,
        cover: els.cover.checked,
      }),
    })
    clearResult()
    toast(`已加入队列：${data.title || data.group_name || ''}`, 'success')
    startTaskPolling(cfg)
  } catch (err) {
    toast(err.message || '下载失败', 'error')
  } finally {
    els.btnQuick.disabled = false
  }
}

// ---------- 初始化事件 ----------
async function init() {
  const cfg = await getConfig()
  const configured = !!(cfg.baseUrl && cfg.apiKey)
  els.setupTip.classList.toggle('hidden', configured)
  els.panel.classList.toggle('hidden', !configured)

  // 设置入口在已配置/未配置两种状态下都必须可用，故绑定放在提前返回之前
  els.btnOptions.addEventListener('click', () => chrome.runtime.openOptionsPage())
  els.btnGoOptions.addEventListener('click', () => chrome.runtime.openOptionsPage())
  const linkPrereq = $('#link-options-prereq')
  if (linkPrereq) {
    linkPrereq.addEventListener('click', (e) => {
      e.preventDefault()
      chrome.runtime.openOptionsPage()
    })
  }

  if (!configured) return

  renderCfg = cfg

  // 恢复下载方式偏好（默认后端队列）
  const saved = await chrome.storage.local.get({ [DEST_KEY]: 'server' })
  setDest(saved[DEST_KEY] === 'local' ? 'local' : 'server')
  syncDownloadUI()

  // 主页模式开关（默认开启，快手作者页注入下载按钮）
  const hm = await chrome.storage.sync.get({ vdcat_home_mode: 'on' })
  els.homeMode.checked = hm.vdcat_home_mode !== 'off'
  els.homeMode.addEventListener('change', () => {
    chrome.storage.sync.set({ vdcat_home_mode: els.homeMode.checked ? 'on' : 'off' })
  })

  // 自动识别当前标签页是否为抖音/快手，是则自动填入
  autoFillFromActiveTab()

  // 连通性检查（不强制阻塞）
  apiRequest(cfg, '/api/health').catch(() => {})

  loadTasks(cfg).catch((err) => {
    if (err && err.status === 401) {
      toast('API Key 无效或已停用，请重新设置', 'error')
    }
  })

  els.btnPaste.addEventListener('click', async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text) {
        els.input.value = text.trim()
        refreshMode()
        toast('已粘贴剪贴板内容')
      } else {
        toast('剪贴板为空', 'error')
      }
    } catch {
      toast('无法读取剪贴板，请手动粘贴', 'error')
    }
  })

  els.btnTab.addEventListener('click', async () => {
    const tab = await getActiveTab()
    if (!tab || !tab.url) {
      toast('读取当前标签页失败', 'error')
      return
    }
    els.input.value = tab.url
    const pf = detectPlatform(tab.url)
    if (pf) {
      els.platform.value = pf
      toast(`已填入当前页地址（${platformLabel(pf)}）`)
    } else {
      toast('已填入当前页地址（未识别平台，将自动识别）')
    }
    refreshMode()
  })

  els.btnBatchPage.addEventListener('click', async () => {
    const tab = await getActiveTab()
    if (!tab || !tab.id) {
      toast('无法获取当前标签页', 'error')
      return
    }
    if (!isKuaishouProfile(tab.url || '')) {
      toast('请在快手作者主页使用整页下载', 'error')
      return
    }
    try {
      await chrome.tabs.sendMessage(tab.id, { type: 'batch-start' })
    } catch {
      // content script 未注入（如扩展刚安装未刷新页面）时动态注入后重试
      try {
        await chrome.scripting.executeScript({
          target: { tabId: tab.id },
          files: ['content/content.js'],
        })
        await chrome.scripting.insertCSS({
          target: { tabId: tab.id },
          files: ['content/content.css'],
        })
        await new Promise((r) => setTimeout(r, 300))
        await chrome.tabs.sendMessage(tab.id, { type: 'batch-start' })
      } catch (err2) {
        toast('启动整页下载失败，请刷新作者主页后重试：' + (err2 && err2.message ? err2.message : err2), 'error')
        return
      }
    }
    window.close() // 后续编排由页面 content script 接管
  })

  els.btnClear.addEventListener('click', () => {
    els.input.value = ''
    clearResult()
    refreshMode()
    els.input.focus()
  })

  // 输入框内容（含手动粘贴、键入）变化时立即重算主页/解析模式，不锁死
  els.input.addEventListener('input', () => refreshMode())

  els.btnParse.addEventListener('click', () => doParse(cfg))
  els.btnQuick.addEventListener('click', () => doQuickDownload(cfg))

  els.btnDownload.addEventListener('click', async () => {
    if (!lastResult) return
    els.btnDownload.disabled = true
    try {
      // 本地下载：直接复用已解析出的直链，无需重新请求后端入队
      if (getDest() === 'local') {
        const n = await downloadLocally(cfg, lastResult)
        toast(`已触发本地下载 ${n} 个文件`, 'success')
        return
      }
      // 后端队列：extDownload 以原始链接重新解析并入队，保证服务端记录实际使用 cookie
      const raw = els.input.value.trim()
      const url = extractFirstURL(raw) || (lastResult.info && lastResult.info.page_url)
      if (!url) {
        toast('缺少原始链接，无法入队', 'error')
        return
      }
      const data = await apiRequest(cfg, '/api/external/download', {
        method: 'POST',
        body: JSON.stringify({ url, platform: els.platform.value, cover: els.cover.checked }),
      })
      toast(`已加入队列：${data.title || ''}`, 'success')
      startTaskPolling(cfg)
    } catch (err) {
      toast(err.message || '下载失败', 'error')
    } finally {
      els.btnDownload.disabled = false
    }
  })

  // 下载方式切换：持久化偏好 + 同步结果区按钮文案
  els.segDest.addEventListener('change', () => {
    const dest = getDest()
    chrome.storage.local.set({ [DEST_KEY]: dest })
    syncDownloadUI()
  })

  els.btnCopy.addEventListener('click', async () => {
    const v = lastResult && lastResult.info && lastResult.info.video_url
    if (!v) {
      toast('暂无直链', 'error')
      return
    }
    try {
      await navigator.clipboard.writeText(v)
      toast('无水印直链已复制', 'success')
    } catch {
      toast('复制失败', 'error')
    }
  })

  els.btnRefresh.addEventListener('click', () => {
    loadTasks(cfg)
    toast('已刷新')
  })

  // Ctrl/⌘ + Enter 快捷解析
  els.input.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') doParse(cfg)
  })
}

document.addEventListener('DOMContentLoaded', init)
