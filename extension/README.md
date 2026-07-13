# Bili UP Extension

<div align="center">

**一个现代化的浏览器扩展，用于保存 YouTube 和 Bilibili 视频 URL 及字幕**

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![WXT](https://img.shields.io/badge/WXT-0.20.11-orange.svg)](https://wxt.dev/)
[![React](https://img.shields.io/badge/React-19-61dafb.svg)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.9-blue.svg)](https://www.typescriptlang.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-3.0-38bdf8.svg)](https://tailwindcss.com/)

</div>

## ✨ 特性

- 🎯 **一键保存** - 快速保存视频 URL 和字幕
- ⌨️ **快捷键支持** - Ctrl/Cmd + Shift + S/T 快速操作
- 🎨 **现代化 UI** - 使用 React + Tailwind CSS 构建的美观界面
- 📦 **本地存储** - 自动保存到浏览器本地，随时查看
- 🌐 **视频提交同步** - 登录后将数据提交到可配置的视频接口
- 🔄 **实时字幕获取** - 自动获取 YouTube 视频字幕
- 🎬 **双平台支持** - YouTube 和 Bilibili 双平台兼容
- 🛡️ **类型安全** - 完整的 TypeScript 类型支持
- 🔐 **账号登录** - 支持会员服务登录与注册
- 🧩 **本地 API 封装** - 插件内直接封装认证、会员与视频提交接口，不依赖外部 SDK 包
- ⚙️ **DeepSeek 集成** - 配置 DeepSeek API 密钥进行 AI 翻译
- 🌐 **Web 管理** - 通过外部 Web 页面管理视频和查看详情

## 🚀 快速开始

## 登录要求

- 插件必须先通过会员服务的登录或注册接口完成账号认证，登录后才可以提交视频。
- 默认视频提交根地址由 `VITE_BACKEND_URL` 提供，用户登录后也可以在弹窗里修改该根地址。
- 认证接口固定走 `VITE_MEMBERSHIP_URL` 下的 `/auth/login`、`/auth/register`、`/auth/me`，视频提交时会基于用户配置的根地址自动拼接 `/api/v1/submit`。
- 插件在 `utils/api.ts` 中本地封装这些接口调用，不要求额外安装独立 SDK。

### 安装依赖

```bash
# 使用 yarn（推荐）
yarn install

# 或使用 npm
npm install
```

### 开发模式

```bash
# Chrome/Edge 开发版本
yarn dev

# Firefox 开发版本
yarn dev:firefox
```

开发服务器将在 `http://localhost:3000` 启动，扩展构建文件在 `dist/` 目录。

### 生产构建

```bash
# Chrome/Edge 生产版本
yarn build

# Firefox 生产版本
yarn build:firefox
```

### 打包发布

```bash
# 打包为 zip 文件
yarn zip

# Firefox 版本
yarn zip:firefox
```

## 📦 安装到浏览器

### Chrome / Edge

1. 打开浏览器扩展管理页面：
   - Chrome: `chrome://extensions/`
   - Edge: `edge://extensions/`

2. 启用右上角的 **"开发者模式"**

3. 点击 **"加载已解压的扩展程序"**

4. 选择项目的 `dist/chrome-mv3` 目录

5. 扩展安装完成！图标会出现在浏览器工具栏

### Firefox

1. 打开 `about:debugging#/runtime/this-firefox`

2. 点击 **"临时载入附加组件"**

3. 选择 `dist/firefox-mv2/manifest.json` 文件

4. 扩展安装完成！

## 📚 文档

- [后端服务地址配置](docs/BACKEND_CONFIG.md)
- [Cookies 加密与后端解密说明](docs/COOKIES_ENCRYPTION.md)

## 🎮 使用指南

### 基本操作

#### 1. 保存视频 URL

**方法一：使用浮动按钮**
1. 访问 YouTube 或 Bilibili 视频页面
2. 点击页面右下角的蓝色浮动按钮
3. 选择 **"保存当前 URL"**
4. 如果尚未登录，插件会提示先登录或注册账号
5. 登录后看到成功通知即表示已提交到 后端 API

**方法二：使用快捷键**
- 按下 `Ctrl + Shift + S` (Windows/Linux)
- 或 `Cmd + Shift + S` (macOS)

#### 2. 保存视频字幕

**方法一：使用浮动按钮**
1. 访问有字幕的 YouTube 视频
2. 点击页面右下角的浮动按钮
3. 选择 **"保存字幕"**
4. 字幕会自动下载为 `.srt` 文件

**方法二：使用快捷键**
- 按下 `Ctrl + Shift + T` (Windows/Linux)
- 或 `Cmd + Shift + T` (macOS)

#### 3. 管理保存的内容

1. 点击浏览器工具栏中的扩展图标
2. 在弹出的窗口中切换标签页：
   - **"保存的 URL"** - 查看所有保存的视频链接
   - **"保存的字幕"** - 查看所有保存的字幕
3. 可以执行的操作：
   - 📥 下载字幕文件
   - 🔗 打开原始视频链接
   - 🗑️ 删除单个项目
   - 🧹 清空所有数据

#### 4. 管理视频和查看详情

1. 在扩展 Popup 中，点击标题下方的 **"扩展设置"** 链接
2. 自动在新标签页打开 Web 管理页面（默认 `https://stage.api2key.com`）
3. 在 Web 页面中可以：
   - 📊 查看所有已提交视频的列表
   - 🔍 查看视频详细信息和处理进度
   - 📝 查看任务步骤和执行状态
   - 📁 下载相关文件（视频、字幕、封面等）
   - 🔄 重试失败的处理步骤
   - ⚙️ 管理更多高级设置

#### 5. 设置和配置

1. 右键点击扩展图标，选择 **"选项"**（或从扩展管理页面进入）
2. 在设置页面中，您可以：

**常规设置标签：**
- 配置 **DeepSeek API 密钥** - 用于 AI 翻译功能
- 设置 **后端服务器地址** - 默认为 `https://stage.api2key.com`
- 启用/禁用 **自动保存字幕** 功能

**账户设置标签：**
- 使用 **邮箱密码登录 / 注册** 功能
- 查看当前登录状态和用户信息
- 退出登录

**关于标签：**
- 查看扩展版本和信息
- 访问项目主页和文档

## 🛠️ 技术栈

- **[WXT](https://wxt.dev/)** - 下一代 Web 扩展开发框架
- **[React 19](https://react.dev/)** - 用户界面库
- **[TypeScript 5.9](https://www.typescriptlang.org/)** - 类型安全的 JavaScript
- **[Tailwind CSS 3](https://tailwindcss.com/)** - 实用优先的 CSS 框架
- **[Vite 7](https://vitejs.dev/)** - 极速构建工具

## 📁 项目结构

```
ytb2bili-extension-wxt/
├── components/              # React 组件
│   ├── ActionSelector.tsx   # 浮动操作按钮
│   ├── Notification.tsx     # 通知组件
│   ├── AuthForm.tsx         # 登录 / 注册组件
│   ├── StatusBadge.tsx      # 状态徽章组件
│   └── VideoList.tsx        # 视频列表组件
├── entrypoints/            # WXT 入口点
│   ├── background.ts       # 后台脚本（消息处理、数据存储）
│   ├── content.ts          # 内容脚本（页面注入、用户交互）
│   ├── content-styles.css  # 内容脚本样式
│   ├── popup/              # Popup 页面
│   │   ├── App.tsx         # Popup 主组件（含视频列表）
│   │   ├── index.html      # Popup HTML
│   │   ├── main.tsx        # Popup 入口
│   │   └── style.css       # Popup 样式
│   ├── options/            # 设置页面
│   │   ├── App.tsx         # 设置主组件
│   │   ├── index.html      # 设置页面 HTML
│   │   ├── main.tsx        # 设置页面入口
│   │   └── style.css       # 设置页面样式
│   └── video-detail/       # 视频详情页面
│       ├── App.tsx         # 视频详情主组件
│       ├── index.html      # 详情页面 HTML
│       ├── main.tsx        # 详情页面入口
│       └── style.css       # 详情页面样式
├── types/                  # TypeScript 类型定义
│   └── index.ts
├── utils/                  # 工具函数
│   ├── api.ts              # 后端 API 通信
│   ├── helpers.ts          # 通用工具函数
│   └── youtube-transcript.ts  # YouTube 字幕获取
├── public/                 # 静态资源
│   ├── page-interceptor.js # 页面拦截器（pot 参数捕获）
│   └── icon/               # 扩展图标
├── wxt.config.ts          # WXT 配置文件
├── tailwind.config.js     # Tailwind CSS 配置
├── postcss.config.js      # PostCSS 配置
└── .wxt/tsconfig.json      # WXT 生成的 TypeScript 配置
```

## ⚙️ 配置

### 设置页面配置

推荐使用设置页面进行配置：

1. 右键点击扩展图标 → 选择 **"选项"**
2. 在 **"常规设置"** 标签中配置：
   - **DeepSeek API 密钥**：用于 AI 翻译功能
   - **后端服务器地址**：数据提交的目标地址（默认：`https://stage.api2key.com`）
   - **自动保存字幕**：启用后自动保存获取的字幕
3. 在 **"账户设置"** 标签中：
   - 使用邮箱密码登录或注册
   - 查看登录状态和用户信息

### 代码级配置（高级）

如需直接修改代码配置，编辑 `entrypoints/background.ts`：

```typescript
// 第 6 行
const BACKEND_URL = 'http://localhost:8096/api/v1/submit';
```

将 URL 改为你的后端服务器地址。

### 后端 API 格式

扩展会向后端发送以下格式的 POST 请求：

```json
{
  "url": "https://www.youtube.com/watch?v=xxxxx",
  "title": "视频标题",
  "description": "视频描述",
  "operationType": "translation",  // "translation" 或 "download"
  "status": "pending",
  "playlistId": "playlist_001",
  "subtitles": [
    {
      "text": "字幕文本内容",
      "duration": 3.5,
      "offset": 0.0,
      "lang": "en"
    }
  ],
  "timestamp": "2025-10-13T12:00:00.000Z"
}
```

## 🔧 开发指南

### 环境要求

- Node.js >= 16
- Yarn 或 npm

### 开发流程

1. **克隆项目**
   ```bash
   git clone <repository-url>
   cd ytb2bili-extension-wxt
   ```

2. **安装依赖**
   ```bash
   yarn install
   ```

3. **启动开发服务器**
   ```bash
   yarn dev
   ```

4. **修改代码**
   - 修改后自动热重载
   - 查看终端输出的构建信息

5. **类型检查**
   ```bash
   yarn compile
   ```

### 添加新功能

#### 添加新的内容脚本功能

1. 在 `entrypoints/content.ts` 中添加功能代码
2. 如需 UI，在 `components/` 创建新组件
3. 更新 `types/index.ts` 添加类型定义

#### 添加新的后台功能

1. 在 `entrypoints/background.ts` 中添加消息处理
2. 更新 `types/index.ts` 中的 `MessageType` 枚举
3. 在 content script 中发送对应消息

## 🐛 故障排除

### 构建失败

```bash
# 清理并重新安装
rm -rf node_modules .wxt dist
yarn install
yarn dev
```

### 字幕无法获取

- ✅ 确认视频有可用字幕或自动字幕
- ✅ 检查浏览器控制台是否有错误
- ✅ 尝试刷新页面重新加载扩展

### 后端提交失败

- ✅ 确认后端服务器正在运行
- ✅ 检查 `BACKEND_URL` 配置是否正确
- ✅ 查看浏览器控制台的网络请求日志
- ✅ 确认后端 API 接口格式匹配

### 扩展无法加载

- ✅ 确认已启用开发者模式
- ✅ 检查 `dist/` 目录是否存在
- ✅ 尝试重新构建：`yarn build`
- ✅ 查看浏览器扩展管理页面的错误信息

## 📝 核心功能说明

### pot 参数拦截

扩展使用双重机制捕获 YouTube 字幕请求中的 `pot` 参数：

1. **页面拦截器** (`public/page-interceptor.js`)
   - 在页面上下文中拦截 fetch 和 XHR 请求
   - 通过 postMessage 将参数传递给 content script

2. **webRequest API** (`entrypoints/background.ts`)
   - 在后台拦截网络请求
   - 直接从 URL 中提取 pot 参数

### 字幕获取流程

1. 用户触发保存字幕操作
2. content script 注入页面拦截器
3. 拦截器捕获 pot 参数并缓存
4. 使用 pot 参数请求字幕 API
5. 解析 JSON3 或 XML 格式字幕
6. 转换为 SRT 格式并下载
7. 同时保存到本地存储和后端服务器

### 本地存储结构

使用 Chrome Storage API 存储数据：

```typescript
// 保存的 URL
{
  savedUrls: [
    {
      url: string,
      title: string,
      timestamp: number
    }
  ]
}

// 保存的字幕
{
  savedSubtitles: [
    {
      url: string,
      title: string,
      content: string,  // SRT 格式文本
      timestamp: number
    }
  ]
}
```

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- [WXT](https://wxt.dev/) - 优秀的扩展开发框架
- [React](https://react.dev/) - 强大的 UI 库
- [Tailwind CSS](https://tailwindcss.com/) - 实用的 CSS 框架

## 📧 联系方式

如有问题或建议，请提交 Issue 或联系维护者。

---

<div align="center">
Made with ❤️ using WXT + React + TypeScript
</div>

