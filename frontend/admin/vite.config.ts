import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "ISLET_");
  return {
    plugins: [react()],
    server: {
      port: 5174,
      proxy: {
        // 开发时 API 走管理端口
        "/api": {
          target: env.ISLET_ADMIN_URL || "http://127.0.0.1:18091",
          changeOrigin: false,
        },
        "^/i/": env.ISLET_PUBLIC_URL || "http://127.0.0.1:18090",
        "^/t/": env.ISLET_PUBLIC_URL || "http://127.0.0.1:18090",
      },
    },
    build: {
      outDir: "dist",
      chunkSizeWarningLimit: 1500,
    },
  };
});
