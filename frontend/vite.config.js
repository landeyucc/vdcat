import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'
import { resolve } from 'path'

// 构建产物直接输出到 Go embed 目录
export default defineConfig({
  plugins: [
    vue(),
    vuetify({ autoImport: true }),
  ],
  resolve: {
    alias: { '@': resolve(__dirname, 'src') },
  },
  build: {
    outDir: '../internal/server/web',
    emptyOutDir: true,
  },
  server: {
    port: Number(process.env.VDCAT_FRONT_PORT || 5173),
    proxy: {
      '/api': {
        target: process.env.VDCAT_API_TARGET || 'http://127.0.0.1:8626',
        changeOrigin: true,
      },
    },
  },
})
