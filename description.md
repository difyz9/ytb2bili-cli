## User request

项目已提交到 GitHub 仓库 (https://github.com/difyz9/ytb2bili-cli)。在项目路径下发现了一个名为 `${OUTPUT}` 的文件，这应该是一个 bug——构建脚本或 Makefile 中的 `OUTPUT` 变量没有正确配置/定义，导致 `${OUTPUT}` 被作为字面量文件名而不是变量展开。需要排查并修复该问题，确保构建产物输出到正确的路径。
