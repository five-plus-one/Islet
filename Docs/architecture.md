# 系统架构

## 总览

Islet 由单个 Go 二进制承载全部服务，单容器双端口部署，前端构建产物嵌入二进制。

```
                    ┌────────────────────────────────────┐
 公网用户 ──────────▶│ :8080 公网端口                      │
 (浏览/引用图片)      │  GET/HEAD /i/{key}    原图访问      │
                    │  GET/HEAD /t/{key}    缩略图访问     │
 PicGo / 脚本 ──────▶│  POST /api/v1/upload  Token 上传    │──▶ 存储策略
                    │  GET  /api/v1/health                │   local / s3(OSS等)
                    ├────────────────────────────────────┤
 管理员 / 用户 ─────▶│ :8081 管理端口                       │
 (管理端 SPA)        │  管理界面（嵌入的 React SPA）           │──▶ MySQL 8
                    │  /api/v1/*   业务 API（会话/OIDC）   │──▶ Redis 7
                    │  /api/admin/* 管理 API（admin+）     │
                    │  /api/setup/* 初始化向导（仅此端口）   │
                    └────────────────────────────────────┘
```

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go、Gin、GORM、go-redis、minio-go、golang.org/x/image |
| 前端 | React 19、TypeScript、Vite、5plus1-ui |
| 存储 | MySQL 8（业务数据）、Redis 7（限流/缓存）、对象存储（图片） |
| 部署 | Docker / Docker Compose，预构建镜像 `5plus1/islet` |

## 后端模块

```
backend/
├── cmd/server/            # 入口：装配配置、按安装状态启动
├── internal/
│   ├── app/               # 双 gin engine 装配、生命周期
│   ├── config/            # CONFIG_FILE 读写、env 覆盖、优先级合并
│   ├── database/          # MySQL/Redis 连接、AutoMigrate、种子数据
│   ├── middleware/        # 认证（会话/OIDC JWT/API Token）、RBAC
│   ├── model/             # GORM 模型
│   ├── response/          # 统一响应包 {code,message,data}
│   ├── router/            # 全部路由注册（setup/auth/images/...）
│   ├── service/           # 业务服务（认证、存储、图片、分组、统计）
│   ├── storage/           # 存储驱动接口 + local/s3 实现
│   ├── worker/            # 定时任务（回收站清理、会话清理）
│   └── web/               # 嵌入的前端 dist
└── pkg/                   # 可复用工具（无业务依赖）
```

## 认证模型

三种凭证，统一由 `middleware.AuthRequired` 解析：

1. **会话 Cookie**：本地账号登录后签发（随机 32 字节 token，库存 SHA-256 哈希，12h TTL）。
2. **OIDC Access Token**：前端 PKCE 流程取得，`Authorization: Bearer` 携带；后端按标准 OIDC Discovery 取 JWKS 验签（RS256/384/512、ES256/384/521），校验 iss/aud/exp。可附 `X-OIDC-ID-Token` 同步用户资料。首次登录自动建用户（默认待审核，可配置自动通过）。
3. **API Token**：`Authorization: Bearer isl_...`，库存哈希，用于 PicGo / 脚本上传，仅授权上传与自有图片管理。

角色（管理权限）：`super_admin` / `admin` / `user`。分组（配额与存储）：见 data-model。

## 图片生命周期

```
上传（Web/Token）
  → 分组校验（格式白名单 / 单文件大小 / 总配额 / 频率）
  → sha256 去重（同用户同哈希直接返回既有记录）
  → 生成存储 key（按策略命名规则：保留原名 / 随机 / 日期+随机）
  → 解析宽高 + 生成缩略图（最长边 480px）
  → 写入策略指向的存储 → 落库
访问
  → /i/{key}：local=磁盘流式返回；s3 直链=302 CDN；s3 代理=回源转发
  → /t/{key}：缩略图，缺失时回退原图
删除
  → 软删除进回收站（默认保留 30 天，可配）
  → 回收站恢复 / 彻底删除（先删存储对象再删记录）/ 定时清理
```

## 配置优先级

1. 显式设置的非空环境变量
2. `CONFIG_FILE` 持久化配置（容器默认 `/data/islet.json`）
3. 开发默认值

业务运行时设置（品牌、公开域名、注册策略、回收站保留期）存 `system_configs` 表，管理端可改、进程内缓存热生效；基础设施配置（DB/Redis/OIDC/端口）在配置文件，只能通过初始化向导或恢复初始化修改。
