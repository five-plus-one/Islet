# Islet 服务器部署指南

## 快速开始

### 方式一：本地构建（开发 / 全栈一键起）

```bash
cp deploy/.env.example deploy/.env   # 修改 MySQL 密码

docker compose --env-file deploy/.env -f deploy/docker-compose.yml up --build -d
```

### 方式二：预构建镜像（服务器推荐）

```bash
cp deploy/.env.example deploy/.env

docker compose --env-file deploy/.env -f deploy/docker-compose.server.yml up -d
```

### 方式三：外部已有 MySQL / Redis

```bash
docker compose -f deploy/docker-compose.external-db.yml up -d
```

连接信息在初始化向导中填写（支持「测试连接」后再继续）。

### 默认端口

| 宿主端口 | 用途 | 建议 |
|----------|------|------|
| `18090`（容器 8080） | 公网：图片 `/i/{key}`、缩略图 `/t/{key}`、Token 上传 `/api/v1/upload` | 对外开放 |
| `18091`（容器 8081） | 管理端：SPA、管理 API、初始化向导 | 仅内网 / 反代 + 访问控制 |

通过 `APP_PORT` / `ADMIN_PORT` 覆盖。

## 首次初始化

MySQL / Redis 与 OIDC 配置**不需要提前写进文件**。启动后打开 `http://127.0.0.1:18091`：

1. **数据服务**：填写 MySQL / Redis（Redis 可留空，留空时上传限流降级为不限），测试通过后下一步。
2. **管理员身份**：
   - **OIDC**：填 Issuer 与 Client ID → 「检查 Discovery」→ 按 OIDC 标准在 IdP 的 SPA 应用中把回调加入允许列表：
     ```text
     https://admin.example.com/auth/callback        # 正常登录
     https://admin.example.com/setup/auth/callback  # 初始化向导验证
     http://127.0.0.1:18091/auth/callback
     http://127.0.0.1:18091/setup/auth/callback
     ```
     然后点击「用 OIDC 登录验证管理员身份」完成一次真实 PKCE 登录。
   - **本地账号**：用户名 + 密码（≥10 位）。
3. **站点设置**：站点名称、图床公开访问地址（生成图片外链用，通常是 18090 对应的域名）。
4. **完成**：自动建表、种入内置分组（administrators / members）与「本地存储」策略、认领首个超级管理员、锁定初始化接口，业务**进程内热启用**，无需重启容器。

配置写入 `CONFIG_FILE`（容器内 `/data/islet.json`，随 `islet_data` 卷持久化）。

## OIDC 对接说明

Islet 实现标准 OIDC（Authorization Code + PKCE S256），任何符合规范的 IdP 均可接入：

- Issuer：形如 `https://sso.example.com`（后端自动读取 `/.well-known/openid-configuration`）。
- Token 交换由后端代理（`POST /api/v1/auth/oidc/callback`），IdP 的 token endpoint 无需开启 CORS。
- 登录按钮的名称与图标可在管理端「系统设置」自定义（`brand.oidc_label` / `brand.oidc_logo_url`）。
- OIDC 首次登录自动建档，默认**待审核**；可在「系统设置」开启自动通过。管理员在「用户管理」审批并分配分组。

## 反向代理建议

```nginx
# 图床公网（大文件上传需调大 client_max_body_size）
server {
    server_name img.example.com;
    client_max_body_size 256m;
    location / {
        proxy_pass http://127.0.0.1:18090;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

# 管理端（建议加访问控制或内网限制）
server {
    server_name admin.example.com;
    location / {
        proxy_pass http://127.0.0.1:18091;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 生产检查清单

1. **改默认密码**：`deploy/.env` 的 `MYSQL_PASSWORD` / `MYSQL_ROOT_PASSWORD`。
2. **管理端不直接暴露公网**：`18091` 仅内网，或反代后加访问控制。
3. **公开地址正确**：初始化时填写的「图床公开地址」决定图片外链域名。
4. **数据备份**：MySQL 业务库 + Docker 卷 `islet_data`（配置文件、主密钥、本地策略的图片文件都在这里；主密钥丢失会导致 S3 策略密钥无法解密，需重新填写）。
5. **时间同步**：宿主机时间偏差过大会导致对象存储签名失败。

## 故障排查

| 现象 | 处理 |
|------|------|
| 管理端一直显示初始化向导 | 检查 `/data/islet.json` 是否存在且 `installation_complete: true`；容器日志 `docker logs islet` |
| 上传报 401 | Web 端检查是否登录过期；PicGo 检查 `Authorization: Bearer isl_...` 与 Token 状态 |
| 上传报「文件超过大小限制」 | 分组单文件上限 / 站点 `upload.max_bytes` / 反代 `client_max_body_size` 三处逐一检查 |
| OSS 上传报地域不匹配 | Endpoint 需为 `oss-<region>.aliyuncs.com`，Region 填 `<region>`（自动归一化并校验） |
| S3 策略解密失败 | 数据卷与主密钥不同步（只迁库未迁卷）；重新填写 Secret Key 保存即可 |
| 图片 404 | 检查图片所属策略是否停用；本地策略文件是否在 `islet_data` 卷中 |

## img.five-plus-one.com
生产目录 /opt/islet-production；独立数据库 islet，既有 db-network；本地恢复凭据位于 root 专用 credentials.json。每日数据库、运行配置及主密钥备份由 islet-backup.timer 执行；恢复时同时保留 OSS 原对象与配置主密钥。
