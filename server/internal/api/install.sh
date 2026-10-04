#!/usr/bin/env sh
# extbrain 一键安装：CLI + Skill（从 my-extbrain 服务直接分发）
# 用法：curl -fsSL <你的实例>/install.sh | sh
set -eu

BASE="${EXTBRAIN_BASE:-__BASE__}"
BIN_DIR="${EXTBRAIN_BIN_DIR:-}"
SKILL_DIR="${EXTBRAIN_SKILL_DIR:-$HOME/.agents/skills}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')   # darwin / linux
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "✗ 不支持的架构: $arch" >&2; exit 1 ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "==> 下载 extbrain CLI（${os}_${arch}）"
CLI_URL="$BASE/downloads/extbrain_${os}_${arch}.tar.gz"
if ! curl -fSL --progress-bar -o "$TMP/cli.tar.gz" "$CLI_URL"; then
  echo "✗ 下载失败：$CLI_URL" >&2
  echo "  （如果这是自部署实例，请确认服务器 downloads/ 目录已放入发行包）" >&2
  exit 1
fi
tar -xzf "$TMP/cli.tar.gz" -C "$TMP" extbrain

# 安装位置：显式指定 > /usr/local/bin（可写时）> ~/.local/bin
if [ -z "$BIN_DIR" ]; then
  if [ -w /usr/local/bin ] 2>/dev/null; then
    BIN_DIR=/usr/local/bin
  else
    BIN_DIR="$HOME/.local/bin"
  fi
fi
mkdir -p "$BIN_DIR"
mv "$TMP/extbrain" "$BIN_DIR/extbrain"
chmod +x "$BIN_DIR/extbrain"
echo "==> CLI 已装到 $BIN_DIR/extbrain"
"$BIN_DIR/extbrain" --version || true

echo "==> 安装 Skill（默认装到 ${SKILL_DIR}，供 Claude Code / ZCode 等 AI 使用）"
mkdir -p "$SKILL_DIR"
if curl -fsSL -o "$TMP/skill.tar.gz" "$BASE/downloads/extbrain-skill.tar.gz"; then
  rm -rf "$SKILL_DIR/extbrain"
  mkdir -p "$SKILL_DIR/extbrain"
  tar -xzf "$TMP/skill.tar.gz" -C "$SKILL_DIR/extbrain"
  echo "==> Skill 已装到 $SKILL_DIR/extbrain"
  # 操作手册随 Skill 一起放到本地：地址已由服务端注入，可直接整份扔给任何 AI
  if curl -fsSL -o "$SKILL_DIR/extbrain/操作手册.md" "$BASE/guide.md"; then
    echo "==> 操作手册已装到 $SKILL_DIR/extbrain/操作手册.md"
  else
    echo "⚠ 操作手册下载失败，可访问 $BASE/guide 在线查看"
  fi
else
  echo "⚠ Skill 包下载失败（CLI 已可用），稍后可重试"
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "⚠ 请把 $BIN_DIR 加入 PATH：export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

echo ""
echo "✓ 完成。最后一步："
echo "    extbrain auth login --server $BASE   （会打开浏览器，点一下「授权」即完成，无需手抄密钥）"
echo "    然后对你的 AI 说：「帮我记一下……」「查查我收藏的……」"
echo "    操作手册已备好（整份扔给任何 AI 即用）：$SKILL_DIR/extbrain/操作手册.md"
