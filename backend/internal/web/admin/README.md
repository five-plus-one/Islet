# admin 构建产物占位

`frontend/admin` 执行 `npm run build` 后，把 `dist/` 内容复制到本目录（Docker 构建自动完成）：

```
cp -r frontend/admin/dist/* backend/internal/web/admin/
```

本地开发无需此步骤：前端用 `npm run dev`（Vite 代理 API 到 127.0.0.1:18081）。
