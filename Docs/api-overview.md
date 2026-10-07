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
| POST | /api/admin/system/reset | 恢复初始化（super_admin，重新认证+确认短语） |

## 鉴权头

- 会话：`Cookie: islet_session=...`（管理端口本地登录）
- OIDC：`Authorization: Bearer {access_token}`（+ 可选 `X-OIDC-ID-Token`）
- API Token：`Authorization: Bearer isl_...`
