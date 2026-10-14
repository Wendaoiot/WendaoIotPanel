import axios from 'axios'
import { ElMessage } from 'element-plus'

const http = axios.create({
  baseURL: '/api/v1',
  timeout: 15000
})

http.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
}, error => {
  return Promise.reject(error)
})

http.interceptors.response.use(response => {
  // 文件下载（blob）不经过统一的 {code,msg,data} 包装，直接返回完整 response
  if (response.config.responseType === 'blob' || response.data instanceof Blob) {
    return response
  }
  const res = response.data
  if (res.code !== 0) {
    ElMessage.error(res.msg || '请求失败')
    return Promise.reject(new Error(res.msg || '请求失败'))
  }
  return res
}, error => {
  if (error.response) {
    const status = error.response.status
    if (status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      // 管理后台部署在根域（base 为 /），'/iot/' 属于 C 端 H5，勿混用
      window.location.href = '/login'
      ElMessage.error('登录已过期，请重新登录')
    } else if (status === 403) {
      ElMessage.error('没有权限执行此操作')
    } else if (status === 500) {
      ElMessage.error('服务器内部错误')
    } else {
      // 导出接口失败时错误体可能是 Blob（responseType=blob），尝试读出后端 msg
      if (error.response.data instanceof Blob) {
        error.response.data.text().then((text: string) => {
          let msg = '请求失败'
          try {
            const j = JSON.parse(text)
            if (j && j.msg) msg = j.msg
          } catch {
            // 非 JSON 错误体，使用默认提示
          }
          ElMessage.error(msg)
        }).catch(() => ElMessage.error('请求失败'))
      } else {
        ElMessage.error(error.response.data?.msg || '请求失败')
      }
    }
  } else {
    ElMessage.error('网络错误，请检查网络连接')
  }
  return Promise.reject(error)
})

export default http
