# PicList 接入

在“上传密钥”选择“连接 PicList”。PicList 使用内置的“高级自定义”图床（advancedplist），上传仍进入 Islet 统一的认证、配额、格式检查与审核流程。

## 填写配置

| PicList 字段 | 内容 |
| --- | --- |
| 请求地址 endpoint | 站点上传地址 `/api/v1/upload` |
| 请求方法 method | POST |
| 文件字段 formDataKey | files |
| 请求头 headers | JSON：Authorization 为 Bearer、一个空格和本人上传密钥 |
| 请求参数 body | 默认 `{}`；可选择本人有权使用的存储策略和相册 |
| 返回链接路径 resDataPath | data.items.0.url |
| 脚本、链接前缀和网站路径 | 留空 |

上传返回的地址使用站点配置的图片分发入口。接口失败时返回错误信息，配置不将错误文本当成图片地址。撤销或到期的密钥不能继续上传，上传密钥不能读取管理配置或执行审核。

`GET /api/v1/client-configs/piclist` 向已登录的交互式账户返回不含真实密钥的配置模板。请求头使用明显的密钥占位符。工作台在浏览器内填入新建或本人粘贴的密钥，并提供逐项复制与供 PicList Core 使用的配置片段复制；不保存到浏览器持久存储，不把密钥放入 URL，不回传给模板接口。

新密钥只在创建结果中出现一次；可直接进入 PicList 配置。配置结束后关闭弹层会清除当前填写的密钥。已有密钥不能从服务端再次读取。

## 验证范围

已使用官方 PicList-Core 3.0.2 的内置 advancedplist 上传器执行真实上传，核对本人归属、指定相册、返回链接和原图字节；撤销密钥后，同一客户端上传被拒绝。页面验收覆盖配置复制、缺少密钥提示、窄屏、深浅色与键盘关闭。

参考：[PicList 配置说明](https://piclist.cn/configure)、[高级自定义上传](https://piclist.cn/providers/services#高级自定义上传)、[官方上传器实现](https://github.com/Kuingsmile/PicList-Core/blob/v3.0.2/src/plugins/uploader/advancedplist.ts)。
