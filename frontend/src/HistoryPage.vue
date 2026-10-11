<template>
  <div>
    <!-- 过滤工具栏 -->
    <v-card color="surface" elevation="0" class="pa-3 mb-4">
      <v-row align="center" no-gutters>
        <v-col cols="12" sm="4" class="pr-sm-3 mb-2 mb-sm-0">
          <v-select v-model="platformFilter" :items="platformOptions" item-title="title"
            item-value="value" label="平台" density="compact" variant="outlined" prepend-inner-icon="mdi-filter-variant"
            hide-details @update:model-value="onFilterChange" />
        </v-col>
        <v-col cols="12" sm="5" class="pr-sm-3 mb-2 mb-sm-0">
          <v-select v-model="groupFilter" :items="groupOptions" item-title="title" item-value="value"
            label="分组" density="compact" variant="outlined" prepend-inner-icon="mdi-folder-download-outline"
            hide-details :loading="loadingGroups" @update:model-value="reload(1)" />
        </v-col>
        <v-col cols="8" sm="2" class="pr-sm-2">
          <v-select v-model.number="pageSize" :items="sizeOptions" label="每页" density="compact"
            variant="outlined" hide-details @update:model-value="onSizeChange" />
        </v-col>
        <v-col cols="4" sm="1" class="text-right">
          <v-btn icon="mdi-refresh" variant="tonal" :loading="loading" @click="reload()" />
        </v-col>
      </v-row>
    </v-card>

    <div class="d-flex align-center mb-3">
      <span class="text-body-2 text-medium-emphasis">共 {{ total }} 条</span>
    </div>

    <v-card v-if="groups.length === 0 && !loading" color="surface" elevation="0" class="pa-10 text-center">
      <v-icon icon="mdi-history" size="56" color="on-surface-variant" />
      <div class="text-body-1 text-medium-emphasis mt-3">暂无符合条件的下载历史</div>
    </v-card>

    <v-card v-for="g in groups" :key="g.id" class="mb-4 pa-3" color="surface" elevation="0">
      <div class="d-flex align-center mb-3">
        <v-icon icon="mdi-folder-download-outline" class="mr-2" color="secondary" />
        <span class="text-body-1 font-weight-bold">{{ g.name }}</span>
        <v-chip size="x-small" class="ml-2" color="surface-variant">成功 {{ g.done }} / 共 {{ g.total }}</v-chip>
        <v-chip v-if="g.error" size="x-small" class="ml-1" color="error" variant="tonal">失败 {{ g.error }}</v-chip>
        <v-chip v-if="g.canceled" size="x-small" class="ml-1" color="warning" variant="tonal">取消 {{ g.canceled }}</v-chip>
        <v-spacer />
        <v-btn size="small" variant="text" color="error" prepend-icon="mdi-delete-sweep-outline"
          @click="$emit('remove-group', g)">删除整组</v-btn>
      </div>
      <TaskCard v-for="t in g.items" :key="t.id" :t="t"
        @retry="(v) => $emit('retry', v)"
        @cancel="(v) => $emit('cancel', v)"
        @remove="(v) => $emit('remove', v)" />
    </v-card>

    <div v-if="total > 0" class="d-flex justify-center mt-4">
      <v-pagination v-model="page" :length="pages" total-visible="7" density="compact" rounded
        @update:model-value="reload()" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { api } from './api'
import TaskCard from './TaskCard.vue'

const emit = defineEmits(['retry', 'cancel', 'remove', 'remove-group', 'changed'])

const platformOptions = [
  { title: '全部平台', value: '' },
  { title: '快手', value: 'kuaishou' },
  { title: '抖音', value: 'douyin' },
]
const sizeOptions = [10, 20, 30, 50]

const platformFilter = ref('')
const groupFilter = ref('')
const pageSize = ref(10)
const page = ref(1)
const total = ref(0)
const groups = ref([])
const groupList = ref([])
const loading = ref(false)
const loadingGroups = ref(false)

const pages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const groupOptions = computed(() => {
  const opts = [{ title: '全部分组', value: '' }]
  for (const g of groupList.value) {
    opts.push({ title: `${g.group_name || g.group_id}（${g.count}）`, value: g.group_id })
  }
  return opts
})

async function loadGroups() {
  loadingGroups.value = true
  try {
    groupList.value = await api.historyGroups(platformFilter.value)
    // 当前选中分组在新平台下不存在时清空
    if (groupFilter.value && !groupList.value.some((g) => g.group_id === groupFilter.value)) {
      groupFilter.value = ''
    }
  } catch {
    groupList.value = []
  } finally {
    loadingGroups.value = false
  }
}

async function reload(toPage) {
  if (toPage) page.value = toPage
  loading.value = true
  try {
    const d = await api.history(page.value, pageSize.value, {
      platform: platformFilter.value,
      group_id: groupFilter.value,
    })
    groups.value = d.groups || []
    total.value = d.total || 0
    // 删除后可能当前页变空
    if (groups.value.length === 0 && page.value > 1 && total.value > 0) {
      page.value = Math.max(1, Math.ceil(total.value / pageSize.value))
      await reload()
      return
    }
    emit('changed')
  } catch {
    groups.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

async function onFilterChange() {
  groupFilter.value = ''
  await loadGroups()
  await reload(1)
}

async function onSizeChange() {
  await reload(1)
}

// 供父组件在删除任务/重试后或切换到本页时调用刷新
defineExpose({ reload, loadGroups })
</script>
