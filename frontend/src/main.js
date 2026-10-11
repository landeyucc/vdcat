import { createApp } from 'vue'
import { createVuetify } from 'vuetify'
import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import App from './App.vue'
import { mdDark, mdLight } from './theme'
import { initTheme } from './theme-store'

const vuetify = createVuetify({
  theme: {
    defaultTheme: 'mdLight',
    themes: { mdDark, mdLight },
  },
  defaults: {
    VBtn: { rounded: 'lg' },
    VCard: { rounded: 'xl' },
    VTextField: { variant: 'outlined', density: 'comfortable', hideDetails: 'auto' },
  },
})

// 首次加载：默认跟随浏览器策略；用户手动选择后持久化于 localStorage。
// 之后系统主题变化时，仅在「自动」模式下跟随。
initTheme((name) => {
  vuetify.theme.global.name.value = name
})

createApp(App).use(vuetify).mount('#app')
