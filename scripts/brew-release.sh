#!/usr/bin/env bash
# 把私有主仓库的 release 产物镜像到公开 Homebrew tap 仓库，并更新 Formula
#
# 用法:  ./scripts/brew-release.sh v0.4.0
# 前置:  1) git tag v0.4.0 && git push origin v0.4.0 已触发 Release workflow
#        2) 本机 gh 已登录（gh auth login）
#
# 做了什么:
#   1. 从私有仓库 difyz9/ytb2bili-cli 下载 4 平台 tar.gz
#   2. 重打包为仅含 ytb 二进制（README/示例配置不随公开 tap 泄露）
#   3. 上传到公开仓库 difyz9/homebrew-tap 的同名 release
#   4. 生成 Formula/ytb.rb（含版本号+sha256）提交到 tap 仓库
#
# 之后用户即可: brew update && brew install difyz9/tap/ytb（或 upgrade）

set -euo pipefail

MAIN_REPO="difyz9/ytb2bili-cli"
TAP_REPO="difyz9/homebrew-tap"

TAG="${1:?用法: $0 v0.4.0}"
VERSION="${TAG#v}"

command -v gh >/dev/null 2>&1 || { echo "❌ 需要 gh CLI（brew install gh && gh auth login）"; exit 1; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 1. 从私有主仓库下载 4 平台产物
echo "⬇️  从 $MAIN_REPO 下载 v$VERSION 产物..."
for arch in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  gh release download "v$VERSION" -R "$MAIN_REPO" -p "ytb_${VERSION}_${arch}.tar.gz" -D "$TMP"
done

# 2. 重打包：只保留 ytb 二进制
echo "📦 重打包（仅二进制）..."
mkdir -p "$TMP/repacked"
for f in "$TMP"/ytb_*.tar.gz; do
  name="$(basename "$f")"
  work="$(mktemp -d)"
  tar -xzf "$f" -C "$work"
  tar -czf "$TMP/repacked/$name" -C "$work" ytb
  rm -rf "$work"
done

# 3. 计算 sha256
sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
S_DARWIN_ARM=$(sha256 "$TMP/repacked/ytb_${VERSION}_darwin_arm64.tar.gz")
S_DARWIN_INTEL=$(sha256 "$TMP/repacked/ytb_${VERSION}_darwin_amd64.tar.gz")
S_LINUX_ARM=$(sha256 "$TMP/repacked/ytb_${VERSION}_linux_arm64.tar.gz")
S_LINUX_INTEL=$(sha256 "$TMP/repacked/ytb_${VERSION}_linux_amd64.tar.gz")

# 4. 上传二进制到 tap 仓库 release（tag 不存在会自动创建）
echo "⬆️  上传到 $TAP_REPO release v$VERSION ..."
gh release create "v$VERSION" -R "$TAP_REPO" \
  --title "ytb v$VERSION" \
  --notes "Binary release mirrored from private repo $MAIN_REPO v$VERSION (binary only)" \
  --target main \
  "$TMP"/repacked/*.tar.gz 2>/dev/null \
  || gh release upload "v$VERSION" -R "$TAP_REPO" "$TMP"/repacked/*.tar.gz --clobber

# 5. 生成 Formula 并提交到 tap 仓库
echo "🧾 更新 Formula/ytb.rb ..."
cat > "$TMP/ytb.rb" <<EOF
class Ytb < Formula
  desc "YouTube to Bilibili video republishing pipeline CLI"
  homepage "https://github.com/difyz9/ytb2bili-cli"
  version "${VERSION}"

  on_macos do
    on_arm do
      url "https://github.com/${TAP_REPO}/releases/download/v${VERSION}/ytb_${VERSION}_darwin_arm64.tar.gz"
      sha256 "${S_DARWIN_ARM}"
    end

    on_intel do
      url "https://github.com/${TAP_REPO}/releases/download/v${VERSION}/ytb_${VERSION}_darwin_amd64.tar.gz"
      sha256 "${S_DARWIN_INTEL}"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/${TAP_REPO}/releases/download/v${VERSION}/ytb_${VERSION}_linux_arm64.tar.gz"
      sha256 "${S_LINUX_ARM}"
    end

    on_intel do
      url "https://github.com/${TAP_REPO}/releases/download/v${VERSION}/ytb_${VERSION}_linux_amd64.tar.gz"
      sha256 "${S_LINUX_INTEL}"
    end
  end

  def install
    bin.install "ytb"
  end

  test do
    assert_match version.to_s, shell_output("\#{bin}/ytb --version")
  end
end
EOF

B64=$(base64 -i "$TMP/ytb.rb" | tr -d '\n')
EXISTING_SHA=$(gh api "repos/${TAP_REPO}/contents/Formula/ytb.rb" -q '.sha' 2>/dev/null || true)
if [ -n "$EXISTING_SHA" ]; then
  gh api -X PUT "repos/${TAP_REPO}/contents/Formula/ytb.rb" \
    -f message="ytb ${VERSION}" -f branch=main -f content="$B64" -f sha="$EXISTING_SHA" -q '.commit.sha' >/dev/null
else
  gh api -X PUT "repos/${TAP_REPO}/contents/Formula/ytb.rb" \
    -f message="ytb ${VERSION}" -f branch=main -f content="$B64" -q '.commit.sha' >/dev/null
fi

echo ""
echo "✅ 完成！用户安装/升级命令："
echo "   brew update && brew install difyz9/tap/ytb    # 首次"
echo "   brew upgrade difyz9/tap/ytb                   # 已安装时升级"
