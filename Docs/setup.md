# 初始化向导

## 目标

默认支持本地管理员初始化，不依赖 OIDC。安装完成标志在数据库连接、种子数据、管理员与设置持久化成功后写入；临时数据库检查连接及时关闭。安装完成后保留只读 `GET /api/setup/status`，返回 `ready`，写入向导接口停止提供。

Islet 必须能在 MySQL / Redis / OIDC 尚未配置时启动，并在管理端端口提供一次性初始化向导。公网端口不暴露任何初始化写接口。

## 状态机

1. 启动读取环境变量与 `CONFIG_FILE`（容器默认 `/data/islet.json`）。
2. 配置完整且 MySQL、Redis 可连接 → 正常业务路由。
3. 配置缺失或依赖不可用 → 管理端进入 `setup_required`；公网端口返回「服务尚未初始化」。
4. 向导步骤：
   a. **数据服务**：MySQL / Redis，各自「测试连接」，通过后暂存。
   b. **管理员身份**：标准 OIDC 或本地账号二选一。
      - OIDC：填 Issuer / Client ID（可选 Audience、Scopes）→ 后端做 OIDC Discovery 检查并**持久化到配置文件** → 前端发起 PKCE 真实登录（回调 `/setup/auth/callback`）→ 回调页把 token 提交 `/api/setup/verify/admin` → 后端验签通过后暂存 `bootstrap_subject`。
      - 本地：用户名 + 密码（≥10 位），暂存 bcrypt 哈希。
   c. **站点设置**：站点名、图床公开访问地址（生成图片链接用）。
   d. **确认启用**：后端在事务中建库表、种入内置分组与本地存储策略、认领首个 `super_admin`、写安装状态、锁定 setup 接口、清理临时凭据、业务在当前进程内启用。

## 配置优先级

1. 显式设置的非空环境变量
2. `CONFIG_FILE` 持久化配置
3. 开发默认值

compose 传入的空值不覆盖初始化产物；`SETUP_DEFAULT_MYSQL_HOST` / `SETUP_DEFAULT_REDIS_HOST` 只作向导首屏默认值。

## 安装后调整认证

OIDC 连接信息（Issuer / Client ID / Audience / Scopes）与本地登录开关存于 `system_configs`（`auth.oidc_*`、`auth.local_enabled`），super_admin 可在「系统设置」随时修改并做 Discovery 检查，进程内热生效，无需重启。数据库配置优先于配置文件；两处均未配置 OIDC 时登录页仅显示本地登录。

## 安全边界

- `/api/setup/*` 只在管理端监听器注册。
- 初始化完成后全部 setup 写接口返回 `setup_locked`。
- 首个 super_admin 认领在数据库事务内完成，防并发重复认领。
- OIDC 角色声明不作为后台授权依据；管理员身份只来自安装认领与 super_admin 的显式授权。
- 状态接口只回传非敏感配置与缺失项，不回传密码或 DSN。
- 恢复初始化（设置页「清除所有配置」）：要求 super_admin 重新认证 + 确认短语；清 system_configs、安装状态、会话，禁用旧账号，删除运行配置文件并写 reset marker，回退初始化态。图片与存储记录保留。
