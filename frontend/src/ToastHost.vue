<template>
  <!-- 右下角通知容器：多条自上而下堆叠，每条 5s 自动消失 -->
  <div class="toast-host" aria-live="polite">
    <TransitionGroup name="toast" tag="div" class="toast-list">
      <div v-for="m in messages" :key="m.id"
        class="toast-card d-flex align-center pa-3 rounded-lg elevation-6"
        :class="`toast-${m.type}`">
        <v-icon :icon="m.icon" class="mr-3 flex-shrink-0" size="22" />
        <div class="toast-text text-body-2 flex-grow-1">{{ m.text }}</div>
        <v-btn icon="mdi-close" variant="text" size="x-small" class="ml-2 flex-shrink-0"
          @click="toast.dismiss(m.id)" />
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup>
import { useToastMessages } from './toast'

const messages = useToastMessages()
</script>

<style scoped>
.toast-host {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 3000;
  pointer-events: none;
}
.toast-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-end;
}
.toast-card {
  pointer-events: auto;
  min-width: 260px;
  max-width: min(420px, calc(100vw - 32px));
  color: #fff;
  border-left: 4px solid rgba(255, 255, 255, 0.45);
}
.toast-success { background: #2e7d32; }
.toast-error   { background: #c62828; }
.toast-warning { background: #ef6c00; }
.toast-info    { background: #1565c0; }
.toast-text {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
}

/* 进入/离开动画 */
.toast-enter-active,
.toast-leave-active {
  transition: all 0.28s cubic-bezier(0.4, 0, 0.2, 1);
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(40px) scale(0.96);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(40px) scale(0.96);
}
.toast-leave-active {
  position: absolute;
  right: 0;
}
.toast-move {
  transition: transform 0.28s ease;
}
</style>
