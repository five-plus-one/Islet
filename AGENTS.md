# Islet — 项目规范

## 项目简介

Islet 是一个自托管多用户图床，基于 Go + Vue 3 构建。支持标准 OIDC 单点登录（PKCE）、本地管理员账号、用户分组配额、多存储策略（本地磁盘 / 阿里云 OSS / S3 兼容对象存储）、PicGo 兼容上传 API、回收站与统计看板。架构模式沿用 GravityLink 的成熟实践。

## 目录约定

```
Islet/
├── AGENTS.md              # 本文件，项目规范
├── Docs/                  # 所有设计文档（进 git）
│   ├── README.md          # 文档索引
│   ├── architecture.md    # 系统架构
│   ├── data-model.md      # 数据库 Schema
│   ├── api-overview.md    # API 设计规范
│   ├── setup.md           # 初始化向导
│   └── storage.md         # 存储策略设计
├── workspace/             # 临时工作区（不进 git）
│   └── status.md          # 当前项目状态（必须及时更新）
├── backend/               # Go 后端
├── frontend/              # Vue 3 前端
│   └── admin/             # 管理端 SPA
└── deploy/                # 部署配置（compose 三套 + DEPLOY.md）
```

## 工作流规范

1. **文档先行**：任何功能改动前，先更新 `Docs/` 对应文档，再动代码
2. **状态同步**：每次开始和结束工作，更新 `workspace/status.md`
3. **workspace 保洁**：临时文件用完即删，不留垃圾文件
4. **不允许边写边设计**：不清楚的需求先在 workspace 草稿，确认后才写入 Docs

## 开发纪律

- 改完主动运行验证（命令见下），不只改不验
- 密钥、token 不进代码、不进 commit、不进日志（使用 `.env`）
- 大改动先进 Plan Mode 出方案，确认后再动手
- 红线操作（删文件、改 .env、数据库 schema 变更、git push）必须先问用户

## 命名约定

- Go 包名：小写单词，无下划线（`internal/router`，`internal/storage`）
- Go 文件名：小写下划线（`image_service.go`，`storage_s3.go`）
- 数据库表名：小写下划线，无前缀（`images`，`storage_policies`，`stat_daily`）
- API 路由：小写中划线（`/api/v1/storage-policies`，`/api/admin/user-groups`）
- Vue 组件：PascalCase（`ImageCard.vue`，`PolicyDrawer.vue`）

## 端口约定

| 端口 | 用途 | 建议 |
|------|------|------|
| `8080`（容器内） | 公网：图片访问 `/i`、缩略图 `/t`、Token 上传 API | 对外开放 |
| `8081`（容器内） | 管理端：SPA、管理 API、初始化向导 | 仅内网或加防火墙 / 反代 |

宿主机默认映射 `18090` / `18091`，通过 `APP_PORT` / `ADMIN_PORT` 覆盖。

## 验证命令

```bash
# 后端
cd backend && go build ./... && go test ./...

# 前端
cd frontend/admin && npm run type-check && npm run build

# 部署配置
docker compose -f deploy/docker-compose.yml config
```
