/// <reference types="@dcloudio/types" />
/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * 小程序端（MP-WEIXIN）直连的服务器地址，必须为 HTTPS，
   * 且需在微信公众平台配置 request 合法域名，如 https://api.example.com。
   * H5 端无需配置，走 Vite 代理 /api -> 后端。
   */
  readonly VITE_API_BASE_URL?: string
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
