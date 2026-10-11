<template>
  <v-app>
    <Login v-if="needsAuth" @success="onLogin" />

    <template v-else>
      <!-- 移动端顶栏（仅 md 以下渲染，桌面端彻底不占位） -->
      <v-app-bar v-if="!mdAndUp" flat density="compact" color="surface"
        class="app-mobile-bar">
        <v-app-bar-nav-icon icon="mdi-menu" @click="drawerOpen = !drawerOpen" />
        <v-app-bar-title class="font-weight-bold">VDCAT · 短视频下载</v-app-bar-title>
      </v-app-bar>

      <!-- 桌面端：常驻导航栏（md 及以上渲染） -->
      <v-navigation-drawer v-if="mdAndUp" permanent color="surface" width="210"
        location="left" class="app-drawer">
        <drawer-nav :tab="tab" :auth-required="authRequired"
          @select="selectTab" @settings="settingsOpen = true" @logout="logout" />
      </v-navigation-drawer>

      <!-- 移动端：临时抽屉（md 以下渲染） -->
      <v-navigation-drawer v-else v-model="drawerOpen" temporary color="surface" width="240"
        location="left" class="app-drawer">
        <drawer-nav :tab="tab" :auth-required="authRequired"
          @select="selectTab" @settings="settingsOpen = true; drawerOpen = false" @logout="logout" />
      </v-navigation-drawer>

      <v-main class="bg-background">
        <v-container fluid class="pa-6" style="max-width: 1100px">
          <!-- ============ 下载页 ============ -->
          <div v-show="tab === 'download'">
            <!-- 页面标题 -->
            <div class="d-flex align-center mb-4 page-head">
              <v-avatar color="primary" size="44" rounded="lg" class="mr-3 page-head-icon">
                <v-icon icon="mdi-download-box-outline" size="26" color="on-primary" />
              </v-avatar>
              <div>
                <div class="text-h5 font-weight-bold">视频下载</div>
                <div class="text-body-2 text-medium-emphasis">快手 / 抖音短视频解析与批量下载</div>
              </div>
            </div>

            <v-card class="mb-5 pa-2" color="surface" elevation="0">
              <v-card-item>
                <v-card-title class="d-flex align-center text-h6">
                  <v-icon icon="mdi-link-variant" class="mr-2" color="primary" />
                  粘贴视频链接
                </v-card-title>
                <v-card-subtitle>支持快手 / 抖音链接或整段分享文本，每行一个，批量解析排队下载。</v-card-subtitle>
              </v-card-item>
              <v-card-text>
                <v-textarea
                  v-model="urlInput"
                  rows="3"
                  placeholder="粘贴快手/抖音链接或分享文本，如 https://v.douyin.com/xxxx/ 或 3.14 复制打开抖音…"
                  prepend-inner-icon="mdi-content-paste"
                  variant="outlined"
                  density="comfortable"
                  hide-details
                  @keydown.ctrl.enter="parseAll"
                />
                <v-row align="center" class="mt-3" no-gutters>
                  <v-col cols="12" sm="auto" class="pr-sm-4">
                    <v-btn-toggle v-model="platform" mandatory color="primary" variant="outlined" density="default">
                      <v-btn value="auto" prepend-icon="mdi-auto-fix">自动识别</v-btn>
                      <v-btn value="kuaishou" prepend-icon="mdi-alpha-k-box">快手</v-btn>
                      <v-btn value="douyin" prepend-icon="mdi-music-note">抖音</v-btn>
                    </v-btn-toggle>
                  </v-col>
                  <v-col cols="12" sm="auto">
                    <v-btn color="secondary" variant="tonal" size="large" prepend-icon="mdi-magnify-scan"
                      :loading="parsing" class="px-5" @click="parseAll">解析</v-btn>
                    <v-btn variant="tonal" size="large" prepend-icon="mdi-close-box-multiple-outline"
                      class="ml-2" :disabled="!urlInput.trim()" @click="clearInput">清空</v-btn>
                  </v-col>
                  <v-col cols="12" sm="auto" class="ml-sm-3 mt-2 mt-sm-0">
                    <v-checkbox v-model="downloadCover" label="同时下载封面" density="compact" hide-details color="primary" />
                  </v-col>
                  <v-spacer />
                  <v-col cols="12" sm="auto">
                    <v-btn variant="tonal" prepend-icon="mdi-link-plus" @click="directOpen = true">手动直链</v-btn>
                  </v-col>
                </v-row>
              </v-card-text>
            </v-card>

            <!-- 解析结果：每项可勾选，支持全选，勾选后才能下载 -->
            <v-card v-if="parseResults.length" class="mb-5 pa-2" color="surface" elevation="0">
              <v-card-item>
                <div class="d-flex align-center">
                  <v-icon icon="mdi-format-list-bulleted-square" class="mr-2" color="primary" />
                  <span class="text-h6">解析结果</span>
                  <v-chip size="small" class="ml-3" color="surface-variant">
                    已选 {{ selectedCount }} / {{ parseResults.length }}
                  </v-chip>
                  <v-spacer />
                  <v-btn variant="text" size="small" prepend-icon="mdi-broom" @click="clearResults">清空</v-btn>
                </div>
              </v-card-item>
              <v-card-text>
                <v-checkbox v-model="selectAll" :label="selectAll ? '取消全选' : '全选'"
                  density="compact" color="primary" hide-details class="mb-3"
                  :disabled="!parseResults.length" />
                <v-card v-for="(r, idx) in parseResults" :key="r.key" variant="flat"
                  color="surface-variant" class="mb-2 result-card pa-3">
                  <div class="d-flex result-row">
                    <!-- 左：勾选 + 视频预览（独立圆角盒子，宽度随横竖屏自适应） -->
                    <div class="d-flex align-center result-left flex-shrink-0">
                      <v-checkbox :model-value="r.checked" density="compact" hide-details color="primary"
                        class="mr-1 check-box" @update:model-value="(v) => setChecked(idx, v)" />
                      <div class="preview-box" :style="{ width: boxWidth(r) }">
                        <video :src="mediaSrc(r.info.video_url)" controls preload="metadata" playsinline
                          class="preview-video" @loadedmetadata="onMeta(idx, $event)">
                          您的浏览器不支持视频播放
                        </video>
                        <v-img v-if="r.info.cover_url" :src="mediaSrc(r.info.cover_url)" cover
                          class="cover-badge">
                          <template #placeholder>
                            <div class="d-flex align-center justify-center fill-height text-medium-emphasis">
                              <v-icon icon="mdi-image-off" size="16" />
                            </div>
                          </template>
                        </v-img>
                      </div>
                    </div>
                    <!-- 右：信息（独立信息框，弹性填充） -->
                    <div class="result-info flex-grow-1">
                      <div class="text-body-1 font-weight-medium line-clamp-2">{{ r.info.title || '无标题' }}</div>
                      <div class="d-flex align-center flex-wrap my-2">
                        <v-icon icon="mdi-account-outline" size="14" class="mr-1" />
                        <span class="text-body-2">{{ r.info.author || '未知作者' }}</span>
                        <v-chip size="x-small" class="ml-2" color="surface" variant="flat">
                          {{ platformText(r.platform) }}
                        </v-chip>
                        <v-chip size="x-small" class="ml-1" color="surface" variant="flat">
                          {{ r.info.source === 'api' ? '接口解析' : 'HTML提取' }}
                        </v-chip>
                      </div>
                      <div class="text-caption text-medium-emphasis">
                        分辨率 {{ r.info.width || '?' }}×{{ r.info.height || '?' }}
                        <span v-if="r.info.duration"> · 时长 {{ Math.round(r.info.duration / 1000) }}s</span>
                      </div>
                    </div>
                  </div>
                </v-card>

                <div class="d-flex justify-end mt-3">
                  <v-btn color="primary" size="large" prepend-icon="mdi-download-circle-outline"
                    :loading="downloading" :disabled="selectedCount === 0" class="px-6"
                    @click="downloadSelected">
                    下载选中{{ selectedCount ? `（${selectedCount}）` : '' }}
                  </v-btn>
                </div>
              </v-card-text>
            </v-card>
          </div>

          <!-- ============ 下载队列页 ============ -->
          <div v-show="tab === 'queue'">
            <div class="d-flex align-center mb-4 page-head">
              <v-avatar color="primary" size="44" rounded="lg" class="mr-3 page-head-icon">
                <v-icon icon="mdi-tray-arrow-down" size="26" color="on-primary" />
              </v-avatar>
              <div>
                <div class="text-h5 font-weight-bold">下载队列</div>
                <div class="text-body-2 text-medium-emphasis">进行中的下载任务，实时进度</div>
              </div>
            </div>
            <QueuePage :tasks="activeTasks"
              @refresh="refreshTasks" @retry="retry" @cancel="cancelTask" @remove="removeTask" />
          </div>

          <!-- ============ 下载记录页 ============ -->
          <div v-show="tab === 'history'">
            <div class="d-flex align-center mb-4 page-head">
              <v-avatar color="primary" size="44" rounded="lg" class="mr-3 page-head-icon">
                <v-icon icon="mdi-history" size="26" color="on-primary" />
              </v-avatar>
              <div>
                <div class="text-h5 font-weight-bold">下载记录</div>
                <div class="text-body-2 text-medium-emphasis">历史任务查询，支持按分组 / 平台过滤与分页</div>
              </div>
            </div>
            <HistoryPage ref="historyPageRef"
              @retry="retry" @cancel="cancelTask" @remove="removeTask"
              @remove-group="removeGroup" @changed="refreshTasks" />
          </div>

          <!-- ============ Cookie 管理页 ============ -->
          <div v-show="tab === 'cookies'">
            <div class="d-flex align-center mb-4 page-head">
              <v-avatar color="primary" size="44" rounded="lg" class="mr-3 page-head-icon">
                <v-icon icon="mdi-cookie-cog-outline" size="26" color="on-primary" />
              </v-avatar>
              <div>
                <div class="text-h5 font-weight-bold">Cookie 管理</div>
                <div class="text-body-2 text-medium-emphasis">多平台多账号 Cookie，失败自动切换</div>
              </div>
            </div>
            <CookieManager />
          </div>

          <!-- ============ API 接入页 ============ -->
          <div v-show="tab === 'api'">
            <div class="d-flex align-center mb-4 page-head">
              <v-avatar color="primary" size="44" rounded="lg" class="mr-3 page-head-icon">
                <v-icon icon="mdi-api" size="26" color="on-primary" />
              </v-avatar>
              <div>
                <div class="text-h5 font-weight-bold">API 接入</div>
                <div class="text-body-2 text-medium-emphasis">为浏览器扩展生成 API Key 与请求地址</div>
              </div>
            </div>
            <ApiAccess />
          </div>

          <!-- ============ 文件页 ============ -->
          <div v-show="tab === 'files'">
            <div class="d-flex align-center mb-4 page-head">
              <v-avatar color="primary" size="44" rounded="lg" class="mr-3 page-head-icon">
                <v-icon icon="mdi-folder-multiple-outline" size="26" color="on-primary" />
              </v-avatar>
              <div>
                <div class="text-h5 font-weight-bold">下载文件</div>
                <div class="text-body-2 text-medium-emphasis">按平台 / 任务分类管理已下载内容</div>
              </div>
            </div>
            <FileManager :files="fileList" :root="fileRoot"
              @refresh="loadFiles" @delete="deleteFile" />
          </div>
        </v-container>
      </v-main>
    </template>

    <!-- 设置 -->
    <v-dialog v-model="settingsOpen" max-width="620">
      <v-card>
        <v-card-title class="d-flex align-center">
          <v-icon icon="mdi-cog-outline" class="mr-2" /> 设置
        </v-card-title>
        <v-card-text>
          <!-- 外观主题（仅前端，即时生效并持久化） -->
          <div class="text-body-2 text-medium-emphasis mb-2">外观主题</div>
          <v-btn-toggle mandatory color="primary" variant="outlined" divided
            :model-value="themeMode" @update:model-value="changeTheme" class="mb-4 w-100">
            <v-btn v-for="o in themeOptions" :key="o.value" :value="o.value" class="flex-grow-1">
              <v-icon :icon="o.icon" class="mr-1" size="18" />
              <span class="text-body-2">{{ o.title }}</span>
            </v-btn>
          </v-btn-toggle>

          <!-- 解析方式：自动兜底 / 接口解析 / HTML 提取；强制模式失败不切换 -->
          <div class="text-body-2 text-medium-emphasis mb-2">解析方式</div>
          <v-btn-toggle v-model="cfg.parse_mode" mandatory color="primary" variant="outlined" divided
            density="comfortable" class="w-100 mb-1">
            <v-btn value="auto" class="flex-grow-1">
              <v-icon icon="mdi-auto-fix" class="mr-1" size="18" />
              <span class="text-body-2">自动</span>
            </v-btn>
            <v-btn value="api" class="flex-grow-1">
              <v-icon icon="mdi-api" class="mr-1" size="18" />
              <span class="text-body-2">接口解析</span>
            </v-btn>
            <v-btn value="html" class="flex-grow-1">
              <v-icon icon="mdi-language-html5" class="mr-1" size="18" />
              <span class="text-body-2">HTML 提取</span>
            </v-btn>
          </v-btn-toggle>
          <v-switch v-model="cfg.force_parse"
            :label="cfg.parse_mode === 'auto' ? '强制此模式（自动模式下始终兜底，此选项无效）' : '强制此模式（失败后不切换其它方案）'"
            :disabled="cfg.parse_mode === 'auto'" density="compact" hide-details color="primary"
            class="mb-3 mt-1" />

          <v-text-field v-model="cfg.download_dir" label="下载目录（服务器绝对路径）"
            prepend-icon="mdi-folder-outline" />
          <v-row class="mt-1">
            <v-col cols="12" sm="6">
              <v-text-field v-model.number="cfg.concurrency" type="number" label="并发任务数" min="1" max="8" />
            </v-col>
            <v-col cols="12" sm="6" class="d-flex align-center justify-around">
              <v-switch v-model="cfg.write_meta" label="保存元数据" color="primary" hide-details />
            </v-col>
          </v-row>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="settingsOpen = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :loading="savingCfg" @click="saveCfg">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 手动直链 -->
    <v-dialog v-model="directOpen" max-width="620">
      <v-card>
        <v-card-title class="d-flex align-center">
          <v-icon icon="mdi-link-plus" class="mr-2" /> 手动直链下载
        </v-card-title>
        <v-card-text>
          <p class="text-body-2 text-medium-emphasis">
            自动解析失败时，可从 F12 → Network 过滤媒体请求，复制 .mp4 直链。下载时自动携带快手 Referer 绕过防盗链。
          </p>
          <v-text-field v-model="directURL" label="视频直链 (https://...mp4)" prepend-icon="mdi-link" density="compact" />
          <v-text-field v-model="directReferer" label="Referer（可选，默认快手首页）"
            prepend-icon="mdi-arrow-decision-outline" density="compact" class="mt-2" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="directOpen = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :loading="directing" @click="startDirect">开始下载</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 全局右下角消息提示（替代浏览器 alert） -->
    <ToastHost />

    <!-- 全局确认对话框（替代浏览器 confirm） -->
    <ConfirmDialog />
  </v-app>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted, onUnmounted } from 'vue'
import { useDisplay } from 'vuetify'
import { api, tokenStore, mediaSrc } from './api'
import Login from './Login.vue'
import FileManager from './FileManager.vue'
import CookieManager from './CookieManager.vue'
import ApiAccess from './ApiAccess.vue'
import QueuePage from './QueuePage.vue'
import HistoryPage from './HistoryPage.vue'
import DrawerNav from './DrawerNav.vue'
import ToastHost from './ToastHost.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import { toast } from './toast'
import { confirmDialog } from './confirm'
import { useTheme } from 'vuetify'
import { themeMode, setThemeMode, resolveTheme } from './theme-store'

const needsAuth = ref(true)
const authRequired = ref(true)
const vuetifyTheme = useTheme()
const { mdAndUp } = useDisplay()

// 主题切换：auto 跟随系统 / light 浅色 / dark 深色，选择后持久化
const themeOptions = [
  { value: 'auto', icon: 'mdi-theme-light-dark', title: '跟随系统' },
  { value: 'light', icon: 'mdi-white-balance-sunny', title: '浅色' },
  { value: 'dark', icon: 'mdi-weather-night', title: '深色' },
]
function changeTheme(mode) {
  setThemeMode(mode)
  vuetifyTheme.global.name.value = resolveTheme(mode)
}
const tab = ref('download')
const drawerOpen = ref(false)
function selectTab(name) {
  tab.value = name
  // 移动端为临时抽屉，选中后自动收起
  if (!mdAndUp.value) drawerOpen.value = false
  // 切到对应页面时自动刷新一次
  if (name === 'files') loadFiles()
  if (name === 'queue') refreshTasks()
  if (name === 'history' && historyPageRef.value) {
    historyPageRef.value.loadGroups().then(() => historyPageRef.value.reload())
  }
}

const urlInput = ref('')

// 仅前端持久化的偏好设置（localStorage）
const prefStore = {
  get(key, def) {
    try {
      const v = localStorage.getItem(key)
      return v === null ? def : v === 'true'
    } catch { return def }
  },
  set(key, val) {
    try { localStorage.setItem(key, val ? 'true' : 'false') } catch { /* ignore */ }
  },
}
const platform = ref(localStorage.getItem('vdcat_platform') || 'auto')
const downloadCover = ref(prefStore.get('vdcat_download_cover', true))
watch(platform, (v) => { try { localStorage.setItem('vdcat_platform', v) } catch { /* ignore */ } })
watch(downloadCover, (v) => prefStore.set('vdcat_download_cover', v))

const parsing = ref(false)
const downloading = ref(false)
// 解析结果：{ key, checked, platform, cookie, info }
const parseResults = ref([])
const activeTasks = ref([])
const historyPageRef = ref(null)

// 解析结果勾选：已选数量与全选（支持取消全选）
const selectedCount = computed(() => parseResults.value.filter((r) => r.checked).length)
const selectAll = computed({
  get: () => parseResults.value.length > 0 && parseResults.value.every((r) => r.checked),
  set: (v) => { parseResults.value.forEach((r) => { r.checked = v }) },
})
function setChecked(idx, v) {
  if (parseResults.value[idx]) parseResults.value[idx].checked = !!v
}
function clearResults() {
  parseResults.value = []
}

// 预览盒高度固定，宽度按视频真实宽高比反推，使视频铺满圆角盒无黑边
const PREVIEW_H = 220
function onMeta(idx, e) {
  const v = e.target
  if (!v || !v.videoWidth || !v.videoHeight) return
  const r = parseResults.value[idx]
  if (r) r.ratio = v.videoWidth / v.videoHeight
}
function boxWidth(r) {
  // 无元数据时竖屏默认 9:16；限制在 124~400px
  const ratio = r.ratio || 9 / 16
  const w = Math.round(PREVIEW_H * ratio)
  return `${Math.max(124, Math.min(400, w))}px`
}

const fileList = ref([])
const fileRoot = ref('')

const settingsOpen = ref(false)
const savingCfg = ref(false)
const cfg = reactive({
  cookie: '', download_dir: '', concurrency: 2, write_meta: true,
  parse_mode: 'auto', force_parse: false,
})

const directOpen = ref(false)
const directURL = ref('')
const directReferer = ref('https://www.kuaishou.com/')
const directing = ref(false)

let timer = null

async function bootstrap() {
  try {
    const h = await api.health()
    authRequired.value = !!h.auth_required
    if (!h.auth_required || tokenStore.get()) {
      if (tokenStore.get()) {
        // 验证 token 有效性
        try {
          await api.getConfig()
          enterApp()
          return
        } catch {
          tokenStore.clear()
        }
      }
      if (!h.auth_required) {
        enterApp()
        return
      }
    }
    needsAuth.value = true
  } catch {
    needsAuth.value = true
  }
}

function enterApp() {
  needsAuth.value = false
  loadConfig()
  refreshTasks()
  loadFiles()
  timer = setInterval(() => {
    refreshTasks()
  }, 1500)
}

function onLogin() {
  enterApp()
}

function logout() {
  tokenStore.clear()
  if (timer) clearInterval(timer)
  needsAuth.value = true
}

async function loadConfig() {
  try {
    Object.assign(cfg, await api.getConfig())
    if (!['auto', 'api', 'html'].includes(cfg.parse_mode)) cfg.parse_mode = 'auto'
  } catch { /* ignore */ }
}

async function saveCfg() {
  savingCfg.value = true
  try {
    const c = await api.saveConfig(cfg)
    Object.assign(cfg, c)
    settingsOpen.value = false
    toast.success('设置已保存')
  } catch (e) {
    toast.error(e.message)
  } finally {
    savingCfg.value = false
  }
}

// 仅提取 https 链接：抖音/快手分享口令、换行长文本里的文字一律忽略，
// http 及其它任何非 https 协议也忽略。支持一段文本含多个链接。
function splitUrls(text) {
  const re = /https:\/\/[^\s，。、；“”"'<>（）()[\]{}]+/g
  const out = []
  let m
  while ((m = re.exec(text)) !== null) {
    const u = m[0].replace(/[.,;:!?，。、；！？]+$/, '')
    if (u && !out.includes(u)) out.push(u)
  }
  return out
}

// 生成本次批量下载组：时间戳 + 平台标签（auto 时用“批量”）
function makeBatchGroup(pfForName) {
  const d = new Date()
  const p = (n) => String(n).padStart(2, '0')
  const ts = `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`
  const pfText = { kuaishou: '快手', douyin: '抖音' }[pfForName] || '批量'
  return { group_id: `g${Date.now()}-${Math.random().toString(36).slice(2, 7)}`, group_name: `${ts}-${pfText}` }
}

// 第一步：仅解析，生成结果列表（不触发下载）
async function parseAll() {
  const urls = splitUrls(urlInput.value)
  if (urls.length === 0) {
    toast.warning('未在文本中找到 https 链接\n请粘贴包含 https:// 开头链接的分享文本（http 链接不被支持）')
    return
  }
  // 每次解析自动清除上一次的解析结果
  parseResults.value = []
  parsing.value = true
  const results = []
  try {
    for (const url of urls) {
      try {
        const res = await api.parse(url, platform.value)
        const info = res.info || res // 兼容 ResolveResult
        const pf = res.platform || info.platform || platform.value
        results.push({
          key: `${Date.now()}-${results.length}-${Math.random().toString(36).slice(2, 7)}`,
          checked: true,
          platform: pf,
          cookie: res.cookie_content || '',
          info,
        })
      } catch (e) {
        toast.error(`解析失败 [${url}]\n${e.message}`)
      }
    }
    parseResults.value = results
    // 解析后保留输入框内容，由用户手动清空
  } finally {
    parsing.value = false
  }
}

function clearInput() {
  urlInput.value = ''
}

// 第二步：仅下载勾选的解析结果，归入同一下载组
async function downloadSelected() {
  const picked = parseResults.value.filter((r) => r.checked)
  if (picked.length === 0) return
  downloading.value = true
  // 组名平台：若全部同平台用该平台名，否则用“批量”
  const pfs = new Set(picked.map((r) => r.platform))
  const samePf = pfs.size === 1 ? picked[0].platform : ''
  const batch = makeBatchGroup(samePf)
  const doneKeys = new Set()
  try {
    for (const r of picked) {
      const info = r.info
      await api.download({
        platform: r.platform,
        cookie: r.cookie,
        group_id: batch.group_id,
        group_name: batch.group_name,
        video_url: info.video_url, cover_url: info.cover_url, page_url: info.page_url,
        title: info.title, author: info.author, author_id: info.author_id,
        id: info.id, cover: downloadCover.value,
      })
      doneKeys.add(r.key)
      await refreshTasks()
    }
    // 从结果列表移除已成功提交下载的项
    parseResults.value = parseResults.value.filter((r) => !doneKeys.has(r.key))
    if (doneKeys.size > 0) toast.success(`已提交 ${doneKeys.size} 个任务到下载队列`)
  } catch (e) {
    toast.error(`下载提交失败：${e.message}`)
  } finally {
    downloading.value = false
    await refreshTasks()
    await reloadHistory()
  }
}

async function startDirect() {
  if (!directURL.value.trim()) return
  directing.value = true
  try {
    await api.downloadDirect(directURL.value, directReferer.value)
    directOpen.value = false
    directURL.value = ''
    toast.success('已加入下载队列')
    await refreshTasks()
  } catch (e) {
    toast.error(e.message)
  } finally {
    directing.value = false
  }
}

async function refreshTasks() {
  try {
    activeTasks.value = await api.activeTasks()
  } catch { /* ignore */ }
}

// 刷新下载记录页（若该页已挂载）
function reloadHistory() {
  return historyPageRef.value ? historyPageRef.value.reload() : Promise.resolve()
}

async function removeGroup(g) {
  const choice = await confirmDialog({
    title: '删除整组',
    message: `删除整组「${g.name}」的 ${g.total} 条记录？\n可选择仅移除记录，或同时删除磁盘文件。`,
    confirmText: '仅移除记录',
    extraText: '同时删除文件',
    danger: true,
    icon: 'mdi-delete-alert-outline',
  })
  if (!choice) return
  const withFile = choice === 'extra'
  try {
    await api.deleteGroup(g.id, withFile)
  } catch (e) {
    if (withFile) {
      try { await api.deleteGroup(g.id, false) } catch (_) { toast.error(e.message); return }
    } else {
      toast.error(e.message)
      return
    }
  }
  toast.success('整组已删除')
  await Promise.all([historyPageRef.value?.loadGroups(), reloadHistory(), refreshTasks()])
}

async function loadFiles() {
  try {
    const d = await api.files()
    fileList.value = d.files || []
    fileRoot.value = d.root || ''
  } catch { /* ignore */ }
}

async function cancelTask(id) {
  try { await api.cancel(id) } catch (e) { toast.error(e.message) }
  await refreshTasks()
}

async function retry(id) {
  try { await api.retry(id); toast.success('已重新加入队列') } catch (e) { toast.error(e.message) }
  await refreshTasks()
}

async function removeTask(t) {
  const choice = await confirmDialog({
    title: '删除任务记录',
    message: `删除任务记录「${t.title || t.id}」？\n可选择仅移除记录，或同时删除磁盘文件。`,
    confirmText: '仅移除记录',
    extraText: '同时删除文件',
    danger: true,
    icon: 'mdi-delete-alert-outline',
  })
  if (!choice) return
  const withFile = choice === 'extra'
  try {
    await api.deleteTask(t.id, withFile)
  } catch (e) {
    if (withFile) {
      // 连文件删除失败时，回退为仅删记录
      try { await api.deleteTask(t.id, false) } catch (_) { toast.error(e.message) }
    } else {
      toast.error(e.message)
    }
  }
  toast.success('记录已删除')
  await Promise.all([refreshTasks(), reloadHistory()])
}

async function deleteFile(f) {
  const ok = await confirmDialog({
    title: '删除文件',
    message: `确定删除文件「${f.name}」？此操作不可恢复。`,
    confirmText: '删除',
    danger: true,
    icon: 'mdi-file-remove-outline',
  })
  if (!ok) return
  try {
    await api.deleteFile(f.rel_path)
    await loadFiles()
    toast.success('文件已删除')
  } catch (e) { toast.error(e.message) }
}

function platformText(p) {
  return { kuaishou: '快手', douyin: '抖音' }[p] || p || ''
}
function statusText(s) {
  return { queued: '等待中', running: '下载中', done: '已完成', error: '失败',
    canceled: '已取消', paused: '已暂停', interrupted: '已中断' }[s] || s
}
function statusColor(s) {
  return { running: 'primary', done: 'success', error: 'error',
    canceled: 'warning', queued: 'info', interrupted: 'error' }[s] || 'secondary'
}
function formatBytes(n) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(i ? 1 : 0)} ${u[i]}`
}

onMounted(bootstrap)
onUnmounted(() => timer && clearInterval(timer))
</script>

<style>
/* 侧边栏分隔线与背景 */
.app-drawer {
  border-right: 1px solid rgba(var(--v-border-color), var(--v-border-opacity, 0.12));
}
.app-mobile-bar {
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity, 0.12));
}

.line-clamp-2 {
  display: -webkit-box;
  line-clamp: 2;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 解析结果卡片：外层圆角父盒子，视频与信息各自独立圆角 */
.result-card {
  border-radius: 14px;
}
.result-row {
  gap: 14px;
  align-items: stretch;
}
.result-left {
  align-items: center;
}
.check-box {
  align-self: flex-start;
  margin-top: 2px;
}
/* 视频：独立圆角容器，尺寸贴合视频真实宽高比，铺满无黑边 */
.preview-box {
  position: relative;
  height: 220px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  overflow: hidden;
}
.preview-video {
  display: block;
  width: 100%;
  height: 100%;
  border-radius: 12px;
  background: #000;
  object-fit: cover;
}
/* 信息：独立浅色圆角信息框 */
.result-info {
  background: rgb(var(--v-theme-surface));
  border-radius: 12px;
  padding: 14px 16px;
  min-width: 0;
}
.cover-badge {
  position: absolute;
  left: 14px;
  bottom: 14px;
  width: 38px;
  height: 54px;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.3);
  pointer-events: none;
}

/* 窄屏：纵向堆叠，预览与信息框各自占满整行 */
@media (max-width: 600px) {
  .result-row {
    flex-direction: column;
    align-items: stretch;
  }
  .result-left {
    justify-content: flex-start;
    width: 100%;
  }
}
</style>
