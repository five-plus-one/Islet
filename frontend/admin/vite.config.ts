import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5174,
    proxy: {
      // 开发时 API 走管理端口
      '/api': 'http://127.0.0.1:18091',
      '/i': 'http://127.0.0.1:18090',
      '/t': 'http://127.0.0.1:18090'
    }
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500
  }
});
