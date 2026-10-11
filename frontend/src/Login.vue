<template>
  <v-main class="bg-background d-flex align-center justify-center">
    <v-card class="pa-4 login-card" color="surface" elevation="0" style="width: 400px; max-width: 92vw">
      <div class="text-center mb-5 mt-2">
        <img src="/favicon.png" alt="vdcat" class="login-logo" />
        <div class="text-h5 font-weight-bold mt-3">VDCAT</div>
        <div class="text-body-2 text-medium-emphasis">短视频下载服务 · 登录</div>
      </div>

      <v-tabs v-model="tab" grow class="mb-4" color="primary" sliderColor="primary">
        <v-tab value="pass">账号密码</v-tab>
        <v-tab value="token">访问令牌</v-tab>
      </v-tabs>

      <v-window v-model="tab">
        <v-window-item value="pass">
          <v-text-field
            v-model="username"
            label="用户名"
            prepend-icon="mdi-account-outline"
            density="comfortable"
            hide-details="auto"
            class="field-float"
            @keyup.enter="doLogin"
          />
          <v-text-field
            v-model="password"
            label="密码"
            type="password"
            prepend-icon="mdi-lock-outline"
            density="comfortable"
            hide-details="auto"
            class="mt-3 field-float"
            @keyup.enter="doLogin"
          />
        </v-window-item>
        <v-window-item value="token">
          <v-textarea
            v-model="token"
            label="Bearer Token（VDCAT_TOKEN）"
            prepend-icon="mdi-key-outline"
            rows="3"
            density="comfortable"
            hide-details="auto"
            class="field-float"
          />
        </v-window-item>
      </v-window>

      <v-alert v-if="error" type="error" density="compact" variant="tonal" class="mt-3" text="">
        {{ error }}
      </v-alert>

      <v-btn
        color="primary"
        block
        size="large"
        class="mt-5"
        :loading="loading"
        @click="doLogin"
      >
        登录
      </v-btn>
    </v-card>
  </v-main>
</template>

<script setup>
import { ref } from 'vue'
import { api, tokenStore } from './api'

const emit = defineEmits(['success'])

const tab = ref('pass')
const username = ref('admin')
const password = ref('')
const token = ref('')
const error = ref('')
const loading = ref(false)

async function doLogin() {
  error.value = ''
  loading.value = true
  try {
    let data
    if (tab.value === 'pass') {
      data = await api.login({ username: username.value, password: password.value })
    } else {
      data = await api.login({ token: token.value.trim() })
    }
    tokenStore.set(data.token)
    emit('success')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-card {
  border: 1px solid rgba(255, 255, 255, 0.06);
}
/* 浮动 label 上移时不被 v-window 容器裁切，并留出避让空间 */
.field-float :deep(.v-field) {
  margin-top: 4px;
}
.v-window,
.v-window__container,
.v-window-item {
  overflow: visible !important;
}
.login-logo {
  width: 64px;
  height: 64px;
  object-fit: contain;
  border-radius: 14px;
}
</style>
