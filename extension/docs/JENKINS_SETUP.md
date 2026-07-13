# Jenkins 自动化构建配置指南

## 📋 前置要求

### 1. Jenkins 插件安装

在 Jenkins 中安装以下插件：
- Git Plugin
- Pipeline Plugin
- NodeJS Plugin (可选)
- Credentials Plugin

### 2. 配置 GitHub Personal Access Token

1. 访问 GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. 点击 "Generate new token (classic)"
3. 设置权限：
   - `repo` (完整仓库访问权限)
   - `workflow` (工作流权限)
4. 生成并复制 token

### 3. 在 Jenkins 中添加 GitHub Token

1. 进入 Jenkins → Manage Jenkins → Manage Credentials
2. 选择合适的域（通常是 Global）
3. 点击 "Add Credentials"
4. 配置如下：
   - **Kind**: Secret text
   - **Scope**: Global
   - **Secret**: 粘贴你的 GitHub Token
   - **ID**: `github-token`
   - **Description**: GitHub Personal Access Token

## 🔧 配置 Jenkins Job

### 方式一：创建 Pipeline Job

1. 在 Jenkins 首页点击 "New Item"
2. 输入任务名称（如 `ytb2bili-extension-Release`）
3. 选择 "Pipeline" 类型
4. 点击 "OK"

### 配置 Pipeline

1. **General 设置**
   - 勾选 "This project is parameterized"（Jenkinsfile 已包含参数定义）
   - 可选：勾选 "GitHub project"，填入项目 URL

2. **Build Triggers**
   - 可以配置 GitHub webhook 自动触发
   - 或定时构建
   - 或手动触发

3. **Pipeline 配置**
   - **Definition**: Pipeline script from SCM
   - **SCM**: Git
   - **Repository URL**: 你的 GitHub 仓库地址
   - **Credentials**: 选择你的 Git 凭据
   - **Branches to build**: `*/main` 或 `*/master`
   - **Script Path**: `Jenkinsfile`

4. 保存配置

## 🚀 运行构建

### 手动触发构建

1. 点击任务名称进入详情页
2. 点击 "Build with Parameters"
3. 配置参数：
   - **GIT_TAG**: 要构建的 Git 标签（如 `v1.0.1`），留空则使用最新标签
   - **CREATE_RELEASE**: 是否创建 GitHub Release（默认：true）
   - **PRERELEASE**: 是否标记为预发布版本（默认：false）
4. 点击 "Build"

### 自动触发构建（推送 Tag 时）

配置 GitHub Webhook：

1. 进入 GitHub 仓库 Settings → Webhooks
2. 点击 "Add webhook"
3. 配置：
   - **Payload URL**: `http://your-jenkins-url/github-webhook/`
   - **Content type**: application/json
   - **Which events**: Just the push event (或选择 "Let me select" 并勾选 "Pushes" 和 "Tags")
4. 保存

然后在 Jenkinsfile 中添加触发器：

```groovy
triggers {
    githubPush()
}
```

## 📝 Jenkinsfile 说明

### 主要阶段

1. **Checkout**: 检出代码，获取 Git 标签
2. **Setup Node.js**: 设置 Node.js 环境
3. **Update Version**: 从 Git 标签更新版本号到 package.json 和 wxt.config.ts
4. **Install Dependencies**: 安装项目依赖
5. **Build Extensions**: 并行构建 Chrome 和 Firefox 扩展
6. **Create Zip Files**: 创建分发包
7. **Archive Artifacts**: 归档构建产物到 Jenkins
8. **Create GitHub Release**: 创建 GitHub Release 并上传文件

### 环境变量

需要在 Jenkinsfile 中修改：

```groovy
environment {
    GITHUB_REPO = 'your-username/ytb2bili-extension'  // 修改为你的仓库
    GITHUB_TOKEN = credentials('github-token')
}
```

## 🔍 常见问题

### 1. GitHub CLI 未安装

Jenkinsfile 会自动尝试安装 GitHub CLI，但如果失败，你需要手动在 Jenkins 节点上安装：

**macOS:**
```bash
brew install gh
```

**Linux (Debian/Ubuntu):**
```bash
type -p curl >/dev/null || sudo apt install curl -y
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
sudo chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
sudo apt update
sudo apt install gh -y
```

### 2. 权限问题

确保 Jenkins 用户有权限：
- 访问 Git 仓库
- 写入工作目录
- 执行 shell 命令

### 3. Node.js 版本

如果系统 Node.js 版本不符合要求，可以：
- 使用 NodeJS Plugin 配置特定版本
- 或使用 nvm 管理 Node.js 版本

### 4. Release 已存在

如果尝试创建已存在的 Release，会失败。可以：
- 删除已存在的 Release
- 使用不同的标签
- 修改 Jenkinsfile 添加 `--clobber` 选项

## 🎯 高级配置

### 使用环境特定的配置

可以为不同环境创建不同的参数：

```groovy
parameters {
    choice(name: 'ENVIRONMENT', choices: ['production', 'staging', 'development'], description: 'Build environment')
}
```

### 添加测试阶段

在 Build Extensions 之前添加：

```groovy
stage('Run Tests') {
    steps {
        script {
            sh "yarn test"
        }
    }
}
```

### 通知配置

在 `post` 块中添加邮件或 Slack 通知：

```groovy
post {
    success {
        mail to: 'team@example.com',
             subject: "Build Success: ${env.BUILD_TAG}",
             body: "Build completed successfully!"
    }
}
```

## 📚 参考资源

- [Jenkins Pipeline 文档](https://www.jenkins.io/doc/book/pipeline/)
- [GitHub CLI 文档](https://cli.github.com/manual/)
- [WXT 文档](https://wxt.dev/)
