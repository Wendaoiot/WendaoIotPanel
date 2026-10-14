import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  // base 保持默认 "/"：管理后台部署在根域，资源需解析为 /assets/...
  // 注意：/iot/ 子路径只属于 web-app（在其 manifest.json 配置 h5 base），这里不要设置。
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    port: 3000,
    proxy: {
      '/api/v1/ws': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true
      },
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
