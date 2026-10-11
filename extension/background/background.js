/* VDCat 扩展后台 Service Worker
 * 职责：
 *  1) 主页模式下用 chrome.webRequest（只读观察）嗅探快手视频媒体流，按 tab 分发给 content script
 *  2) content script 抓到媒体后，由后台经后端 /api/media 代理调 chrome.downloads 落盘
 * MV3 中 webRequest 仅观察不阻塞无需 webRequestBlocking 权限。
 */
'use strict'

// tabId -> { vid, batch, startTs, seen, candidates, timer }
const sniffers = new Map()
const SNIFF_TIMEOUT_MS = 20000

// 快手视频 CDN 主机后缀，与后端 media.go 白名单保持一致
const MEDIA_HOST_RE =
  /(^|\.)(kwaicdn\.com|ndcimgs\.com|ksyuncs\.com|yxixiguavideo\.com|yxixigua\.com|kuaishouv\.com|gifshow\.com|ksapisrv\.com)$/

// XHR/fetch 拉流时用 URL 特征二次确认；media 类型（<video> 请求）命中 CDN 主机即视为视频流
function isVideoURL(url, type) {
  let u
  try {
    u = new URL(url)
  } catch {
    return false
  }
  if (!/^https?:$/.test(u.protocol)) return false
  const hostMatch = MEDIA_HOST_RE.test(u.hostname)
  if (!hostMatch) return false
  if (type === 'media') return true
  const s = (u.pathname + u.search).toLowerCase()
  return (
    /\.mp4($|[?&])/.test(s) ||
    /\.m4s($|[?&])/.test(s) ||
    s.includes('mime_type=video') ||
    s.includes('video%2fmp4') ||
    s.includes('video/mp4')
  )
}

// URL 归一化，用于判断“同一资源的重复请求”。
// 快手同一视频可能因签名/缓存参数（如 &_t 时间戳、t= 过期时间、ws*、ws0 等）产生两条 URL，
// 这里忽略已知易变查询参数后比较；路径本身一致即认为是同一文件。
const VOLATILE_QUERY_KEYS = new Set([
  '_', 't', 'tt', '_t', 'ts', 'time', 'timestamp', 'nonce', 'r', 'ran',
  'ws0', 'ws1', 'ws2', 'wshc', 'tag', 'bp', 'di', 'kpn', 'cc', 'clientCacheKey',
])

function normalizeURL(raw) {
  let u
  try {
    u = new URL(raw)
  } catch {
    return raw
  }
  const kept = []
  u.searchParams.forEach((value, key) => {
    const k = key.toLowerCase()
    // 保留签名主体（sig/sign/k）及资源定位参数，仅丢弃易变项
    if (VOLATILE_QUERY_KEYS.has(k)) return
    kept.push(encodeURIComponent(key) + '=' + encodeURIComponent(value))
  })
  kept.sort()
  return u.protocol + '//' + u.host.toLowerCase() + u.pathname + (kept.length ? '?' + kept.join('&') : '')
}

// 判断两个 URL 是否指向同一资源：归一化后相等，或路径文件名部分完全相同
function sameResource(a, b) {
  if (a === b) return true
  const na = normalizeURL(a)
  const nb = normalizeURL(b)
  if (na === nb) return true
  try {
    return new URL(a).pathname === new URL(b).pathname
  } catch {
    return false
  }
}

chrome.webRequest.onBeforeRequest.addListener(
  (details) => {
    const state = sniffers.get(details.tabId)
    if (!state) return
    if (!isVideoURL(details.url, details.type)) return
    if (state.seen.has(details.url)) return
    state.seen.add(details.url)

    // 不同资源（相邻预加载视频/不同清晰度）按时间戳收集，content 用激活 slide + 时间戳选定
    const dupIdx = state.candidates.findIndex((c) => sameResource(c.url, details.url))
    if (dupIdx >= 0) {
      console.info('[VDCat] 同资源重复候选，已忽略', details.type, details.url)
      return
    }
    const candidate = { url: details.url, ts: details.timeStamp || Date.now() }
    state.candidates.push(candidate)

    if (state.batch) keepAlive(state)
    chrome.tabs
      .sendMessage(details.tabId, {
        type: 'media-found',
        vid: state.vid,
        batch: !!state.batch,
        url: candidate.url,
        ts: candidate.ts,
      })
      .catch(() => {})
  },
  { urls: ['<all_urls>'], types: ['media', 'xmlhttprequest', 'other'] }
)

// 批量模式的嗅探保活：候选到达或收到心跳时重置空闲超时
const BATCH_IDLE_MS = 120000
function keepAlive(state) {
  if (!state.batch) return
  clearTimeout(state.timer)
  state.timer = setTimeout(() => sniffers.delete(state.tabId), BATCH_IDLE_MS)
}

// 批量连续嗅探：长时存活、vid 不限，由 content 的心跳/停止控制
function startBatchSniff(tabId) {
  const old = sniffers.get(tabId)
  if (old) clearTimeout(old.timer)
  const state = {
    tabId,
    vid: '',
    batch: true,
    startTs: Date.now(),
    seen: new Set(),
    candidates: [],
    timer: setTimeout(() => sniffers.delete(tabId), BATCH_IDLE_MS),
  }
  sniffers.set(tabId, state)
}

function startSniff(tabId, vid) {
  const old = sniffers.get(tabId)
  if (old) clearTimeout(old.timer)
  const state = {
    vid: vid || '',
    startTs: Date.now(), // 用于区分点击前已在途的预加载请求
    seen: new Set(),
    candidates: [],
    timer: setTimeout(() => sniffers.delete(tabId), SNIFF_TIMEOUT_MS),
  }
  sniffers.set(tabId, state)
}

function stopSniff(tabId) {
  const state = sniffers.get(tabId)
  if (state) {
    clearTimeout(state.timer)
    sniffers.delete(tabId)
  }
}

// 经后端媒体代理下载（CDN 直连防盗链），api_key 走 query（downloads 无法加自定义头）。
// chrome.downloads.download 即使上游 4xx/5xx 也会先返回 id，真正失败是稍后的
// interrupted 状态，因此这里在拿到 id 后短暂观察：进入下载态/完成态才算成功，
// 立即 interrupted 则抛错，让 content 侧延后尝试下一个候选。
function observeDownload(id, timeoutMs) {
  return new Promise((resolve, reject) => {
    let settled = false
    const finish = (fn, arg) => {
      if (settled) return
      settled = true
      chrome.downloads.onChanged.removeListener(onChanged)
      clearTimeout(timer)
      fn(arg)
    }
    const onChanged = (delta) => {
      if (delta.id !== id || !delta.state) return
      if (delta.state.current === 'complete') finish(resolve, id)
      else if (delta.state.current === 'interrupted') {
        chrome.downloads.search({ id }, (items) => {
          const reason = items && items[0] && items[0].error
          finish(reject, new Error('下载中断' + (reason ? '：' + reason : '')))
        })
      }
    }
    chrome.downloads.onChanged.addListener(onChanged)
    const timer = setTimeout(() => {
      chrome.downloads.search({ id }, (items) => {
        const st = items && items[0] && items[0].state
        if (st === 'interrupted') {
          finish(reject, new Error('下载中断：' + ((items[0] && items[0].error) || '未知错误')))
        } else {
          finish(resolve, id)
        }
      })
    }, timeoutMs)
  })
}

function platformReferer(host) {
  if (/kuaishou|chenzhongtech|gifshow|yxixigua|ksyuncs|ndcimgs|kwaicdn/.test(host)) {
    return 'https://www.kuaishou.com/'
  }
  if (/douyin|iesdouyin|amemv|douyinpic|douyinvod|byteimg|bytecdntp/.test(host)) {
    return 'https://www.douyin.com/'
  }
  return ''
}

// 通过流式写入 OPFS（源私有文件系统）接管下载：
// fetch 响应头到达即开始边下边写，不等待整文件、不占大块内存；
// 写完后用文件句柄 URL 立即启动 chrome.downloads，文件名完全由扩展控制
// （不会被上游 media.mp4 覆盖），且无需配置后端。
async function downloadViaFileStream(url, savePath) {
  let host
  try {
    host = new URL(url).hostname
  } catch (_) {
    host = ''
  }
  const referer = platformReferer(host)
  const resp = await fetch(url, {
    method: 'GET',
    headers: referer ? { Referer: referer } : {},
    credentials: 'include',
    redirect: 'follow',
  })
  if (!resp.ok) throw new Error('直链抓取失败 HTTP ' + resp.status)
  if (!resp.body) throw new Error('响应不支持流式读取')

  const root = await navigator.storage.getDirectory()
  const tmpName = 'vd_' + Date.now() + '_' + Math.random().toString(36).slice(2) + '.tmp'
  const fileHandle = await root.getFileHandle(tmpName, { create: true })
  const writable = await fileHandle.createWritable()
  let total = 0
  try {
    const reader = resp.body.getReader()
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      await writable.write(value)
      total += value.length
    }
    if (total === 0) throw new Error('抓取内容为空')
    await writable.close()
  } catch (e) {
    try { await writable.abort() } catch (_) {}
    try { await root.removeEntry(tmpName) } catch (_) {}
    throw e
  }

  // 文件完整就绪后用 blob: URL 启动下载，文件名由扩展控制，不被上游 media.mp4 覆盖
  const file = await fileHandle.getFile()
  const objectUrl = URL.createObjectURL(file)
  try {
    const id = await chrome.downloads.download({
      url: objectUrl,
      filename: savePath,
      conflictAction: 'uniquify',
      saveAs: false,
    })
    setTimeout(() => {
      URL.revokeObjectURL(objectUrl)
      root.removeEntry(tmpName).catch(() => {})
    }, 120000)
    await observeDownload(id, 4000)
    return { id, via: 'stream' }
  } catch (e) {
    URL.revokeObjectURL(objectUrl)
    try { await root.removeEntry(tmpName) } catch (_) {}
    throw e
  }
}

// 回退：已配置后端时走媒体代理（仍带 dl 文件名头）
async function downloadViaProxy(url, savePath) {
  const cfg = await chrome.storage.sync.get({ baseUrl: '', apiKey: '' })
  const baseUrl = (cfg.baseUrl || '').replace(/\/+$/, '')
  if (!baseUrl || !cfg.apiKey) {
    throw new Error('未配置服务地址或 API Key，且直链抓取失败')
  }
  const baseName = savePath.split('/').pop()
  const proxy =
    baseUrl +
    '/api/media?url=' + encodeURIComponent(url) +
    '&api_key=' + encodeURIComponent(cfg.apiKey) +
    '&dl=' + encodeURIComponent(baseName)
  const id = await chrome.downloads.download({
    url: proxy,
    filename: savePath,
    conflictAction: 'uniquify',
    saveAs: false,
  })
  await observeDownload(id, 4000)
  return { id, via: 'proxy' }
}

// 检测是否已配置后端（决定能否走瞬时弹出的代理通道）
async function hasBackend() {
  const cfg = await chrome.storage.sync.get({ baseUrl: '', apiKey: '' })
  return Boolean((cfg.baseUrl || '').replace(/\/+$/, '') && cfg.apiKey)
}

async function startDownload(url, filename) {
  const savePath = filename || 'VDCat/video.mp4'
  // 已配置后端：直接把代理 URL 交给浏览器，瞬时弹出下载项、由浏览器流式接管，
  // 不占 SW 内存/时间，文件名由后端 dl 头保证；
  // 未配置后端：退回 OPFS 流式文件接管，纯本地完成命名（需先在后台抓取完）。
  if (await hasBackend()) {
    try {
      return await downloadViaProxy(url, savePath)
    } catch (err1) {
      return await downloadViaFileStream(url, savePath)
    }
  }
  try {
    return await downloadViaFileStream(url, savePath)
  } catch (err2) {
    return await downloadViaProxy(url, savePath)
  }
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  const tabId = sender.tab && sender.tab.id
  ;(async () => {
    try {
      if (msg.type === 'sniff-start') {
        if (tabId === undefined) throw new Error('缺少标签页上下文')
        startSniff(tabId, String(msg.vid || ''))
        sendResponse({ ok: true })
      } else if (msg.type === 'sniff-stop' || msg.type === 'batch-stop') {
        if (tabId !== undefined) stopSniff(tabId)
        sendResponse({ ok: true })
      } else if (msg.type === 'batch-start') {
        if (tabId === undefined) throw new Error('缺少标签页上下文')
        startBatchSniff(tabId)
        sendResponse({ ok: true })
      } else if (msg.type === 'batch-keepalive') {
        const st = tabId !== undefined ? sniffers.get(tabId) : null
        if (st) keepAlive(st)
        sendResponse({ ok: true })
      } else if (msg.type === 'sniff-candidates') {
        // 供 content 拉取候选（含时间戳）与嗅探起始时间
        const state = tabId !== undefined ? sniffers.get(tabId) : null
        sendResponse({
          ok: true,
          startTs: state ? state.startTs : 0,
          candidates: state ? state.candidates.slice() : [],
        })
      } else if (msg.type === 'local-download') {
        // 唯一允许来自页面 DOM 按钮（content script）的下载通道：
        // 优先内存接管直接落盘（不依赖后端），绝不进入云端下载队列。
        // 云端队列只能由 popup 面板解析后调用 /api/external/download 发起。
        if (!sender.tab) throw new Error('本地下载通道仅限页面 content script 调用')
        const result = await startDownload(msg.url, msg.filename)
        sendResponse({ ok: true, id: result.id })
      } else {
        sendResponse({ ok: false, error: '未知消息类型' })
      }
    } catch (err) {
      sendResponse({ ok: false, error: err.message || String(err) })
    }
  })()
  return true // 异步响应
})
