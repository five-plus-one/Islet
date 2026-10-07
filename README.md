# Islet

Islet 是一个自托管多用户图床，基于 Go + Vue 3 构建。标准 OIDC 单点登录（PKCE）、本地管理员账号、用户分组配额、多存储策略（本地磁盘 / 阿里云 OSS / S3 兼容对象存储）、PicGo 兼容上传 API、回收站与统计看板。单容器双端口部署，前端嵌入 Go 二进制。

## 功能

- **认证**：标准 OIDC（Authorization Code + PKCE S256，后端代理 token 交换，兼容 Logto / Authentik / Keycloak 等任何规范 IdP）或本地 bcrypt 管理员账号；RBAC（super_admin / admin / user）。OIDC 登录按钮的名称与图标可在后台自定义。
- **用户分组**：分组决定「能用多少、能传什么、存到哪里」——总配额、单文件上限、格式白名单、可用存储策略、每小时上传频率。内置管理员组与默认组。
- **多存储策略**：本地磁盘 / S3 兼容（阿里云 OSS 自动 endpoint 归一化与地域校验、MinIO、R2、COS 等），直链 / 302 或应用代理两种访问模式；密钥 AES-256-GCM 加密入库；一键连通性测试。
- **上传**：Web 拖拽 / 粘贴 / 批量，PicGo 兼容 API；sha256 内容去重、自动宽高解析、480px 缩略图、三种命名规则（保留原名 / 随机 / 日期目录）。
- **图片管理**：图库筛选（相册 / 标签 / 类型 / 关键词）、直链 / Markdown / HTML / BBCode 复制、相册、标签、回收站（软删除 + 保留期自动清理）。
- **看板**：总量 / 用量 / 趋势 / 分组分布统计，审计日志。
- **初始化向导**：首次启动在管理端完成 MySQL / Redis / 管理员身份 / 站点设置，无需手改配置文件；业务进程内热启用。

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go、Gin、GORM、minio-go、go-redis |
| 前端 | Vue 3、TypeScript、Vite、Naive UI、Pinia |
| 存储 | MySQL 8（业务数据）、Redis 7（可选：上传限流）、对象存储（图片） |
| 部署 | Docker / Docker Compose，单容器双端口 |

## 快速启动

```bash
cp deploy/.env.example deploy/.env   # 修改 MySQL 密码

# 本地构建
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up --build -d

# 或预构建镜像
docker compose --env-file deploy/.env -f deploy/docker-compose.server.yml up -d
```

启动后访问 `http://127.0.0.1:18091` 完成初始化向导。详细步骤、OIDC 回调地址配置、反向代理示例与故障排查见 [deploy/DEPLOY.md](deploy/DEPLOY.md)。

### 默认端口

| 端口 | 用途 | 建议 |
|------|------|------|
| `18090` | 公网：图片 `/i/{key}`、缩略图 `/t/{key}`、Token 上传 API | 对外开放 |
| `18091` | 管理端：SPA、管理 API、初始化向导 | 仅内网 / 反代 + 访问控制 |

### PicGo 接入

1. 管理端「API Token」创建 Token（`isl_...`，仅创建时展示一次）。
2. PicGo 自定义 Web 图床：URL `http://your-host:18090/api/v1/upload?picgo=1`，Header `Authorization: Bearer isl_...`，POST multipart，字段 `files`。
3. 响应 `{"success":true,"result":[url]}`。

## 文档

- [文档索引](Docs/README.md)
- [系统架构](Docs/architecture.md)
- [数据库模型](Docs/data-model.md)
- [API 设计](Docs/api-overview.md)
- [存储策略](Docs/storage.md)
- [初始化向导](Docs/setup.md)
- [服务器部署](deploy/DEPLOY.md)

## 开发

```bash
# 后端
cd backend && go build ./... && go test ./...

# 前端（Vite 代理 API 到 127.0.0.1:18091）
cd frontend/admin && npm run dev        # 开发
cd frontend/admin && npm run build      # 类型检查 + 构建

# 部署配置
docker compose -f deploy/docker-compose.yml config
```

前端构建产物在 Docker 构建时自动嵌入 `backend/internal/web/admin`；本地开发无需手动复制。

## 项目状态

v0.1.0：认证 / 分组配额 / 多存储策略 / 上传访问 / PicGo / 相册标签 / 回收站 / 统计看板 / 初始化向导 / Docker 化 已全部实现并通过容器全链路验收。
