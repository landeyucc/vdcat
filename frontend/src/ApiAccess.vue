<template>
  <v-card color="surface" elevation="0" class="pa-2">
    <v-card-item>
      <v-card-title class="d-flex align-center text-h6">
        <v-icon icon="mdi-api" class="mr-2" color="primary" />
        浏览器扩展接入
      </v-card-title>
      <v-card-subtitle>
        生成独立 API Key 供浏览器扩展调用。扩展使用 X-Api-Key 请求头鉴权，与网页登录态互不影响；停用或删除 Key 后该扩展立即失效。
      </v-card-subtitle>
    </v-card-item>

    <v-card-text>
      <!-- 请求地址 -->
      <div class="text-body-2 text-medium-emphasis mb-2">服务请求地址（Base URL）</div>
      <v-card variant="flat" color="surface-variant" class="pa-3 mb-5">
        <div class="d-flex align-center flex-wrap">
          <v-text-field :model-value="baseUrl" readonly density="compact" variant="outlined"
            class="flex-grow-1 mr-2 mb-0" prepend-icon="mdi-link-variant" hide-details />
          <v-btn variant="tonal" prepend-icon="mdi-content-copy" @click="copy(baseUrl)">复制</v-btn>
        </div>
        <div class="text-caption text-medium-emphasis mt-2">
          浏览器扩展请填写此地址（本机通常为 http://127.0.0.1:8626）。部署到其它主机/端口时以实际访问地址为准。
        </div>
      </v-card>

      <!-- API Key 列表 -->
      <div class="d-flex align-center mb-3">
        <div class="text-body-2 text-medium-emphasis">我的 API Key</div>
        <v-spacer />
        <v-btn color="primary" variant="tonal" size="small" prepend-icon="mdi-plus" @click="openCreate">
          新建 Key
        </v-btn>
      </div>

      <v-card v-for="k in keys" :key="k.id" variant="flat" color="surface-variant" class="pa-3 mb-2">
        <div class="d-flex align-center mb-2">
          <v-icon size="18" :icon="k.enabled ? 'mdi-key-chain' : 'mdi-key-remove-outline'"
            :color="k.enabled ? 'primary' : 'warning'" class="mr-2" />
          <span class="text-body-2 font-weight-medium">{{ k.name || '未命名 Key' }}</span>
          <v-chip size="x-small" class="ml-2" :color="k.enabled ? 'success' : 'warning'" variant="tonal">
            {{ k.enabled ? '启用中' : '已停用' }}
          </v-chip>
          <v-spacer />
          <v-btn :icon="k.enabled ? 'mdi-pause' : 'mdi-play'" variant="text" size="small"
            @click="toggle(k)" />
          <v-btn icon="mdi-refresh" variant="text" size="small" @click="rotate(k)" />
          <v-btn icon="mdi-delete-outline" variant="text" size="small" color="error" @click="remove(k)" />
        </div>
        <div class="d-flex align-center flex-wrap">
          <code class="key-text flex-grow-1 mr-2">{{ k.api_key }}</code>
          <v-btn variant="text" size="small" prepend-icon="mdi-content-copy" @click="copy(k.api_key)">
            复制 Key
          </v-btn>
        </div>
        <div class="text-caption text-medium-emphasis mt-2">
          最近使用：{{ k.last_used_at ? formatTime(k.last_used_at) : '从未' }}
        </div>
      </v-card>

      <v-card v-if="keys.length === 0" variant="flat" color="surface-variant" class="pa-6 text-center">
        <v-icon icon="mdi-key-minus" size="40" color="on-surface-variant" />
        <div class="text-body-2 text-medium-emphasis mt-2">暂无 API Key，点击右上角「新建 Key」生成</div>
      </v-card>

      <!-- 调用示例 -->
      <template v-if="sampleKey">
        <div class="text-body-2 text-medium-emphasis mb-2 mt-6">调用示例</div>

        <v-tabs v-model="tab" color="primary" variant="text" grow class="mb-0">
          <v-tab value="fetch">fetch</v-tab>
          <v-tab value="curl">curl</v-tab>
        </v-tabs>
        <v-window v-model="tab">
          <v-window-item value="fetch">
            <pre class="code-block">{{ fetchExample }}</pre>
          </v-window-item>
          <v-window-item value="curl">
            <pre class="code-block">{{ curlExample }}</pre>
          </v-window-item>
        </v-window>
        <v-btn variant="text" size="small" class="mt-1" prepend-icon="mdi-content-copy"
          @click="copy(tab === 'curl' ? curlExample : fetchExample)">
          复制示例
        </v-btn>

        <!-- 端点说明 -->
        <v-simple-table class="mt-4 api-table">
          <thead>
            <tr><th>端点</th><th>方法</th><th>说明</th></tr>
          </thead>
          <tbody>
            <tr>
              <td><code>/api/external/parse</code></td>
              <td>POST</td>
              <td>仅解析，返回无水印直链 / 封面 / 标题等信息</td>
            </tr>
            <tr>
              <td><code>/api/external/download</code></td>
              <td>POST</td>
              <td>解析并直接加入下载队列（body 可带 cover:true 同时下封面）</td>
            </tr>
            <tr>
              <td><code>/api/external/tasks</code></td>
              <td>GET</td>
              <td>查询进行中的下载任务与进度</td>
            </tr>
          </tbody>
        </v-simple-table>
      </template>
    </v-card-text>

    <!-- 新建 Key -->
    <v-dialog v-model="dialog" max-width="460">
      <v-card>
        <v-card-title>新建 API Key</v-card-title>
        <v-card-text>
          <v-text-field v-model="newName" label="备注名（如：Chrome 扩展）" density="compact"
            prepend-icon="mdi-tag-outline" @keyup.enter="create" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :loading="creating" @click="create">生成</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-card>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from './api'
import { toast } from './toast'
import { confirmDialog } from './confirm'

const keys = ref([])
const dialog = ref(false)
const creating = ref(false)
const newName = ref('')
const tab = ref('fetch')

const baseUrl = computed(() => window.location.origin)
const sampleKey = computed(() => {
  const on = keys.value.find((k) => k.enabled)
  return (on || keys.value[0])?.api_key || ''
})

const fetchExample = computed(() => `fetch("${baseUrl.value}/api/external/download", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Api-Key": "${sampleKey.value || '你的APIKey'}"
  },
  body: JSON.stringify({ url: "分享链接或视频页链接", cover: true })
})
  .then(r => r.json())
  .then(console.log)`)

const curlExample = computed(() => `curl -X POST "${baseUrl.value}/api/external/download" \\
  -H "Content-Type: application/json" \\
  -H "X-Api-Key: ${sampleKey.value || '你的APIKey'}" \\
  -d '{"url":"分享链接或视频页链接","cover":true}'`)

async function load() {
  try {
    keys.value = await api.listAPIKeys()
  } catch {
    keys.value = []
  }
}

function openCreate() {
  newName.value = ''
  dialog.value = true
}

async function create() {
  creating.value = true
  try {
    await api.createAPIKey(newName.value.trim())
    dialog.value = false
    toast.success('API Key 已生成')
    await load()
  } catch (e) {
    toast.error(e.message)
  } finally {
    creating.value = false
  }
}

async function toggle(k) {
  try {
    await api.toggleAPIKey(k.id, !k.enabled)
    await load()
  } catch (e) { toast.error(e.message) }
}

async function rotate(k) {
  const ok = await confirmDialog({
    title: '重置 API Key',
    message: `重置后「${k.name || k.id}」的旧 Key 立即失效，需在扩展中更新，确定继续？`,
    confirmText: '重置',
    danger: true,
    icon: 'mdi-refresh',
  })
  if (!ok) return
  try {
    await api.rotateAPIKey(k.id)
    await load()
    toast.success('API Key 已重置')
  } catch (e) { toast.error(e.message) }
}

async function remove(k) {
  const ok = await confirmDialog({
    title: '删除 API Key',
    message: `确定删除「${k.name || k.id}」？删除后使用该 Key 的扩展将无法调用。`,
    confirmText: '删除',
    danger: true,
    icon: 'mdi-key-remove-outline',
  })
  if (!ok) return
  try {
    await api.deleteAPIKey(k.id)
    await load()
    toast.success('API Key 已删除')
  } catch (e) { toast.error(e.message) }
}

async function copy(text) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success('已复制')
  } catch {
    toast.error('复制失败，请手动选择复制')
  }
}

function formatTime(ts) {
  const d = new Date(ts)
  if (isNaN(d.getTime())) return ts
  return d.toLocaleString()
}

onMounted(load)
</script>

<style scoped>
.key-text {
  font-size: 13px;
  word-break: break-all;
  background: rgb(var(--v-theme-surface));
  padding: 6px 10px;
  border-radius: 6px;
}
.code-block {
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 8px;
  padding: 12px 14px;
  font-size: 12.5px;
  line-height: 1.6;
  overflow-x: auto;
  white-space: pre;
  margin: 8px 0 0;
}
.api-table code {
  font-size: 12px;
}
</style>
