import { ref } from 'vue'

// 主题模式：auto（跟随浏览器/系统）| light | dark
const THEME_KEY = 'vdcat_theme'

export function getStoredMode() {
  const m = localStorage.getItem(THEME_KEY)
  return m === 'light' || m === 'dark' || m === 'auto' ? m : 'auto'
}

// 解析当前系统是否为深色
export function systemPrefersDark() {
  return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
}

// 把模式解析为实际 Vuetify 主题名
export function resolveTheme(mode) {
  if (mode === 'light') return 'mdLight'
  if (mode === 'dark') return 'mdDark'
  return systemPrefersDark() ? 'mdDark' : 'mdLight'
}

// 全局单例：当前模式（auto/light/dark）与实际主题名
export const themeMode = ref(getStoredMode())

export function setThemeMode(mode) {
  themeMode.value = mode
  if (mode === 'auto') {
    localStorage.removeItem(THEME_KEY)
  } else {
    localStorage.setItem(THEME_KEY, mode)
  }
}

// 首次初始化：注册系统主题变化监听（仅 auto 模式时跟随）。
// 返回当前应使用的 Vuetify 主题名。
export function initTheme(apply) {
  const applyCurrent = () => apply(resolveTheme(themeMode.value))
  applyCurrent()
  if (window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => {
      if (themeMode.value === 'auto') applyCurrent()
    }
    // Safari < 14 用 addListener
    if (mq.addEventListener) mq.addEventListener('change', onChange)
    else if (mq.addListener) mq.addListener(onChange)
  }
}
