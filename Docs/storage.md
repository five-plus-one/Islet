# 存储策略设计

## 对象生命周期约束

上传只使用启用且用户分组允许的策略；已有图片通过读取驱动继续访问，即使策略已停用。被图片引用的策略不能删除，也不能直接更改驱动、端点、桶或路径前缀。引用检查包含回收站。

本地文件使用排他创建，S3 使用条件写入，避免同名对象覆盖。图片分享与缩略图统一通过 Islet `/i/{key}`、`/t/{key}` 访问，检查审核状态后再读取存储。对象存储使用私有读取；CDN 如需保留已发布链接，应回源到受控图片入口，禁止跳过状态校验。详见 [图片审核](image-moderation.md)。

永久删除要求图片处于回收状态；对象删除失败时保留数据库记录以便重试。恢复与永久删除使用图片行锁协调。回收、恢复、永久删除均生成持久化缓存刷新任务，后台自动重试并查询全网完成结果。

## 驱动抽象

`internal/storage` 定义接口，策略（storage_policies 行）是驱动的具名实例，可并存任意多条：

```go
type Driver interface {
    // Put 写入对象，返回可存储的相对 key 与最终访问 URL
    Put(ctx, key string, r io.Reader, size int64, contentType string) (url string, err error)
    // Delete 删除对象
    Delete(ctx, key string) error
    // Check 连通性检测：上传探测图并回读校验
    Check(ctx) error
}
```

驱动实现：

- **local**：写 `{DATA_DIR}/uploads/{key}`，URL = `{site.public_base_url}/i/{key}`，经公网端口流式返回。无敏感配置。
- **s3**：minio-go 客户端，覆盖阿里云 OSS、腾讯 COS、MinIO、R2 等 S3 兼容服务。
  - endpoint 归一化：`oss-cn-hangzhou.aliyuncs.com` → `s3.oss-cn-hangzhou.aliyuncs.com`（虚拟主机风格），地域从 host 推导校验。
  - 加速域名回源至应用图片入口；开启阿里云缓存同步后，图片默认返回 CDN 链接，节点缓存 30 天，浏览器每次校验。访问变更自动刷新原图、缩略图及历史路径。未配置加速时由应用流式读取，`url_mode` 保留兼容。
  - 错误翻译为可操作的中文提示（NoSuchBucket / InvalidAccessKeyId / SignatureDoesNotMatch 等，沿用 GravityLink 的映射）。

## 密钥安全

- 主密钥：`{DATA_DIR}/islet.key`，32 字节随机，0600，首次使用时创建。
- 策略 config 整体 AES-256-GCM 加密后入库（nonce 前置 base64）。
- API 永不回传 Secret；回传 `secret_configured` 布尔。空 Secret 保存时保留旧值；旧密文解不开时必须重新填写 Secret 才允许覆盖。
- 密钥文件与数据卷同生命周期：迁库不迁卷会导致解密失败，按上述规则补救。

## 命名规则（naming）

- `keep`：保留原始文件名（净化后），冲突时追加 `-xxxxxxxx` 随机后缀。
- `random`：`{16位随机hex}.{ext}`。
- `date`：`{yyyy/MM/dd}/{16位随机hex}.{ext}`。

key 全程校验：禁止 `..`、反斜杠、绝对路径；local 驱动写入前做目录逃逸检查。

## 上传决策链

1. 解析策略：请求显式指定 policy_id（须在分组白名单内），否则默认策略。
2. 分组校验：mime 白名单（支持 `image/*` 通配）→ 单文件上限 → 频率（Redis INCR 小时桶）。
3. 配额校验：`SUM(images.size) WHERE user_id=? AND deleted_at IS NULL` + 本次 ≤ quota_bytes。
4. 去重：`(user_id, sha256)` 已存在（未删除）→ 直接返回既有记录（PicGo 响应同）。
5. 写入：原图 + 缩略图（最长边 480px，不透明 JPEG q82 / 含透明 PNG）。
6. 落库保留发布地址；读取列表与上传结果按当前策略返回受控分发链接。已有图片的存储位置与已启用的加速域名受保护，仍有未完成同步任务时不能删除策略。

## OSS 条件写入
OSS 使用原生 PutObject 的 x-oss-forbid-overwrite 防覆盖请求；普通 S3 使用 If-None-Match。连接检查复用实际上传并回读校验。参考：https://www.alibabacloud.com/help/en/oss/developer-reference/putobject 。
