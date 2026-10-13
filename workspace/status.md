# Islet 项目状态

更新时间：2026-10-08

## 当前阶段

**工作台改造完成**：统一视觉与响应式布局、上传队列、图库与回收站、管理页面、本地用户管理及后端权限和存储保护已实现。Go 构建/测试/vet、前端类型/构建、三套 Compose、七组浏览器业务验收及真实 MinIO 集成通过。结果见 `Docs/acceptance-2026-10-08.md`。

本地 18091 实例已更新，沿用既有账户与数据卷；旧镜像保留为 `islet:previous-20261008`。外部 OIDC、云厂商存储与公网部署专项验证范围见验收记录。

## 白屏事件（2026-10-07）

- 现象：管理端 18091 全白屏，Vue 已挂载但路由不渲染
- 根因：go:embed 默认排除 _ 开头文件，Vite chunk _plugin-vue_export-helper-*.js 未嵌入，
  请求被 SPA fallback 成 index.html（200+HTML），模块加载器拒载，懒加载路由全部失败
- 修复：//go:embed all:admin + /assets/ 缺失显式 404（提交 61e75d3）
- 教训：嵌入前端产物必须用 all: 前缀；SPA fallback 不得覆盖 /assets/ 路径

## Git

- 2026-10-05 初始化仓库（默认分支 `main`），首个提交 `8cfb13e` 含全部 80 个文件，已打 `v0.1.0` 标签
- 无远程仓库；`git push` 属红线操作，待用户配置远程后另行确认

## 进度

- [x] P0 仓库脚手架 + 文档（AGENTS / Docs / workspace）
- [x] P1 后端骨架：配置体系（env > 文件 > 默认值）、双端口、初始化向导状态机、标准 OIDC（Discovery + PKCE + JWKS 验签 RS/ES 全家族）+ 本地账号 + RBAC
- [x] P2 后端核心：存储策略（local/s3 驱动、AES-GCM 密钥、OSS 归一化、连通测试）、用户分组（配额/白名单/频率）、上传链路（去重/缩略图/命名规则）、/i /t 访问（ETag + immutable）、相册、回收站、统计、审计、worker
- [x] P4 API Token + PicGo 兼容上传（公网端口，`?picgo=1` 响应格式）
- [x] P3 前端：12 个视图（向导/登录/回调/看板/图库/上传/相册/Token/策略/分组/用户/设置/回收站/个人中心），Naive UI
- [x] P5 Dockerfile 三阶段（前端 dist 嵌入二进制）+ 三套 compose + DEPLOY.md，镜像 `5plus1/islet:0.1.0` 构建成功
- [x] 验证：`go build ./... && go test ./...` 全绿（OIDC 验签/密钥箱/存储驱动/配置状态机/中间件）；`vue-tsc && vite build` 通过；三套 compose config 校验通过

## 容器全链路验收记录（2026-10-05）

独立容器 + MySQL 8 网络，覆盖：

- 初始化向导 API 全流程：test/database → verify/admin（本地）→ complete（DB 凭据落盘 + 建表 + 种子 + owner 认领 + 热启用）
- complete 后 setup 接口不可达（完整运行态无 setup 路由）
- 本地登录（session cookie）、/api/v1/auth/me
- multipart 上传 → 日期命名 key → URL 固化（`http://127.0.0.1:18090/i/2026/10/05/<hex>.png`）
- /i 访问 200、content-type 正确、`Cache-Control: public, max-age=31536000, immutable`、ETag；/t 回退原图
- PicGo 格式上传 `{"success":true,"result":[...]}`；sha256 去重（重复上传返回同一记录）
- API Token：创建（一次性明文）→ 公网端口 Token 上传 → 无效 token 401
- 回收站：软删除 → 列表 → 恢复 → 彻底删除（存储文件删除后 /i 404）
- 策略 CRUD（s3 策略密文入库、UTF-8 名称验证）、分组列表、统计 overview

## 验收后修复

- 管理端 SPA 根路径 301 死循环：弃用 http.FileServer（其 /index.html 301 规则），改为直接读嵌入文件返回
- 初始化 complete 缺 DB 密码：向导第 1 步凭据只在前端内存，改为 complete 时随表单一并落盘
- Redis 缺失时业务无法热启用：Redis 改为可选依赖（失败仅限流降级并 warn）
- setupLocked 改为按配置文件安装状态判定（不依赖 DB 与运行态）

## 已定决策

- 标准 OIDC（Discovery + PKCE + 后端代理 token 交换，规避 IdP CORS）；登录按钮名称/图标后台可配
- MySQL 必需，Redis 可选（限流）
- 模块 islet/backend，镜像 5plus1/islet
- 端口：容器 8080 / 8081，宿主默认 18090 / 18091
- 分组管配额与存储，角色管管理权限；OIDC 首登默认待审核（auto_approve 可开）
- 回收站默认保留 30 天；缩略图最长边 480px；>32MB 原图跳过缩略图与宽高解析

## 系统性产品设计（2026-10-07）

- 产品设计：`Docs/product-design.md`。
- 交付：工作台交互与视觉、管理功能、本地用户、权限与图片生命周期、存储读写保护。
- 验收：`Docs/acceptance-2026-10-08.md`。独立验收容器保留，可复现完整测试。

## GitHub 归档（2026-10-13）

- 成果按产品文档、后端、前端、自动化验收四批提交；GitHub 私有仓库：https://github.com/five-plus-one/Islet，主分支 main。
