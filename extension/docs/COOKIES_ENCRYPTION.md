# Cookies 加密与后端解密说明

## 目的

扩展在提交视频信息到后端时，会附带当前页面可读取到的 cookies。由于 cookies 属于敏感信息，前端不会明文传输，而是先加密，再放入请求体中的 `meta` 字段。

本文档说明三件事：

1. 前端实际传了什么
2. 前端如何加密
3. 后端如何正确解密和使用

## 数据流概览

请求入口在 [utils/api.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/api.ts#L76)，cookies 获取逻辑在 [entrypoints/background.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/entrypoints/background.ts#L62)，加密实现位于 [utils/crypto.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/crypto.ts#L52)。

整体流程如下：

1. 前端通过 `browser.cookies.getAll` 获取当前视频页面对应的 cookies。
2. cookies 会先被序列化成 JSON 字符串，而不是直接转成 `Cookie: a=1; b=2` 这种 header 格式。
3. 前端使用 AES-256-GCM 加密该 JSON 字符串。
4. 前端把 `IV + 密文` 拼接后做 Base64 编码。
5. 最终将 Base64 字符串放进提交接口 `/api/v1/submit` 请求体中的 `meta` 字段。

示意：

```text
Cookie[]
  -> JSON.stringify(cookies)
  -> AES-256-GCM encrypt
  -> prepend IV(12 bytes)
  -> Base64
  -> request.body.meta
```

## 前端传输格式

提交体中的关键字段如下：

```json
{
  "url": "https://www.youtube.com/watch?v=xxx",
  "title": "video title",
  "description": "",
  "operationType": "manual",
  "subtitles": [],
  "playlistId": "",
  "timestamp": "2026-03-16T00:00:00.000Z",
  "savedAt": "2026-03-16T00:00:00.000Z",
  "meta": "Base64(IV + ciphertext)"
}
```

其中：

1. `meta` 为空字符串时，表示这次没有成功获取到 cookies，后端应允许为空。
2. `meta` 不为空时，内容是 Base64 字符串，解码后前 12 个字节是 IV，后续字节是 AES-GCM 的密文数据。
3. Web Crypto 的 AES-GCM 输出已经包含认证标签，因此后端只需要把 `ciphertextAndTag` 作为一个整体传给 GCM 解密即可。

## 前端加密细节

### 1. 原始明文

前端不是传 cookie header，而是传 cookies 数组的 JSON 字符串。

cookies 原始结构来自 [utils/cookies.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/cookies.ts#L6)，单项大致如下：

```json
[
  {
    "name": "SESSDATA",
    "value": "xxxx",
    "domain": ".bilibili.com",
    "path": "/",
    "secure": true,
    "httpOnly": true,
    "sameSite": "no_restriction",
    "expirationDate": 1770000000
  }
]
```

### 2. 算法参数

前端使用 Web Crypto API，参数固定如下：

1. 算法：`AES-GCM`
2. 密钥长度：`256 bit`
3. IV 长度：`12 bytes`
4. 输出编码：`Base64`

实现参考 [utils/crypto.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/crypto.ts#L40)。

### 3. 密钥处理规则

前端不会直接使用原始字符串做 AES key，而是先做一遍标准化：

```ts
const keyData = new TextEncoder().encode(keyStr.padEnd(32, '0').slice(0, 32));
```

也就是说：

1. 如果密钥长度小于 32 个字符，则右侧补 `0` 到 32 位。
2. 如果密钥长度大于 32 个字符，则只取前 32 位。
3. 最终使用 UTF-8 字节作为 32 字节 AES key。

当前前端默认配置位于 [utils/config.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/config.ts#L24)：

```ts
COOKIES_ENCRYPT_KEY: import.meta.env.VITE_COOKIES_ENCRYPT_KEY || '59e7052041ce4bd6aff82f6a0bca9cde'
```

如果后端也使用环境变量读取密钥，必须保证和前端使用的是同一份原始字符串，并做相同的补齐/截断逻辑。

### 4. 最终密文格式

前端输出并不是单独返回密文，而是：

```text
Base64( IV[12] + encryptedPayload )
```

其中：

1. `IV[12]` 是随机生成的 12 字节 nonce。
2. `encryptedPayload` 是 Web Crypto 的 AES-GCM 输出，内部已包含 auth tag。

## 后端解密步骤

后端解密时必须按下面顺序处理：

1. 读取请求体 `meta` 字段。
2. 对 `meta` 做 Base64 解码。
3. 取前 12 字节作为 `iv`。
4. 剩余字节作为 `ciphertextAndTag`。
5. 按和前端一致的规则构造 32 字节 key。
6. 使用 AES-256-GCM 解密。
7. 得到明文 JSON 字符串后，再反序列化为 cookies 数组。

## Go 解密示例

如果后端是 Go，建议直接按下面实现。

```go
package cookies

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type BrowserCookie struct {
	Name           string   `json:"name"`
	Value          string   `json:"value"`
	Domain         string   `json:"domain,omitempty"`
	Path           string   `json:"path,omitempty"`
	Secure         bool     `json:"secure,omitempty"`
	HTTPOnly       bool     `json:"httpOnly,omitempty"`
	SameSite       string   `json:"sameSite,omitempty"`
	ExpirationDate *float64 `json:"expirationDate,omitempty"`
	StoreID        string   `json:"storeId,omitempty"`
}

func normalizeKey(raw string) []byte {
	if len(raw) < 32 {
		for len(raw) < 32 {
			raw += "0"
		}
	}
	if len(raw) > 32 {
		raw = raw[:32]
	}
	return []byte(raw)
}

func DecryptCookies(meta string, rawKey string) ([]BrowserCookie, error) {
	if meta == "" {
		return nil, nil
	}

	combined, err := base64.StdEncoding.DecodeString(meta)
	if err != nil {
		return nil, fmt.Errorf("base64 decode meta: %w", err)
	}

	if len(combined) < 13 {
		return nil, fmt.Errorf("invalid encrypted payload length: %d", len(combined))
	}

	iv := combined[:12]
	ciphertext := combined[12:]

	block, err := aes.NewCipher(normalizeKey(rawKey))
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	plain, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt cookies meta: %w", err)
	}

	var cookies []BrowserCookie
	if err := json.Unmarshal(plain, &cookies); err != nil {
		return nil, fmt.Errorf("unmarshal cookies json: %w", err)
	}

	return cookies, nil
}
```

### 转回 Cookie Header

如果后端后续要拿这些 cookies 去请求第三方站点，可以再拼成 header：

```go
package cookies

import "strings"

func ToHeader(cookies []BrowserCookie) string {
	parts := make([]string, 0, len(cookies))
	for _, item := range cookies {
		if item.Name == "" {
			continue
		}
		parts = append(parts, item.Name+"="+item.Value)
	}
	return strings.Join(parts, "; ")
}
```

生成结果类似：

```text
SESSDATA=xxx; bili_jct=yyy; DedeUserID=zzz
```

## Node.js 解密示例

如果后端不是 Go，也可以参考下面的 Node.js 实现：

```ts
function normalizeKey(keyStr: string): Buffer {
  return Buffer.from(keyStr.padEnd(32, '0').slice(0, 32), 'utf8');
}

export function decryptCookies(meta: string, keyStr: string) {
  if (!meta) return [];

  const combined = Buffer.from(meta, 'base64');
  const iv = combined.subarray(0, 12);
  const encrypted = combined.subarray(12);

  const authTag = encrypted.subarray(encrypted.length - 16);
  const ciphertext = encrypted.subarray(0, encrypted.length - 16);

  const decipher = crypto.createDecipheriv('aes-256-gcm', normalizeKey(keyStr), iv);
  decipher.setAuthTag(authTag);

  const plain = Buffer.concat([
    decipher.update(ciphertext),
    decipher.final(),
  ]);

  return JSON.parse(plain.toString('utf8'));
}
```

说明：Node.js `crypto` 需要手动拆出最后 16 字节的 GCM `authTag`；而 Go 的 `cipher.AEAD` 直接接受 `ciphertext || tag`，不需要额外拆分。

## 后端接入建议

1. `meta` 字段按可选字段处理，不要要求必填。
2. 解密失败时，建议记录错误日志，但不要把原始 `meta` 明文输出到日志。
3. 如果后端只关心请求第三方所需的 Cookie header，可以在解密后立即转换成 `name=value; name2=value2`。
4. 如果后端需要更精细的 cookie 管理，保留结构化数组更合适，因为其中包含 `domain`、`path`、`httpOnly`、`secure` 等信息。
5. 前后端需要共用同一个 `COOKIES_ENCRYPT_KEY` 原始值，否则一定解不开。

## 常见问题

### 1. 为什么解密失败？

优先检查下面几项：

1. 前后端密钥原始字符串是否一致。
2. 后端是否也做了 `padEnd(32, '0').slice(0, 32)`。
3. 后端是否正确把前 12 字节当成 IV。
4. 是否把 Base64 解码后的全部剩余内容都交给 GCM 解密。
5. `meta` 字段是否在传输或落库过程中被裁剪、转义或二次编码。

### 2. 为什么不直接传 Cookie header？

当前实现选择传结构化 JSON，有两个好处：

1. 后端可以保留更完整的 cookie 元数据。
2. 后端可以按需要自行转换成 header、对象或持久化格式。

### 3. 如何验证前后端逻辑一致？

可以用同一个测试字符串做联调：

1. 前端本地调用 `encryptData(JSON.stringify(cookies), key)`。
2. 把得到的 `meta` 复制到后端测试接口。
3. 后端解密后比对 JSON 字符串或反序列化结果是否一致。

## 对应源码位置

1. 加密入口：[utils/api.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/api.ts#L94)
2. cookies 获取：[entrypoints/background.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/entrypoints/background.ts#L62)
3. cookies 结构：[utils/cookies.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/cookies.ts#L6)
4. 加密实现：[utils/crypto.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/crypto.ts#L52)
5. 密钥配置：[utils/config.ts](/Users/apple/opt/difyz_202603/1123/ytb2bili_extension/utils/config.ts#L24)