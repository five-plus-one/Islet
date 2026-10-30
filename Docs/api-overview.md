# API 设计

统一响应包：`{"code": 0, "message": "ok", "data": ...}`；错误 code 四位：4000 参数 / 4401 未认证 / 4403 无权限 / 4404 不存在 / 4500 上传链路 / 5000 服务器。

## 公网端口（:8080）

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | /api/v1/health | - | 健康检查（含安装状态标记） |
| GET/HEAD | /i/{key...} | - | 原图；local 流式 / s3 direct 302 / s3 proxy 转发；ETag+immutable 缓存 |
| GET/HEAD | /t/{key...} | - | 缩略图；缺失回退原图 |
| POST | /api/v1/upload | Token/JWT/会话 | multipart `files`；PicGo 兼容响应 `{"success":true,"result":[urls]}` |

## 管理端口（:8081）

初始化（仅未安装时可用，完成后锁定）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/setup/status | setup_required、缺失项、默认值 |
| POST | /api/setup/test/database | 测试 MySQL/Redis |
| POST | /api/setup/test/oidc | OIDC Discovery 检查，返回端点 |
| POST | /api/setup/verify/admin | 提交 OIDC token 或本地账号，暂存管理员身份 |
| POST | /api/setup/complete | 建表、种子、认领 owner、锁定 |

认证：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/auth/config | 模式、issuer、client_id、discovery 端点、品牌（含 OIDC 名称/图标） |
| POST | /api/v1/auth/local/login、/auth/logout、GET /auth/me | 本地登录 / 登出 / 当前用户 |

用户侧（登录用户）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/me | 资料 + 分组 + 配额用量 |
| GET/POST | /api/v1/me/tokens，PUT/DELETE /:id | API Token 管理 |
| GET | /api/v1/images | 本人图库（admin 可加 user_id 看全部）：分页、album_id/tag/mime/关键词/时间筛选、排序 |
| PATCH/DELETE | /api/v1/images/:id | 改名/标签/相册；软删除进回收站 |
| POST | /api/v1/images/:id/restore | 从回收站恢复（校验配额） |
| DELETE | /api/v1/images/:id/purge | 彻底删除（本人或 admin） |
| GET/POST/PATCH/DELETE | /api/v1/albums | 相册 CRUD |
| POST | /api/v1/upload | 上传（同公网端口逻辑） |

管理侧（admin+，super_admin 额外标注）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST/PATCH/DELETE | /api/admin/storage-policies（+/:id/test） | 存储策略 CRUD 与连通测试 |
| GET/POST/PATCH/DELETE | /api/admin/user-groups | 用户分组 CRUD（内置组不可删） |
| GET | /api/admin/users（+/:id/group /:id/role* /:id/status） | 用户管理、审批 |
| GET | /api/admin/stats/overview /trend /by-policy /by-group | 统计看板 |
| GET/PUT | /api/admin/settings | system_configs 读写（品牌、站点、注册、回收站） |
| GET | /api/admin/audit-logs | 审计日志 |
| GET | /api/admin/recycle、POST /purge-all | 回收站总览 / 清空（按保留期） |
| POST | /api/admin/auth/test-oidc | OIDC Discovery 检查（系统设置页用） |
| POST | /api/admin/system/reset | 恢复初始化（super_admin，重新认证+确认短语） |

## OIDC 运行时配置

OIDC 连接信息支持两级来源：`system_configs`（系统设置页可改，热生效）优先于配置文件/环境变量（初始化向导写入）。相关键：`auth.oidc_issuer`、`auth.oidc_client_id`、`auth.oidc_audience`、`auth.oidc_scopes`、`auth.local_enabled`（本地登录开关，默认开）。修改后登录页与 Bearer 验签即时使用新配置。

## 鉴权头

工作台 `/api/v1` 使用交互式会话或 OIDC 身份；上传密钥只用于公网上传。`/api/admin` 额外要求管理员角色，创建本地用户、修改角色与重置系统等操作要求超级管理员。跨来源写请求被拒绝，API 响应禁止缓存。

### 本地账户与图库

- `POST /api/admin/users`：超级管理员创建本地账户，填写用户名、初始密码和分组。
- `POST /api/v1/me/password`：校验当前密码后修改密码，同时撤销现有会话。密码长度为 10–72 字节。
- `GET /api/v1/images`：默认查询本人；管理员显式传入 `scope=all` 才查询全站。分页使用创建时间和 ID 的组合游标，返回 `next_cursor` 和 `has_more`。
- 图片编辑与回收操作校验所有权；相册必须属于图片所有者。永久删除只适用于回收中的图片。
- `/api/v1/me` 提供允许使用的存储名称、公开站点地址及上传限制，存储凭据不下发给普通用户。
- `GET /api/setup/status` 在安装完成后只读返回 `ready`。

- 会话：`Cookie: islet_session=...`（管理端口本地登录）
- OIDC：`Authorization: Bearer {access_token}`（+ 可选 `X-OIDC-ID-Token`）
- API Token：`Authorization: Bearer isl_...`

## 用户保护规则

超级管理员的分组、角色与状态由受保护的管理流程维护；用户列表对其展示保护说明。分组、角色和状态更新先锁定并确认目标存在、可管理，再执行变更；受保护目标返回 403，不存在返回 404，参数无效返回 400。拒绝请求不撤销会话或上传密钥，不记录成功审计。普通成员停用仍在同一事务内撤销会话并禁用上传密钥。

2026-10-30 验收覆盖超级管理员操作返回 403 且会话/密钥有效、无效编号 400、不存在用户 404、普通成员停用状态回读及会话/密钥撤销。Go 全包测试、前端类型检查、生产构建和四组 API 验收通过。
