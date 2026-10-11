import { reactive } from 'vue'

// 全局轻量消息提示（单例 store），由 ToastHost.vue 渲染为右下角通知。
let seq = 0
const messages = reactive([])

const ICONS = {
  success: 'mdi-check-circle-outline',
  error: 'mdi-alert-circle-outline',
  warning: 'mdi-alert-outline',
  info: 'mdi-information-outline',
}

const timers = new Map()

function push(type, text, timeout = 5000) {
  const id = ++seq
  messages.push({
    id,
    type,
    text: String(text ?? ''),
    icon: ICONS[type] || ICONS.info,
    timeout,
  })
  // 5s（默认）后自动消失
  if (timeout > 0) {
    timers.set(id, setTimeout(() => dismiss(id), timeout))
  }
  return id
}

function dismiss(id) {
  const t = timers.get(id)
  if (t) {
    clearTimeout(t)
    timers.delete(id)
  }
  const i = messages.findIndex((m) => m.id === id)
  if (i !== -1) messages.splice(i, 1)
}

export const toast = {
  success: (t, ms) => push('success', t, ms),
  error: (t, ms) => push('error', t, ms),
  warning: (t, ms) => push('warning', t, ms),
  info: (t, ms) => push('info', t, ms),
  dismiss,
}

export function useToastMessages() {
  return messages
}
