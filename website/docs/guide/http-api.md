# HTTP API 服务

内置 HTTP API Server（默认监听 `127.0.0.1:8096`），供外部程序 / 浏览器扩展调用。

```bash
ytb server start                 # 后台启动（默认 127.0.0.1:8096；--addr 可改）
ytb server status                # 运行状态与日志位置（data/server.log）
ytb server restart / stop
```

## 鉴权

- 本地回环访问无需鉴权；
- 对外监听必须设置 `YTB2BILI_SERVER_TOKEN`（Bearer 鉴权）；
- 配套浏览器扩展需配置 `YTB2BILI_ALLOWED_ORIGINS`（允许的跨域来源）。

```bash
curl -H "Authorization: Bearer $YTB2BILI_SERVER_TOKEN" \
     http://localhost:8096/api/v1/tasks
```

端点参考见仓库 `docs/API_DOCS.md`。浏览器扩展源码见 `extension/`（WXT/TypeScript，独立构建）。
