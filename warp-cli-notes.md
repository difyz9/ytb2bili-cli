# Cloudflare WARP CLI 使用笔记

## 一、常用命令速查

### 1.1 查看安装 & 版本
```bash
which warp-cli                        # 检查是否已安装
warp-cli --version                    # 查看版本
```

### 1.2 状态管理（最常用）
```bash
warp-cli status                       # 查看连接状态（短格式）
warp-cli --accept-tos status          # 查看连接状态（标准格式，推荐）

warp-cli connect                      # 连接 WARP
warp-cli disconnect                   # 断开 WARP
```

> **注意：** 如果 `warp-cli status` 不加 `--accept-tos` 提示需要接受服务条款，请使用完整的 `warp-cli --accept-tos status`。

### 1.3 注册/授权（首次使用）
```bash
warp-cli --accept-tos register
# 或使用授权码：warp-cli --accept-tos set-license <key>
```

## 二、Split Tunnel 分流配置

WARP 支持 Split Tunnel（分流），让特定域名或 IP 走直连（不通过 WARP），其余流量走 WARP。

### 2.1 添加域名排除

```bash
warp-cli --accept-tos add-excluded-host <域名>
```

**例如：** 排除 B站相关域名

```bash
warp-cli --accept-tos add-excluded-host bilivideo.com
warp-cli --accept-tos add-excluded-host bilibili.com
warp-cli --accept-tos add-excluded-host hdslb.com
```

### 2.2 添加 IP 路由排除

> **重要：域名排除在某些场景下可能不生效**，必须同时添加 IP 排除。

```bash
warp-cli --accept-tos add-excluded-route <IP/掩码>
```

**排查方法：** 用 `dig` 或 `curl --resolve` 解析域名获取 IP

```bash
# 解析 B站 CDN 域名 IP
dig +short upos-sz-mirrorali.bilivideo.com
dig +short api.bilibili.com

# 测试直连速度（断开 WARP 后）
curl -o /dev/null -s -w "速度: %{speed_download} bytes/s\n" "https://upos-sz-mirrorali.bilivideo.com/..."

# 测试 WARP 下速度（连接 WARP 后对比）
```

### 2.3 查看当前配置

WARP 配置保存在 `/var/lib/cloudflare-warp/settings.json`，包含 `excluded_ips` 和 `excluded_hosts` 字段。

```bash
cat /var/lib/cloudflare-warp/settings.json | python3 -m json.tool
```

**当前系统的 WARP 排除配置：**

**排除的域名：**
| 域名 | 说明 |
|------|------|
| `bilivideo.com` | B站视频CDN（上传/下载） |
| `bilibili.com` | B站主站/API服务器 |
| `hdslb.com` | B站图片CDN（封面等） |

**排除的 IP 路由：**
| IP 地址 | 说明 |
|---------|------|
| `61.147.237.41/32` | B站 CDN 节点 |
| `119.3.70.188/32` | B站 CDN 节点 |
| `139.159.241.37/32` | B站 CDN 节点 |
| `8.134.50.24/32` | B站 CDN 节点 |
| `47.103.24.173/32` | B站 CDN 节点 |
| `148.153.56.163/32` | B站 CDN 节点 |
| `148.153.64.18/32` | B站 CDN 节点 |
| `192.254.90.178/32` | B站 CDN 节点 |
| `192.254.90.179/32` | B站 CDN 节点 |
| `148.153.45.10/32` | B站 CDN 节点 |
| `148.153.46.90/32` | B站 CDN 节点 |
| `148.153.56.162/32` | B站 CDN 节点 |

## 三、实战案例：B站域名添加到 WARP 排除列表

### 问题背景
使用 `ytb2bili-go` 工具搬运 YouTube 视频到 B站时，上传速度极慢（8~10分钟，经常 500 错误）。原因是：
- **YouTube** 需要 WARP 才能访问
- **B站** 走 WARP 后 CDN 上传速度极差（B站 CDN 绕路到海外再回来）

### 解决思路
让 YouTube 走 WARP，B站走直连 → **Split Tunnel 分流**

### 完整操作步骤

#### 第1步：确认问题
```bash
# 连接 WARP 时测速 B站 CDN
warp-cli --accept-tos connect
curl -o /dev/null -s -w "%{speed_download}" "https://upos-sz-mirrorali.bilivideo.com/..."

# 断开 WARP 时测速
warp-cli --accept-tos disconnect
curl -o /dev/null -s -w "%{speed_download}" "https://upos-sz-mirrorali.bilivideo.com/..."
```
→ 结果：直连 18.6 MB/s，WARP 下 ~200 KB/s

#### 第2步：添加域名排除
```bash
warp-cli --accept-tos add-excluded-host bilivideo.com
warp-cli --accept-tos add-excluded-host bilibili.com
warp-cli --accept-tos add-excluded-host hdslb.com
```

#### 第3步：发现域名排除不生效，改加 IP 排除
测试后发现域名排除后，Go 语言程序上传仍然走 WARP。因为 Go 使用自己的 DNS 解析器（纯 Go 实现），可能绕过 WARP 的 DNS 拦截。

**改用 `dig` 解析 IP 后逐个添加 IP 路由排除：**

```bash
# 解析 B站 CDN 和 API 的 IP
dig +short upos-sz-mirrorali.bilivideo.com
dig +short api.bilibili.com

# 添加 IP 排除
warp-cli --accept-tos add-excluded-route <解析出的IP>/32
```

#### 第4步：验证效果
```bash
# 连接 WARP
warp-cli --accept-tos connect

# 测试 YouTube 连通性（走 WARP）
curl -I https://www.youtube.com  # ✅ 通

# 测试 B站 连通性（走直连）
curl -I https://api.bilibili.com  # ✅ 通

# 测试上传速度
cd /home/guan/guan/code/ytb2bili-go
./ytb submit "https://www.youtube.com/watch?v=..."
# 结果：12 秒上传完成（之前 8-10 分钟）
```

### 关键经验总结

| 经验 | 说明 |
|------|------|
| **域名排除不一定够** | Go 等语言的 DNS 解析器可能绕过 WARP 的 DNS 拦截，域名排除不一定会生效 |
| **IP 排除更可靠** | 解析出域名 IP 后用 `add-excluded-route` 添加路由排除，确保流量走直连 |
| **双管齐下** | 域名排除 + IP 排除都加上，最大兼容性 |
| **不要用 `--accept-tos` 的缩写** | WARP 新版本要求 `--accept-tos` 参数，否则命令会报错 |

## 四、故障排查

### 4.1 查看 WARP 状态
```bash
warp-cli --accept-tos status
```

### 4.2 查看排除规则
直接查看配置文件（`get-excluded-routes` 子命令在某些版本不可用）：
```bash
cat /var/lib/cloudflare-warp/settings.json | python3 -c "
import json, sys
data = json.load(sys.stdin)
# 域名排除
print('=== 域名排除 ===')
for host, note in data.get('excluded_hosts', []):
    print(f'  {host}  ({note})')
# IP 排除
print('=== IP 排除 ===')
for ip, note in data.get('excluded_ips', []):
    if note:  # 只显示手动添加的（忽略默认内网段）
        print(f'  {ip}  ({note})')
"
```

### 4.3 测试流量走向
```bash
# 测试直连（断开 WARP）
warp-cli --accept-tos disconnect
curl -I https://api.bilibili.com

# 测试 WARP 通道（连接 WARP）
warp-cli --accept-tos connect
curl -I https://www.youtube.com
```

### 4.4 WARP 无法连接
```bash
# 重启 WARP 服务
systemctl restart warp-svc

# 查看日志
journalctl -u warp-svc --no-pager -n 50
```

## 五、注意事项

1. **WARP 连接状态下重连**：先 `disconnect` 再 `connect`，不要重复 `connect`
2. **排除规则持久化**：重启后保留，无需重复添加
3. **`--accept-tos` 必须带**：WARP CLI 新版要求显式接受服务条款
4. **Split Tunnel 模式**：默认是 exclude-mode（排除模式），即默认走 WARP，排除的走直连
5. **Linux 桌面环境**：Debian 13 + GNOME 下测试通过，其他发行版可能路径不同
