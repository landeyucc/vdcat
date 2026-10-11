<template>
  <v-card class="mb-2 pa-2" color="surface" :elevation="0">
    <v-card-item>
      <v-row align="center" no-gutters>
        <v-col cols="auto" class="pr-3">
          <v-avatar size="44" rounded="md" color="surface-variant">
            <v-icon :icon="t.kind === 'cover' ? 'mdi-image' : 'mdi-video'" />
          </v-avatar>
        </v-col>
        <v-col>
          <div class="d-flex align-center">
            <span class="text-body-2 font-weight-medium text-truncate" style="max-width: 380px">
              {{ t.title || '未命名' }}
            </span>
            <v-chip v-if="t.platform" size="x-small" class="ml-2" color="surface-variant" variant="flat">
              {{ platformText(t.platform) }}
            </v-chip>
            <v-chip size="x-small" class="ml-2" :color="statusColor(t.status)" variant="tonal">
              {{ statusText(t.status) }}
            </v-chip>
          </div>
          <v-progress-linear :model-value="t.progress"
            :color="t.status === 'error' || t.status === 'interrupted' ? 'error' : 'primary'"
            height="6" rounded class="mt-2 mb-1"
            :indeterminate="t.status === 'running' && t.total === 0" />
          <div class="d-flex text-caption text-medium-emphasis">
            <span>{{ formatBytes(t.loaded) }} / {{ t.total ? formatBytes(t.total) : '未知' }}</span>
            <span v-if="t.speed && t.status === 'running'" class="ml-3">{{ formatBytes(t.speed) }}/s</span>
            <span v-if="t.error" class="ml-3 text-error">{{ t.error }}</span>
          </div>
        </v-col>
        <v-col cols="auto" class="d-flex">
          <v-btn v-if="t.status === 'error' || t.status === 'interrupted'"
            icon="mdi-refresh" variant="text" size="small" @click="$emit('retry', t.id)" />
          <v-btn v-if="t.status === 'queued' || t.status === 'running'"
            icon="mdi-close" variant="text" size="small" @click="$emit('cancel', t.id)" />
          <v-btn icon="mdi-trash-can-outline" variant="text" size="small"
            @click="$emit('remove', t)" />
        </v-col>
      </v-row>
    </v-card-item>
  </v-card>
</template>

<script setup>
defineProps({ t: { type: Object, required: true } })
defineEmits(['retry', 'cancel', 'remove'])

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
</script>
