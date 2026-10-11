<template>
  <div>
    <div class="d-flex align-center mb-4">
      <v-chip color="surface-variant" size="large">
        <v-icon start icon="mdi-tray-arrow-down" />进行中 {{ tasks.length }}
      </v-chip>
      <v-spacer />
      <v-btn variant="tonal" size="small" prepend-icon="mdi-refresh" @click="$emit('refresh')">刷新</v-btn>
    </div>

    <v-card v-if="groups.length === 0" color="surface" elevation="0" class="pa-10 text-center">
      <v-icon icon="mdi-tray-arrow-down" size="56" color="on-surface-variant" />
      <div class="text-body-1 text-medium-emphasis mt-3">队列为空，去「下载」页粘贴链接开始下载</div>
    </v-card>

    <div v-for="g in groups" :key="g.id" class="mb-4">
      <div class="d-flex align-center mb-2">
        <v-icon icon="mdi-download-multiple" size="18" class="mr-1" color="primary" />
        <span class="text-body-2 font-weight-bold">{{ g.name }}</span>
        <v-chip size="x-small" class="ml-2" color="surface-variant">{{ g.items.length }} 个</v-chip>
      </div>
      <TaskCard v-for="t in g.items" :key="t.id" :t="t"
        @retry="(v) => $emit('retry', v)"
        @cancel="(v) => $emit('cancel', v)"
        @remove="(v) => $emit('remove', v)" />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import TaskCard from './TaskCard.vue'

const props = defineProps({
  tasks: { type: Array, default: () => [] },
})
defineEmits(['refresh', 'retry', 'cancel', 'remove'])

// 下载队列按组聚合（内存实时，仅视频+封面）
const groups = computed(() => {
  const map = new Map()
  const order = []
  for (const t of props.tasks) {
    const gid = t.group_id || '_default'
    if (!map.has(gid)) {
      map.set(gid, { id: gid, name: t.group_name || '未分组', items: [] })
      order.push(gid)
    }
    map.get(gid).items.push(t)
  }
  return order.map((g) => map.get(g))
})
</script>
