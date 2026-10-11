<template>
  <div class="d-flex flex-column fill-height">
    <div class="d-flex align-center px-4 pt-5 pb-4">
      <img src="/favicon.png" alt="VDCAT" class="brand-logo mr-2" />
      <span class="text-h6 font-weight-bold">VDCAT</span>
    </div>

    <v-list nav density="comfortable" class="px-2">
      <v-list-item v-for="item in items" :key="item.value"
        :color="tab === item.value ? 'primary' : undefined"
        :variant="tab === item.value ? 'tonal' : 'text'"
        :prepend-icon="item.icon" :title="item.title"
        @click="$emit('select', item.value)" />
    </v-list>

    <v-spacer />

    <v-list class="px-2 pb-3">
      <v-list-item prepend-icon="mdi-cog-outline" title="设置" variant="text"
        @click="$emit('settings')" />
      <v-list-item v-if="authRequired" prepend-icon="mdi-logout-variant" title="退出登录"
        variant="text" @click="$emit('logout')" />
      <div class="text-caption text-medium-emphasis px-4 pt-2">短视频下载工具</div>
    </v-list>
  </div>
</template>

<script setup>
defineProps({
  tab: { type: String, required: true },
  authRequired: { type: Boolean, default: true },
})
defineEmits(['select', 'settings', 'logout'])

const items = [
  { value: 'download', title: '下载', icon: 'mdi-download-circle-outline' },
  { value: 'queue', title: '下载队列', icon: 'mdi-tray-arrow-down' },
  { value: 'history', title: '下载记录', icon: 'mdi-history' },
  { value: 'cookies', title: 'Cookie 管理', icon: 'mdi-cookie-cog-outline' },
  { value: 'api', title: 'API 接入', icon: 'mdi-api' },
  { value: 'files', title: '文件', icon: 'mdi-folder-multiple-outline' },
]
</script>

<style scoped>
.brand-logo {
  width: 30px;
  height: 30px;
  object-fit: contain;
  border-radius: 7px;
}
</style>
