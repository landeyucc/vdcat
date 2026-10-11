<template>
  <v-card color="surface" elevation="0" class="pa-2">
    <v-card-item>
      <v-card-title class="d-flex align-center text-h6">
        <v-icon icon="mdi-cookie-cog-outline" class="mr-2" color="primary" />
        Cookie 管理
      </v-card-title>
      <v-card-subtitle>
        同平台可添加多个 Cookie，默认使用首位；解析失败会自动尝试下一个。拖动序号无影响，数字越小越优先。
      </v-card-subtitle>
    </v-card-item>

    <v-card-text>
      <!-- 平台切换 -->
      <v-btn-toggle v-model="platform" mandatory color="primary" variant="outlined" density="compact" class="mb-4">
        <v-btn value="kuaishou" prepend-icon="mdi-alpha-k-box">快手</v-btn>
        <v-btn value="douyin" prepend-icon="mdi-music-note">抖音</v-btn>
      </v-btn-toggle>

      <!-- 列表 -->
      <v-card v-for="(c, idx) in cookies" :key="c.id" variant="flat" color="surface-variant" class="pa-3 mb-2">
        <v-row align="center" no-gutters>
          <v-col cols="auto" class="pr-3 text-center" style="min-width: 42px">
            <div class="text-h6 font-weight-bold" :class="c.enabled ? 'text-primary' : 'text-medium-emphasis'">
              {{ idx + 1 }}
            </div>
            <div class="text-caption text-medium-emphasis">优先</div>
          </v-col>
          <v-col>
            <div class="d-flex align-center">
              <v-icon size="16" :icon="c.enabled ? 'mdi-check-circle' : 'mdi-pause-circle'"
                :color="c.enabled ? 'success' : 'warning'" class="mr-1" />
              <span class="text-body-2 font-weight-medium">{{ c.label || '未命名' }}</span>
              <v-chip size="x-small" class="ml-2" :color="c.enabled ? 'success' : 'warning'" variant="tonal">
                {{ c.enabled ? '启用中' : '已停用' }}
              </v-chip>
            </div>
            <div class="text-caption text-medium-emphasis mt-1 font-monospace">{{ c.preview }}</div>
          </v-col>
          <v-col cols="auto">
            <v-btn icon="mdi-pencil-outline" variant="text" size="small" @click="edit(c)" />
            <v-btn :icon="c.enabled ? 'mdi-pause' : 'mdi-play'" variant="text" size="small"
              @click="toggle(c)" />
            <v-btn icon="mdi-delete-outline" variant="text" size="small" color="error" @click="remove(c)" />
          </v-col>
        </v-row>
      </v-card>

      <v-card v-if="cookies.length === 0" variant="flat" color="surface-variant" class="pa-6 text-center">
        <v-icon icon="mdi-cookie-off-outline" size="40" color="on-surface-variant" />
        <div class="text-body-2 text-medium-emphasis mt-2">
          暂无{{ platformLabel() }} Cookie，部分视频可能需要登录态
        </div>
      </v-card>

      <v-btn block variant="tonal" prepend-icon="mdi-plus" class="mt-3" @click="openAdd">
        添加{{ platformLabel() }} Cookie
      </v-btn>
    </v-card-text>

    <!-- 新增/编辑对话框 -->
    <v-dialog v-model="dialog" max-width="640">
      <v-card>
        <v-card-title>{{ form.id ? '编辑' : '添加' }}{{ platformLabel() }} Cookie</v-card-title>
        <v-card-text>
          <v-text-field v-model="form.label" label="备注名（如：小号A）" density="compact" class="mb-2" />
          <v-textarea v-model="form.content" label="Cookie（整段粘贴，可带 Cookie: 前缀）"
            rows="5" prepend-inner-icon="mdi-text-box-outline" density="compact" />
          <v-row align="center" class="mt-1">
            <v-col cols="6">
              <v-text-field v-model.number="form.sort_order" type="number" label="优先级(小=优先)" density="compact" />
            </v-col>
            <v-col cols="6">
              <v-switch v-model="form.enabled" label="启用" color="primary" hide-details density="compact" />
            </v-col>
          </v-row>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :loading="saving" @click="save">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-card>
</template>

<script setup>
import { ref, reactive, watch, onMounted } from 'vue'
import { api } from './api'
import { toast } from './toast'
import { confirmDialog } from './confirm'

const emit = defineEmits(['changed'])

const platform = ref('kuaishou')
const cookies = ref([])
const dialog = ref(false)
const saving = ref(false)
const form = reactive({ id: 0, platform: 'kuaishou', content: '', label: '', enabled: true, sort_order: 0 })

const platformLabel = () => (platform.value === 'kuaishou' ? '快手' : '抖音')

async function load() {
  try {
    cookies.value = await api.listCookies(platform.value)
  } catch (e) {
    cookies.value = []
  }
}

function openAdd() {
  Object.assign(form, { id: 0, platform: platform.value, content: '', label: '', enabled: true, sort_order: 0 })
  dialog.value = true
}

function edit(c) {
  Object.assign(form, {
    id: c.id, platform: c.platform, content: c.content, label: c.label,
    enabled: c.enabled, sort_order: c.sort_order,
  })
  dialog.value = true
}

async function save() {
  if (!form.content.trim()) { toast.warning('Cookie 内容不能为空'); return }
  saving.value = true
  try {
    const payload = {
      platform: platform.value, content: form.content, label: form.label,
      enabled: form.enabled, sort_order: Number(form.sort_order) || 0,
    }
    if (form.id) await api.updateCookie(form.id, payload)
    else await api.createCookie(payload)
    dialog.value = false
    toast.success(form.id ? 'Cookie 已更新' : 'Cookie 已添加')
    await load()
    emit('changed')
  } catch (e) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function toggle(c) {
  try {
    await api.toggleCookie(c.id, !c.enabled)
    await load()
    emit('changed')
  } catch (e) { toast.error(e.message) }
}

async function remove(c) {
  const ok = await confirmDialog({
    title: '删除 Cookie',
    message: `确定删除 Cookie「${c.label || c.id}」？此操作不可恢复。`,
    confirmText: '删除',
    danger: true,
    icon: 'mdi-cookie-remove-outline',
  })
  if (!ok) return
  try {
    await api.deleteCookie(c.id)
    await load()
    emit('changed')
    toast.success('Cookie 已删除')
  } catch (e) { toast.error(e.message) }
}

watch(platform, load)
onMounted(load)
</script>
