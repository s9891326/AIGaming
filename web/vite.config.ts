import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// 後端位址，可用環境變數覆寫：例如 BACKEND_URL=http://localhost:9000 npm run dev
const backend = process.env.BACKEND_URL ?? 'http://localhost:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // 開發時把 /api 與 /ws 代理到 Go 後端，前後端同源，所以後端不需要 CORS。
    proxy: {
      '/api': { target: backend, changeOrigin: true },
      // /ws 不改 Host：後端會檢查 Origin 與 Host 是否同源（防止其他網站連進來）。
      '/ws': { target: backend, ws: true },
    },
  },
})
