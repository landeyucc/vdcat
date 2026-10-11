import { reactive } from 'vue'

// 全局确认对话框（Promise 化，替代浏览器原生 confirm）。
// 支持：标题、正文、确认/取消文案、危险样式、以及「第三选项」(extra)
// 用于删除时“仅移除记录 / 同时删除文件”这类三态选择。
const state = reactive({
  open: false,
  title: '',
  message: '',
  confirmText: '确定',
  cancelText: '取消',
  color: 'primary',
  icon: 'mdi-help-circle-outline',
  // 可选第三按钮（如“同时删除文件”）
  extraText: '',
  _resolve: null,
})

/**
 * 弹出确认框。
 * @returns Promise<boolean|'extra'>  确认=true，取消/关闭=false，第三按钮='extra'
 */
export function confirmDialog(options = {}) {
  state.title = options.title || '请确认'
  state.message = options.message || ''
  state.confirmText = options.confirmText || '确定'
  state.cancelText = options.cancelText || '取消'
  state.color = options.danger ? 'error' : options.color || 'primary'
  state.icon = options.icon || (options.danger ? 'mdi-alert-circle-outline' : 'mdi-help-circle-outline')
  state.extraText = options.extraText || ''
  state.open = true
  return new Promise((resolve) => {
    state._resolve = resolve
  })
}

function settle(val) {
  state.open = false
  const r = state._resolve
  state._resolve = null
  if (r) r(val)
}

export function useConfirmState() {
  return {
    state,
    confirm: () => settle(true),
    cancel: () => settle(false),
    extra: () => settle('extra'),
  }
}
