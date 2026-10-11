/* VDCat 主页模式 content script（快手作者主页）
 * 单卡片：注入下载按钮 -> 后台嗅探媒体流（嗅探期间隐藏弹层）-> 本地下载 -> 自动关闭弹层。
 * 整页批量：由 popup 触发，逐条播放/下载，带暂停、自动重试与去重。
 */
'use strict'

;(function () {
  const HOME_MODE_KEY = 'vdcat_home_mode'
  const CARD_SELECTOR = '.card-container .photo-card'
  const PLAYER_SELECTOR = '.player-pop'
  const CLOSE_SELECTOR = '.player-pop .close.circle-btn, .player-pop div.close[class*="circle"]'
  const CLOSE_DELAY_MS = 2000
  const TASK_TIMEOUT_MS = 20000
  // 首个候选失败后，等待备用候选到达的轮询参数
  const FALLBACK_STEP_MS = 600
  const FALLBACK_ROUNDS = 8 // 最多等待约 4.8s 收集更多候选
  // 打开弹层后等待「当前激活 slide」视频元素挂载的轮询参数
  const ACTIVE_STEP_MS = 200
  const ACTIVE_ROUNDS = 25 // 最多等待约 5s

  // 读取弹层内当前激活 slide（被点击的那条，而非预加载的相邻视频）的视频地址。
  // 快手弹层用 swiper，激活 slide 带 swiper-slide-active；视频可能是 <video src>
  // 或 blob: MSE 地址，这里返回 {src, isBlob}。
  function readActiveVideoURL() {
    const pop = document.querySelector(PLAYER_SELECTOR)
    if (!pop) return null
    const active =
      pop.querySelector('.swiper-slide-active') ||
      pop.querySelector('.swiper-slide')
    if (!active) return null
    // 激活 slide 的 data-e2e-vid 可用于二次校验
    const activeVid = active.getAttribute('data-e2e-vid') || ''
    const videos = Array.from(active.querySelectorAll('video'))
    for (const v of videos) {
      const src =
        (v.currentSrc && v.currentSrc) ||
        (v.src && v.src) ||
        (v.querySelector('source') && v.querySelector('source').src) ||
        ''
      if (src) return { src, isBlob: /^blob:/i.test(src), activeVid }
    }
    return { src: '', isBlob: false, activeVid }
  }

  let enabled = false
  let running = false
  let observer = null
  /** @type {{vid:string,firstURL:string}|null} */
  let pending = null

  // ---------- 工具 ----------

  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

  function sendMessage(msg) {
    return new Promise((resolve, reject) => {
      chrome.runtime.sendMessage(msg, (resp) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message))
        } else {
          resolve(resp)
        }
      })
    })
  }

  function isProfilePage() {
    return /^\/profile\//.test(location.pathname)
  }

  function sanitizeFileName(name) {
    const cleaned = (name || '')
      .replace(/[\\/:*?"<>|\r\n\t]+/g, '_')
      .replace(/\s+/g, ' ')
      .trim()
    const chars = Array.from(cleaned)
    return (chars.length > 80 ? chars.slice(0, 80).join('') : cleaned) || ''
  }

  function guessExt(url) {
    try {
      const path = new URL(url).pathname.toLowerCase()
      const m = path.match(/\.([a-z0-9]{2,4})$/)
      if (m && ['mp4', 'm4s', 'mov', 'webm', 'mkv'].includes(m[1])) return m[1]
    } catch {}
    return 'mp4'
  }

  // 与 background 同规则的归一化，用于在 content 侧二次确认重复候选
  const VOLATILE_KEYS = new Set([
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
      if (!VOLATILE_KEYS.has(key.toLowerCase())) {
        kept.push(encodeURIComponent(key) + '=' + encodeURIComponent(value))
      }
    })
    kept.sort()
    return u.protocol + '//' + u.host.toLowerCase() + u.pathname + (kept.length ? '?' + kept.join('&') : '')
  }
  function sameResource(a, b) {
    if (!a || !b) return false
    if (a === b || normalizeURL(a) === normalizeURL(b)) return true
    try {
      return new URL(a).pathname === new URL(b).pathname
    } catch {
      return false
    }
  }

  // 从封面图 clientCacheKey 提取视频 ID（与 data-e2e-vid 一致）
  function extractVid(card) {
    const img = card.querySelector('.cover img, img.cover-img')
    const src = (img && (img.currentSrc || img.src || img.getAttribute('src'))) || ''
    let m = src.match(/[?&]clientCacheKey=([^&]+)/i)
    if (m) {
      return decodeURIComponent(m[1]).replace(/\.(mp4|jpg|jpeg|png|webp|gif)$/i, '')
    }
    const wrap = card.closest('[data-e2e-vid]')
    if (wrap) return wrap.getAttribute('data-e2e-vid')
    return 'ks_' + Date.now()
  }

  function cardName(card, vid) {
    // 文件名直接取卡片 .caption 的完整文本（含 #话题 标签），如「大鸡排 #大香蕉」
    const caption = sanitizeFileName(card.querySelector('.caption')?.textContent || '')
    if (caption) return caption
    // caption 缺失时回退：作者-视频ID
    const author = sanitizeFileName(card.querySelector('.user .name, .name')?.textContent || '')
    return sanitizeFileName(author ? author + '-' + vid : vid)
  }

  // ---------- 页面提示 ----------

  let toastEl = null
  let toastTimer = null
  function ensureToast() {
    if (toastEl) return toastEl
    toastEl = document.createElement('div')
    toastEl.className = 'vdcat-toast'
    document.documentElement.appendChild(toastEl)
    return toastEl
  }
  function toast(msg, type) {
    const el = ensureToast()
    el.textContent = msg
    el.className = 'vdcat-toast' + (type ? ' vdcat-' + type : '')
    clearTimeout(toastTimer)
    toastTimer = setTimeout(() => el.classList.add('vdcat-hide'), 3500)
  }

  // ---------- 弹层控制 ----------

  function openCard(card) {
    const target = card.querySelector('.cover, .aspect-div') || card
    target.dispatchEvent(
      new MouseEvent('click', { bubbles: true, cancelable: true, view: window })
    )
  }

  function closePlayer() {
    const btn = document.querySelector(CLOSE_SELECTOR)
    if (btn) {
      btn.dispatchEvent(
        new MouseEvent('click', { bubbles: true, cancelable: true, view: window })
      )
      return true
    }
    return false
  }

  // ---------- 下载流程 ----------

  // 候选缓冲：持续接收后台 media-found（含时间戳），用于在多条预加载流中
  // 精确选出被点击那条。collected: [{url, ts}]
  let collected = []
  let collectHandler = null
  function startCollecting(vid) {
    stopCollecting()
    collected = []
    collectHandler = (msg) => {
      if (msg && msg.type === 'media-found' && msg.vid === vid && msg.url) {
        if (!collected.some((c) => sameResource(c.url, msg.url))) {
          collected.push({ url: msg.url, ts: msg.ts || 0 })
        }
      }
    }
    chrome.runtime.onMessage.addListener(collectHandler)
  }
  function stopCollecting() {
    if (collectHandler) {
      chrome.runtime.onMessage.removeListener(collectHandler)
      collectHandler = null
    }
    collected = []
  }

  // 等待弹层激活 slide 的真实视频地址就绪（http 直链优先；blob 也返回以便决策）
  async function waitActiveVideo(vid) {
    for (let i = 0; i < ACTIVE_ROUNDS; i++) {
      const info = readActiveVideoURL()
      if (info && info.src) {
        // 激活 slide 的 vid 与目标一致，且拿到非空地址
        if (!info.activeVid || info.activeVid === vid) return info
      }
      await sleep(ACTIVE_STEP_MS)
    }
    return null
  }

  // 综合「激活 slide 真实地址」与「带时间戳候选」选出目标视频流。
  async function resolveTargetURL(vid) {
    const active = await waitActiveVideo(vid)

    // 合并后台候选（含等待期间陆续到达的）
    const remote = await fetchCandidates()
    for (const c of remote.candidates || []) {
      const url = typeof c === 'string' ? c : c.url
      const ts = typeof c === 'object' ? c.ts || 0 : 0
      if (url && !collected.some((x) => sameResource(x.url, url))) collected.push({ url, ts })
    }

    // 1) 激活 slide 的 http(s) 直链：直接用，并尝试与候选对齐取同一资源
    if (active && active.src && !active.isBlob) {
      const hit = collected.find((c) => sameResource(c.url, active.src))
      const chosen = hit ? hit.url : active.src
      console.info('[VDCat] 采用激活 slide 直链', chosen)
      return chosen
    }

    // 2) 激活 slide 是 blob(MSE) 或拿不到地址：用时间戳从候选中选。
    //    目标视频由本次点击触发，其媒体请求应在嗅探开始之后；
    //    预加载的相邻流可能更早或更晚。取嗅探起始之后、时间戳最早的一条，
    //    即「点击后第一个被请求的视频流」，对应当前激活 slide。
    const startTs = remote.startTs || 0
    let after = collected.filter((c) => !startTs || c.ts >= startTs - 500)
    if (!after.length) after = collected.slice()
    if (!after.length) return ''
    after.sort((a, b) => a.ts - b.ts)
    console.info('[VDCat] 激活地址为 blob/缺失，按时间戳选取', after[0], '共', after.length, '条候选')
    return after[0].url
  }

  // 拉取后台已收集的全部不同资源候选（首个之后可能还在陆续到达）
  async function fetchCandidates() {
    try {
      const resp = await sendMessage({ type: 'sniff-candidates' })
      return resp && resp.ok && Array.isArray(resp.candidates) ? resp.candidates : []
    } catch {
      return []
    }
  }

  // 用给定 URL 发起一次下载；成功返回 true。
  // 页面 DOM 下载按钮只允许本地下载（chrome.downloads 经媒体代理落盘），
  // 固定走 background 的 local-download 通道，与 popup 的下载方式选择无关；
  // 云端下载队列只能由 popup 面板解析后发送，页面按钮无此入口。
  async function tryDownload(url, name) {
    const filename = `VDCat/${name}.${guessExt(url)}`
    const resp = await sendMessage({ type: 'local-download', url, filename })
    return !!(resp && resp.ok)
  }

  async function handleDownload(card, btn) {
    if (!enabled) return
    if (running) {
      toast('上一个视频仍在处理中，请稍候', 'warn')
      return
    }
    const vid = extractVid(card)
    const name = cardName(card, vid)

    running = true
    pending = { vid, firstURL: '' }
    btn.classList.add('vdcat-loading')
    btn.textContent = ''
    document.body.classList.add('vdcat-busy')

    let finished = false
    let timeoutHandle = null

    const finish = (ok, message) => {
      if (finished) return
      finished = true
      clearTimeout(timeoutHandle)
      pending = null
      running = false
      stopCollecting()
      sendMessage({ type: 'sniff-stop' }).catch(() => {})

      // 抓到资源后已等待 2 秒再关闭；失败时也尝试关闭可能打开的弹层
      const doRestore = () => {
        document.body.classList.remove('vdcat-sniffing', 'vdcat-busy')
        btn.classList.remove('vdcat-loading')
        btn.textContent = '下载'
        if (message) toast(message, ok ? 'success' : 'error')
      }
      closePlayer()
      setTimeout(doRestore, 400)
    }

    // 整体超时兜底
    timeoutHandle = setTimeout(() => finish(false, '未捕获到视频流，请重试'), TASK_TIMEOUT_MS)

    try {
      await sendMessage({ type: 'sniff-start', vid })
    } catch (err) {
      finish(false, '扩展后台通信失败：' + (err.message || err))
      return
    }

    // 先开启候选收集（拿点击之后的时间戳序列），再加隐藏类并点击打开弹层
    startCollecting(vid)
    document.body.classList.add('vdcat-sniffing')
    openCard(card)

    // 1) 结合「激活 slide 真实视频地址 + 候选时间戳」精确选出被点击那条，
    //    排除弹层预加载的相邻 2-3 条视频流。
    const targetURL = await resolveTargetURL(vid)
    if (finished) return
    if (!targetURL) {
      finish(false, '未捕获到当前视频流，请重试')
      return
    }
    if (pending) pending.firstURL = targetURL

    // 2) 目标流优先下载；失败时按时间戳顺序延后尝试其余候选，
    //    成功即结束（下载数量恒为 1），全部失败才报错。
    const tried = new Set()
    const ordered = [targetURL]
    let successURL = ''

    for (let i = 0; i < ordered.length; i++) {
      const url = ordered[i]
      if (!url || tried.has(url)) continue
      tried.add(url)

      let ok = false
      try {
        ok = await tryDownload(url, name)
      } catch (err) {
        console.warn('[VDCat] 候选下载失败，准备延后尝试', url, err && err.message)
        ok = false
      }

      if (ok) {
        successURL = url
        break
      }
      if (finished) return

      // 失败：拉取后台最新候选，把尚未尝试的新候选追加进队列；
      // 若暂无新候选，短暂等待其到达后重试，最多 FALLBACK_ROUNDS 轮。
      let extended = false
      for (let r = 0; r < FALLBACK_ROUNDS; r++) {
        await sleep(FALLBACK_STEP_MS)
        if (finished) return
        const all = await fetchCandidates()
        let gotNew = false
        for (const c of all.candidates || []) {
          const cu = typeof c === 'string' ? c : c.url
          if (cu && !tried.has(cu) && !ordered.includes(cu)) {
            ordered.push(cu)
            gotNew = true
            extended = true
          }
        }
        if (gotNew) break
      }
      console.info(
        extended
          ? `[VDCat] 第 ${i + 1} 条候选失败，已延后到备用候选`
          : `[VDCat] 第 ${i + 1} 条候选失败，暂无更多备用候选`
      )
    }

    if (successURL) {
      toast('已触发下载：' + name, 'success')
      // 抓到资源后保留隐藏状态 2 秒，再自动点击关闭按钮
      setTimeout(() => finish(true, ''), CLOSE_DELAY_MS)
    } else {
      finish(false, '视频流下载失败，已尝试全部候选，请重试')
    }
  }

  // ---------- 整页批量下载（作者主页）----------
  // 由 popup 在识别到路径为 /profile/<id> 的作者主页时通过 batch-start 触发。
  // 流程：打开第一个卡片弹层 -> 下载当前激活 slide -> 等待1s -> 点「下一条」
  //       -> 5s 内等新视频加载 -> 循环；失败/到底则暂停，由用户「重试/结束」。
  // 已下载 vid 记录在 chrome.storage.local 临时库，重试不重复下载。
  // 批量期间不隐藏弹层，改用黑色遮罩阻止操作页面，并拦截刷新/离开。

  const BATCH_DB_KEY = 'vdcat_batch_done_v1'
  const NEXT_BTN_SELECTOR =
    '.player-pop .nextVideo .next, .player-pop .hover-tip.nextVideo .next'
  const BATCH_FALLBACK_WAIT_MS = 1000 // 下载后兜底等待 1 秒再跳
  const BATCH_LOAD_WAIT_MS = 5000 // 下一条加载最长等待 5 秒
  const BATCH_ACTIVE_WAIT_MS = 5000 // 当前 slide 视频元素最长等待 5 秒
  const BATCH_KEEPALIVE_MS = 30000
  const BATCH_AUTO_RETRY = 4 // 推进/识别失败自动重试次数（覆盖平台 21/41/61… 不传数据）
  const BATCH_RETRY_BASE_MS = 1500 // 自动重试基础等待，逐次递增

  /** @type {null|{
   *  stop:boolean, paused:boolean, done:Set<string>, count:number,
   *  cycleStart:number, candidates:Array<{url:string,ts:number}>,
   *  keepalive:number, mediaHandler:Function
   * }} */
  let batch = null
  let batchOverlay = null
  let onBeforeUnload = null

  function clickNextVideo() {
    const btn = document.querySelector(NEXT_BTN_SELECTOR)
    if (!btn) return false
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }))
    return true
  }

  // 关闭弹层内「连播」开关：开启时 .auto-play-btn 的 .toggle-switch 带 is-active
  // 且 aria-checked="true"。点击整个按钮关闭，避免页面自动播下一条干扰扩展编排。
  function autoPlaySwitch() {
    const root = document.querySelector(PLAYER_SELECTOR + ' .auto-play-btn')
    if (!root) return null
    const sw = root.querySelector('.toggle-switch')
    if (!sw) return null
    const on = sw.classList.contains('is-active') ||
      sw.getAttribute('aria-checked') === 'true'
    return { root, sw, on }
  }

  // 打开弹层后轮询检查连播开关，若为开启则点击关闭；每次任务开始都执行（刷新后会重置为开）。
  async function ensureAutoPlayOff(timeoutMs) {
    const deadline = Date.now() + timeoutMs
    let clicked = false
    while (Date.now() < deadline) {
      if (!batch || batch.stop || batch.paused) return
      const ap = autoPlaySwitch()
      if (ap) {
        if (ap.on) {
          ap.root.dispatchEvent(new MouseEvent('click', {
            bubbles: true, cancelable: true, view: window,
          }))
          clicked = true
          // 等待 Vue 状态更新后复核
          await sleep(300)
          const after = autoPlaySwitch()
          if (after && !after.on) {
            console.info('[VDCat][batch] 已关闭连播开关')
            return
          }
        } else {
          console.info('[VDCat][batch] 连播已处于关闭状态')
          return
        }
      }
      await sleep(200)
    }
    if (clicked) console.warn('[VDCat][batch] 已点击连播开关但未确认关闭')
  }

  function activeSlideVid() {
    const active = document.querySelector(
      PLAYER_SELECTOR + ' .swiper-slide-active, ' + PLAYER_SELECTOR + ' .swiper-slide'
    )
    return active ? active.getAttribute('data-e2e-vid') || '' : ''
  }

  function activeSlideName(vid) {
    const active = document.querySelector(
      PLAYER_SELECTOR + ' .swiper-slide-active, ' + PLAYER_SELECTOR + ' .swiper-slide'
    )
    const caption = sanitizeFileName(active?.querySelector('.caption')?.textContent || '')
    if (caption) return caption
    const author = sanitizeFileName(active?.querySelector('.name')?.textContent || '')
    return author ? author + '-' + vid : vid || 'video'
  }

  // 从快手直链的 clientCacheKey 查询参数提取视频 ID，如 ...clientCacheKey=3xn4ybnreygegqg_b.mp4
  function vidFromURL(raw) {
    if (!raw) return ''
    try {
      const v = new URL(raw).searchParams.get('clientCacheKey') || ''
      const d = decodeURIComponent(v).replace(/\.(mp4|m4s|jpg|jpeg|png|webp|gif)$/i, '')
      // 去掉清晰度后缀，如 _b / _h
      return d.replace(/_[a-z0-9]+$/i, '') || d
    } catch {
      return ''
    }
  }

  // 等待激活 slide 就绪并解析当前视频流地址（http 直链优先，blob 时按时间戳选候选）。
  // 不把 data-e2e-vid 作为必要条件：某些时机激活 slide 未带该属性，
  // 此时直接用已拿到的 video 直链，并从其 clientCacheKey 提取 vid。
  async function resolveBatchTarget() {
    const deadline = Date.now() + BATCH_ACTIVE_WAIT_MS
    let info = null
    while (Date.now() < deadline) {
      if (batch.stop || batch.paused) return null
      info = readActiveVideoURL()
      // 任一可下载依据就绪即退出：有 http 直链、或已收到嗅探候选
      const httpReady = info && info.src && !info.isBlob
      const candReady = batch.candidates.length > 0
      const perfReady = !!latestPerfVideoURL()
      if (httpReady || candReady || perfReady) break
      await sleep(150)
    }

    let url = ''
    // 1) 激活 slide 是 http(s) 直链：优先使用，天然排除预加载相邻流
    if (info && info.src && !info.isBlob) {
      const hit = batch.candidates.find((c) => sameResource(c.url, info.src))
      url = hit ? hit.url : info.src
    }
    // 2) blob(MSE) 或无直链：本周期时间戳候选 -> performance -> 最新候选
    if (!url) {
      const probeDeadline = Date.now() + 3000
      while (!url && Date.now() < probeDeadline) {
        if (batch.stop || batch.paused) return null
        url = pickCycleCandidate()
        if (!url) await sleep(200)
      }
    }
    if (!url) url = latestPerfVideoURL()
    if (!url) {
      const any = batch.candidates.slice().sort((a, b) => b.ts - a.ts)[0]
      if (any) url = any.url
    }
    if (!url) return null

    // vid：优先激活 slide 的 data-e2e-vid，缺失则从直链 clientCacheKey 提取
    const vid = (info && info.activeVid) || vidFromURL(url) || ('ks_' + Date.now())
    console.info('[VDCat][batch] 选定目标 vid=', vid, 'url=', url)
    return { vid, url }
  }

  function pickCycleCandidate() {
    // 本周期（切换到当前 slide）之后出现的候选，取最早一条
    const list = batch.candidates
      .filter((c) => c.ts >= batch.cycleStart - 500)
      .sort((a, b) => a.ts - b.ts)
    if (list.length) return list[0].url
    // 本周期无候选时（cycleStart 边界误差），放宽到比上一周期更晚的候选
    const relaxed = batch.candidates.slice().sort((a, b) => b.ts - a.ts)[0]
    return relaxed ? relaxed.url : ''
  }

  // 从页面性能资源时间线读取最近发起的视频资源（content 脚本可直接访问，
  // 作为 webRequest 候选缺失时的兜底）。仅取快手视频 CDN 的 mp4/m4s 地址。
  function latestPerfVideoURL() {
    try {
      const entries = performance.getEntriesByType('resource')
      let best = null
      for (const e of entries) {
        const u = e.name || ''
        if (!/^https?:/.test(u)) continue
        let host = ''
        try {
          host = new URL(u).hostname
        } catch {
          continue
        }
        if (
          !/(kwaicdn\.com|ndcimgs\.com|ksyuncs\.com|yxixiguavideo\.com|yxixigua\.com|kuaishouv\.com|gifshow\.com)/.test(
            host
          )
        )
          continue
        const s = u.toLowerCase()
        if (!/\.mp4($|\?)/.test(s) && !/\.m4s($|\?)/.test(s) && !s.includes('mime_type=video'))
          continue
        if (!best || e.startTime > best.startTime) best = e
      }
      return best ? best.name : ''
    } catch {
      return ''
    }
  }

  // 判定 slide 是否切换：优先 data-e2e-vid 变化；vid 取不到时用
  // 激活 slide video 直链的 clientCacheKey 是否与上一条不同来判断。
  async function waitSlideChanged(prevVid, prevURL, timeoutMs) {
    const prevKey = vidFromURL(prevURL) || prevVid
    const deadline = Date.now() + timeoutMs
    while (Date.now() < deadline) {
      if (batch.stop || batch.paused) return ''
      const vid = activeSlideVid()
      if (vid && prevVid && vid !== prevVid) {
        batch.cycleStart = Date.now()
        return vid
      }
      const info = readActiveVideoURL()
      const curKey = (info && info.src && !info.isBlob) ? vidFromURL(info.src) : ''
      if (curKey && prevKey && curKey !== prevKey) {
        batch.cycleStart = Date.now()
        return vid || curKey
      }
      await sleep(200)
    }
    return ''
  }

  // ---------- 浮动状态面板（可拖动，不锁定页面）----------

  function ensureOverlay() {
    if (batchOverlay) return batchOverlay
    const mask = document.createElement('div')
    mask.className = 'vdcat-batch-mask'
    mask.innerHTML =
      '<div class="vdcat-batch-panel">' +
      '<div class="vdcat-batch-drag" data-k="drag">' +
      '<span class="vdcat-batch-title">VDCat 整页下载</span>' +
      '<span class="vdcat-batch-grip" title="拖动移动">⠿</span>' +
      '</div>' +
      '<div class="vdcat-batch-stat">已下载 <b data-k="count">0</b> 条</div>' +
      '<div class="vdcat-batch-status" data-k="status">正在准备…</div>' +
      '<div class="vdcat-batch-tip">下载进行中页面已锁定并模糊显示，暂停后可操作页面。建议新建窗口浏览，将本窗口最小化等待。</div>' +
      '<div class="vdcat-batch-spinner" data-k="spinner"></div>' +
      '<div class="vdcat-batch-actions">' +
      '<button type="button" class="vdcat-batch-btn vdcat-batch-toggle" data-k="toggle">暂停</button>' +
      '<button type="button" class="vdcat-batch-btn vdcat-batch-end" data-k="end">结束</button>' +
      '</div>' +
      '</div>'
    document.documentElement.appendChild(mask)
    const root = mask.querySelector('.vdcat-batch-panel')
    const q = (k) => mask.querySelector('[data-k="' + k + '"]')
    batchOverlay = {
      mask,
      root,
      count: q('count'),
      status: q('status'),
      toggle: q('toggle'),
      end: q('end'),
      spinner: q('spinner'),
      drag: q('drag'),
    }
    // 暂停 / 继续（暂停后按钮变继续；失败态也用它恢复）
    batchOverlay.toggle.addEventListener('click', () => {
      if (!batch || batch.stop) return
      // 防抖：避免暂停/继续在极短时间内被连续触发导致状态抖动
      const now = Date.now()
      if (now - (batch.lastToggleAt || 0) < 300) return
      batch.lastToggleAt = now
      if (batch.paused) resumeBatch()
      else manualPauseBatch()
    })
    batchOverlay.end.addEventListener('click', () => endBatch('用户结束本次整页下载'))
    enablePanelDrag(root, q('drag'))
    return batchOverlay
  }

  // 标题栏拖动；自动限制在视口内
  function enablePanelDrag(panel, handle) {
    let sx = 0
    let sy = 0
    let sl = 0
    let st = 0
    let dragging = false

    const onMove = (e) => {
      if (!dragging) return
      const pt = e.touches ? e.touches[0] : e
      let nl = sl + (pt.clientX - sx)
      let nt = st + (pt.clientY - sy)
      const maxL = window.innerWidth - panel.offsetWidth - 8
      const maxT = window.innerHeight - panel.offsetHeight - 8
      nl = Math.max(8, Math.min(nl, maxL))
      nt = Math.max(8, Math.min(nt, maxT))
      panel.style.left = nl + 'px'
      panel.style.top = nt + 'px'
      panel.style.right = 'auto'
      panel.style.bottom = 'auto'
    }
    const onUp = () => {
      dragging = false
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      document.removeEventListener('touchmove', onMove)
      document.removeEventListener('touchend', onUp)
    }
    const onDown = (e) => {
      const pt = e.touches ? e.touches[0] : e
      dragging = true
      const rect = panel.getBoundingClientRect()
      sx = pt.clientX
      sy = pt.clientY
      sl = rect.left
      st = rect.top
      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
      document.addEventListener('touchmove', onMove, { passive: false })
      document.addEventListener('touchend', onUp)
      e.preventDefault()
    }
    handle.addEventListener('mousedown', onDown)
    handle.addEventListener('touchstart', onDown, { passive: false })
  }

  function setBatchStatus(text, paused) {
    const ov = ensureOverlay()
    ov.status.textContent = text
    ov.count.textContent = String(batch ? batch.count : 0)
    ov.toggle.textContent = paused ? '继续' : '暂停'
    // 工作中：全屏遮罩锁定并模糊页面；暂停：移除遮罩，恢复清晰与点击
    ov.mask.classList.toggle('vdcat-mask-paused', !!paused)
    ov.root.classList.toggle('vdcat-batch-is-paused', !!paused)
    ov.spinner.classList.toggle('hidden', !!paused)
  }

  function removeOverlay() {
    if (batchOverlay && batchOverlay.mask.parentNode) {
      batchOverlay.mask.parentNode.removeChild(batchOverlay.mask)
    }
    batchOverlay = null
  }

  // ---------- 生命周期 ----------

  async function startBatch() {
    if (batch && !batch.stop) {
      toast('整页下载已在进行中', 'warn')
      return
    }
    const firstCard = document.querySelector(CARD_SELECTOR)
    if (!firstCard) {
      toast('未找到视频卡片，请确认在作者主页', 'error')
      return
    }

    // 每次启动重置临时库（id 与文件名双维度去重）
    await chrome.storage.local.set({ [BATCH_DB_KEY]: { ids: [], names: [] } })

    batch = {
      stop: false,
      paused: false,
      looping: false,
      epoch: 0,
      lastToggleAt: 0,
      done: new Set(),
      doneNames: new Set(),
      retries: 0,
      count: 0,
      cycleStart: Date.now(),
      candidates: [],
      keepalive: 0,
      mediaHandler: (msg) => {
        if (!batch || batch.stop) return
        if (msg && msg.type === 'media-found' && msg.batch && msg.url) {
          if (!batch.candidates.some((c) => sameResource(c.url, msg.url))) {
            batch.candidates.push({ url: msg.url, ts: msg.ts || 0 })
          }
        }
      },
    }
    chrome.runtime.onMessage.addListener(batch.mediaHandler)

    onBeforeUnload = (e) => {
      if (batch && !batch.stop) {
        e.preventDefault()
        e.returnValue = '整页下载正在进行中，确定要离开此页面吗？'
        return e.returnValue
      }
    }
    window.addEventListener('beforeunload', onBeforeUnload)

    ensureOverlay()
    setBatchStatus('正在打开第一个视频…', false)

    try {
      await sendMessage({ type: 'batch-start' })
    } catch (err) {
      endBatch('扩展后台通信失败：' + (err.message || err))
      return
    }
    batch.keepalive = setInterval(() => {
      sendMessage({ type: 'batch-keepalive' }).catch(() => {})
    }, BATCH_KEEPALIVE_MS)

    // 批量期间不隐藏弹层（与单卡片下载不同），遮罩负责锁定操作
    openCard(firstCard)
    setBatchStatus('正在检查连播开关…', false)
    // 每次开始都确保关闭「连播」（页面刷新后会自动开启），由扩展程序控制推进
    await ensureAutoPlayOff(6000)
    if (batch.stop) return
    runBatchLoop()
  }

  async function runBatchLoop() {
    // 活动锁：同一时刻只允许一个编排循环，防止恢复时与未退出的旧循环并发
    if (!batch || batch.looping) return
    batch.looping = true
    const myEpoch = batch.epoch
    // epoch 失活（用户在循环卡住期间暂停又继续，生成了新循环）则本循环立即作废
    const stale = () => !batch || batch.stop || batch.epoch !== myEpoch || batch.paused
    try {
      while (batch && !batch.stop && !batch.paused && batch.epoch === myEpoch) {
        sendMessage({ type: 'batch-keepalive' }).catch(() => {})
        batch.cycleStart = Date.now()
        setBatchStatus('正在识别当前视频…', false)

        const target = await resolveBatchTarget()
        if (stale()) return

        // 识别失败（平台在 21/41/61… 边界常不返回数据）：自动重试而非立即暂停
        if (!target) {
          if (batch.retries < BATCH_AUTO_RETRY) {
            batch.retries++
            const wait = BATCH_RETRY_BASE_MS * batch.retries
            setBatchStatus('当前视频未加载，自动重试 ' + batch.retries + '/' + BATCH_AUTO_RETRY +
              '（' + Math.round(wait / 1000) + 's）…', false)
            console.info('[VDCat][batch] 识别为空，自动重试', batch.retries)
            // 再次尝试点击下一条以触发数据拉取
            clickNextVideo()
            await sleep(wait)
            if (stale()) return
            continue
          }
          pauseBatch('当前视频多次加载失败，请确认页面后点击「继续」')
          return
        }

        const name = activeSlideName(target.vid)
        const isDup = batch.done.has(target.vid) ||
          (name && batch.doneNames.has(name))

        if (isDup) {
          setBatchStatus('该视频已下载（' + name + '），跳到下一条…', false)
        } else {
          setBatchStatus('正在下载第 ' + (batch.count + 1) + ' 条：' + name, false)
          // 下载本身也带自动重试（偶发 CDN/网络失败）
          let ok = false
          for (let attempt = 1; attempt <= BATCH_AUTO_RETRY && !stale(); attempt++) {
            try {
              ok = await tryDownload(target.url, name)
            } catch (err) {
              console.warn('[VDCat][batch] 下载失败 attempt', attempt, err)
              ok = false
            }
            if (ok) break
            if (attempt < BATCH_AUTO_RETRY) {
              const wait = BATCH_RETRY_BASE_MS * attempt
              setBatchStatus('下载失败，自动重试 ' + attempt + '/' + BATCH_AUTO_RETRY +
                '（' + Math.round(wait / 1000) + 's）…', false)
              await sleep(wait)
            }
          }
          // 下载刚结束：若用户已暂停/已停止/epoch 已变，立即停在当前条，绝不自动推进
          if (stale()) {
            console.info('[VDCat][batch] 下载完成但处于暂停态，保持停在当前视频')
            return
          }
          if (!ok) {
            pauseBatch('第 ' + (batch.count + 1) + ' 条多次下载失败，网络恢复后点击「继续」')
            return
          }
          // 同时按 id 与当前文件名去重，任一命中即视为已下载
          batch.done.add(target.vid)
          if (name) batch.doneNames.add(name)
          batch.count++
          batch.retries = 0
          await persistBatchDone()
        }

        // 下载成功后等待 1 秒兜底，再触发下一条
        setBatchStatus('已下载 ' + batch.count + ' 条，准备下一条…', false)
        await sleep(BATCH_FALLBACK_WAIT_MS)
        if (stale()) return

        const prevVid = target.vid
        // 推进到下一条：失败/未切换时自动重试（边界位置平台不传数据，需再次触发）
        let advanced = false
        for (let attempt = 1; attempt <= BATCH_AUTO_RETRY && !stale(); attempt++) {
          batch.cycleStart = Date.now()
          if (!clickNextVideo()) {
            // 未找到按钮通常意味着到最后一条；少量重试后才暂停
            if (attempt < BATCH_AUTO_RETRY) {
              await sleep(BATCH_RETRY_BASE_MS * attempt)
              continue
            }
            pauseBatch('未找到「下一条」按钮，可能已到最后一条；确认后可「结束」')
            return
          }
          setBatchStatus('等待下一条视频加载…' +
            (attempt > 1 ? '（重试 ' + attempt + '/' + BATCH_AUTO_RETRY + '）' : ''), false)
          const nextVid = await waitSlideChanged(prevVid, target.url, BATCH_LOAD_WAIT_MS)
          if (stale()) return
          if (nextVid) {
            advanced = true
            batch.retries = 0
            break
          }
          if (attempt < BATCH_AUTO_RETRY) {
            const wait = BATCH_RETRY_BASE_MS * attempt
            setBatchStatus('下一条未加载，自动重试 ' + attempt + '/' + BATCH_AUTO_RETRY +
              '（' + Math.round(wait / 1000) + 's）…', false)
            console.info('[VDCat][batch] 下一条未切换，自动重试', attempt)
            await sleep(wait)
          }
        }
        if (!advanced) {
          pauseBatch('下一条多次未加载，可能已到最后一条或网络异常。恢复后点击「继续」')
          return
        }
      }
    } catch (err) {
      if (batch && !batch.stop) pauseBatch('发生异常：' + (err && err.message ? err.message : err))
    } finally {
      if (batch) batch.looping = false
    }
  }

  function pauseBatch(text) {
    if (!batch || batch.stop) return
    batch.paused = true
    console.info('[VDCat][batch] >>> pauseBatch paused=', batch.paused, 'looping=', batch.looping)
    setBatchStatus(text || '已暂停', true)
    toast('整页下载已暂停', 'warn')
  }

  // 用户手动暂停：循环会在最近的 await 检查点退出，页面恢复可操作
  function manualPauseBatch() {
    if (!batch || batch.stop) return
    console.info('[VDCat][batch] 点击暂停，当前 paused=', batch.paused, 'looping=', batch.looping)
    pauseBatch('已手动暂停，页面可正常操作，点击「继续」恢复下载')
  }

  function resumeBatch() {
    if (!batch || batch.stop || !batch.paused) {
      console.info('[VDCat][batch] 忽略 continue：stop/paused 状态不符',
        batch ? { stop: batch.stop, paused: batch.paused, looping: batch.looping } : null)
      return
    }
    console.info('[VDCat][batch] 点击继续，looping=', batch.looping)
    batch.paused = false
    setBatchStatus('继续下载…', false)
    // 仅当旧循环已彻底退出（looping=false，如暂停在识别/等待间隙）才新开循环。
    // 若旧循环仍卡在下载 await 中，它醒来读到 paused=false 会自然继续，
    // 此处绝不另起循环，避免双循环并发。
    if (!batch.looping) {
      batch.epoch++
      runBatchLoop()
    }
  }

  async function persistBatchDone() {
    try {
      await chrome.storage.local.set({
        [BATCH_DB_KEY]: {
          ids: Array.from(batch.done),
          names: Array.from(batch.doneNames),
        },
      })
    } catch {}
  }

  async function endBatch(reason) {
    if (!batch) return
    const total = batch.count
    batch.stop = true
    batch.paused = false
    clearInterval(batch.keepalive)
    if (batch.mediaHandler) chrome.runtime.onMessage.removeListener(batch.mediaHandler)
    if (onBeforeUnload) window.removeEventListener('beforeunload', onBeforeUnload)
    sendMessage({ type: 'batch-stop' }).catch(() => {})
    try {
      await chrome.storage.local.remove(BATCH_DB_KEY)
    } catch {}
    closePlayer()
    removeOverlay()
    document.body.classList.remove('vdcat-sniffing', 'vdcat-busy')
    toast(reason || ('整页下载结束，共下载 ' + total + ' 条'), 'success')
    batch = null
    onBeforeUnload = null
  }

  // ---------- 卡片注入 ----------

  function injectButton(card) {
    if (card.dataset.vdcatBtn) return
    card.dataset.vdcatBtn = '1'
    card.classList.add('vdcat-card')
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'vdcat-dl-btn'
    btn.textContent = '下载'
    btn.title = 'VDCat：嗅探媒体并下载到本机'
    // 捕获阶段拦截，防止点按钮时顺带打开弹层
    btn.addEventListener(
      'click',
      (e) => {
        e.preventDefault()
        e.stopPropagation()
        e.stopImmediatePropagation()
        handleDownload(card, btn)
      },
      true
    )
    card.appendChild(btn)
  }

  function scanCards(root) {
    const scope = root && root.querySelectorAll ? root : document
    scope.querySelectorAll(CARD_SELECTOR).forEach(injectButton)
  }

  function startObserver() {
    if (observer) return
    observer = new MutationObserver((mutations) => {
      for (const m of mutations) {
        for (const node of m.addedNodes) {
          if (node.nodeType !== 1) continue
          if (node.matches?.(CARD_SELECTOR) || node.querySelector?.(CARD_SELECTOR)) {
            scanCards(node)
          }
        }
      }
    })
    observer.observe(document.body, { childList: true, subtree: true })
  }

  function stopObserver() {
    if (observer) {
      observer.disconnect()
      observer = null
    }
  }

  // ---------- 开关 ----------

  async function loadEnabled() {
    const obj = await chrome.storage.sync.get({ [HOME_MODE_KEY]: 'on' })
    enabled = obj[HOME_MODE_KEY] !== 'off'
    applyMode()
  }

  function applyMode() {
    const active = enabled && isProfilePage()
    document.body.classList.toggle('vdcat-home-on', active)
    if (active) {
      scanCards(document)
      startObserver()
    } else {
      stopObserver()
      document.body.classList.remove('vdcat-home-on')
    }
  }

  // 后台嗅探结果
  chrome.runtime.onMessage.addListener((msg) => {
    if (msg && msg.type === 'media-found' && pending && msg.vid === pending.vid) {
      pending.onFound(msg.url)
    }
  })

  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === 'sync' && changes[HOME_MODE_KEY]) {
      enabled = changes[HOME_MODE_KEY].newValue !== 'off'
      applyMode()
    }
  })

  // popup「下载本页面全部视频」入口（作者主页，无需开启卡片注入开关）
  chrome.runtime.onMessage.addListener((msg) => {
    if (msg && msg.type === 'batch-start') {
      startBatch()
    }
  })

  // SPA 内路由切换（首页 -> 作者页）也能生效
  let lastPath = location.pathname
  setInterval(() => {
    if (location.pathname !== lastPath) {
      lastPath = location.pathname
      applyMode()
    }
  }, 1000)

  loadEnabled()
})()
