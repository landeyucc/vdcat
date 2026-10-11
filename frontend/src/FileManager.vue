<template>
  <v-card class="pa-2" color="surface" elevation="0">
    <v-card-item class="d-flex align-center py-2">
      <v-card-subtitle v-if="root" class="pt-0 pb-0 text-caption text-truncate mr-3">
        <v-icon icon="mdi-folder-outline" size="14" class="mr-1" />{{ root }}
      </v-card-subtitle>
      <v-spacer />
      <v-btn variant="tonal" size="small" prepend-icon="mdi-refresh" @click="$emit('refresh')">
        刷新
      </v-btn>
    </v-card-item>

    <v-card-text>
      <div v-if="files.length === 0" class="text-center text-medium-emphasis py-8">
        <v-icon icon="mdi-folder-open-outline" size="48" />
        <div class="mt-2">暂无文件</div>
      </div>

      <v-row v-else dense>
        <v-col v-for="f in files" :key="f.rel_path" cols="12" md="6">
          <v-card variant="flat" color="surface-variant" class="pa-3 file-item">
            <v-row align="center" no-gutters>
              <v-col cols="auto" class="pr-3">
                <v-icon :icon="f.is_video ? 'mdi-video' : 'mdi-image'" size="30" :color="f.is_video ? 'primary' : 'secondary'" />
              </v-col>
              <v-col>
                <div class="d-flex align-center" style="min-width:0">
                  <v-chip size="x-small" class="mr-2 flex-shrink-0"
                    :color="parts(f.rel_path).platform === '抖音' ? 'pink-accent-3' : 'amber-darken-2'"
                    variant="tonal">
                    {{ parts(f.rel_path).platform }}
                  </v-chip>
                  <span class="text-body-2 font-weight-medium text-truncate">{{ parts(f.rel_path).task }}</span>
                </div>
                <div class="text-caption text-medium-emphasis text-truncate">
                  {{ f.name }} · {{ formatBytes(f.size) }} · {{ formatTime(f.mod_time) }}
                </div>
              </v-col>
              <v-col cols="auto">
                <v-btn v-if="f.is_video" icon="mdi-play" variant="text" size="small"
                  :href="raw(f.rel_path)" target="_blank" />
                <v-btn icon="mdi-download" variant="text" size="small"
                  :href="raw(f.rel_path)" :download="f.name" />
                <v-btn icon="mdi-delete-outline" variant="text" size="small" color="error"
                  @click="$emit('delete', f)" />
              </v-col>
            </v-row>
          </v-card>
        </v-col>
      </v-row>
    </v-card-text>
  </v-card>
</template>

<script setup>
import { api } from './api'

defineProps({ files: { type: Array, default: () => [] }, root: String })
defineEmits(['refresh', 'delete'])

function raw(p) {
  return api.rawURL(p)
}
// 将 rel_path（平台/任务文件夹/文件名）拆分为平台与任务名
function parts(rel) {
  const segs = (rel || '').split('/').filter(Boolean)
  const file = segs.length ? segs[segs.length - 1] : rel
  const platform = segs.length >= 2 ? segs[0] : ''
  // 任务文件夹名去掉前导 "YYYYMMDD-HHMMSS-" 时间戳便于阅读
  let task = segs.length >= 3 ? segs[segs.length - 2] : ''
  task = task.replace(/^\d{8}-\d{6}-/, '')
  return { platform, task: task || file, file }
}
function formatBytes(n) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(i ? 1 : 0)} ${u[i]}`
}
function formatTime(t) {
  try { return new Date(t).toLocaleString() } catch { return t }
}
</script>

<style scoped>
.file-item {
  height: 100%;
}
</style>
