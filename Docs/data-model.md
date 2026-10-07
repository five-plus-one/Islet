# 数据库模型

MySQL 8，utf8mb4。全部表 GORM AutoMigrate 管理，软删除用 `deleted_at`。

## users

| 列 | 类型 | 说明 |
|----|------|------|
| id | BIGINT PK AI | |
| auth_source | ENUM('oidc','local') | |
| sso_id | VARCHAR(128) NULL UNIQUE | OIDC sub |
| username | VARCHAR(64) NOT NULL UNIQUE | |
| email | VARCHAR(128) NULL UNIQUE | |
| password_hash | VARCHAR(255) NULL | 本地账号 bcrypt |
| role | ENUM('super_admin','admin','user') | 管理权限 |
| group_id | BIGINT NOT NULL DEFAULT 2 | 用户分组（配额/存储） |
| status | ENUM('pending','active','disabled') | OIDC 首登默认 pending |
| last_login_at | DATETIME NULL | |
| created_at / updated_at / deleted_at | | |

## auth_sessions

本地账号会话：user_id、token_hash(SHA-256, UNIQUE)、expires_at、last_seen_at。

## user_groups

用户分组：管「能用多少、能传什么、存到哪里」，与管理权限无关。

| 列 | 类型 | 说明 |
|----|------|------|
| id | BIGINT PK AI | 1=administrators（内置）、2=members（内置默认） |
| name | VARCHAR(64) UNIQUE | |
| description | VARCHAR(255) | |
| quota_bytes | BIGINT | 总配额字节，0=不限 |
| max_file_bytes | BIGINT | 单文件上限，0=不限 |
| allowed_mimes | JSON | 格式白名单，如 ["image/png","image/jpeg","image/*"] |
| policy_ids | JSON | 可用存储策略 ID 列表，空=全部 |
| uploads_per_hour | INT | 每小时上传次数上限，0=不限 |
| built_in | BOOL | 内置组不可删（可改参数） |
| status | ENUM('active','disabled') | 停用后组内用户不可上传 |
| created_at / updated_at | | |

super_admin 不受分组限制。

## storage_policies

| 列 | 类型 | 说明 |
|----|------|------|
| id | BIGINT PK AI | 1=本地存储（内置默认） |
| name | VARCHAR(64) UNIQUE | |
| driver | ENUM('local','s3') | |
| config | TEXT | AES-GCM 加密的驱动配置 JSON |
| url_mode | ENUM('direct','proxy') | s3：直链 CDN 或经应用代理 |
| naming | ENUM('keep','random','date') | keep=保留原名 / random=随机 / date=日期目录+随机 |
| is_default | BOOL | 新上传默认策略（唯一） |
| status | ENUM('active','disabled') | |
| created_at / updated_at / deleted_at | | |

s3 config 明文结构：endpoint、region、bucket、prefix、cdn、access_key、secret_key、path_style。local 无敏感配置。

## images

| 列 | 类型 | 说明 |
|----|------|------|
| id | BIGINT PK AI | |
| user_id | BIGINT INDEX | |
| policy_id | BIGINT | 上传时的策略 |
| album_id | BIGINT NULL INDEX | |
| storage_key | VARCHAR(512) UNIQUE | 策略内对象 key |
| url | VARCHAR(1024) | 最终访问 URL（上传时固化） |
| thumb_key | VARCHAR(512) NULL | 缩略图 key |
| original_name | VARCHAR(255) | |
| mime | VARCHAR(128) INDEX | |
| size | BIGINT | 字节 |
| width / height | INT | |
| sha256 | CHAR(64) INDEX | 同用户去重：(user_id, sha256) |
| tags | JSON | 字符串数组 |
| created_at / updated_at / deleted_at | | 软删除=回收站 |

## albums

id、user_id INDEX、name、description、created_at/updated_at/deleted_at。相册仅归属本人。

## api_tokens

| 列 | 类型 | 说明 |
|----|------|------|
| id | BIGINT PK AI | |
| user_id | BIGINT INDEX | |
| name | VARCHAR(64) | |
| token_hash | CHAR(64) UNIQUE | SHA-256(明文) |
| expires_at | DATETIME NULL | |
| last_used_at | DATETIME NULL | |
| status | ENUM('active','disabled') | |
| created_at | | |

明文形如 `isl_<32hex>`，仅创建时返回一次。

## system_configs

key_name UNIQUE / value TEXT / description / updated_at。已用键：

- `brand.name`、`brand.logo_url`、`brand.oidc_label`、`brand.oidc_logo_url`：品牌与 OIDC 登录按钮文案图标
- `site.public_base_url`：图床公开访问地址（local 策略 URL 生成）
- `site.title`：站点名
- `users.auto_approve`：OIDC 首登是否自动通过（默认 false）
- `recycle.retention_days`：回收站保留天数（默认 30，0=永久）

## installation_state / audit_logs

installation_state：单行，installed、owner_user_id、schema_version、installed_at。审计：user_id、action、target_type、target_id、detail JSON、ip、created_at（INDEX created）。
