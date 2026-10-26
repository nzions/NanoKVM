#!/usr/bin/env bash
# Runs once inside the dev container after it is created (see
# .devcontainer/devcontainer.json). Keeps the container aligned with the
# CLI flow in the Makefile and scripts/build-in-container.sh.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"


# Frontend dependencies. postCreate has no TTY, and CI=true lets pnpm make
# its non-interactive choices -- e.g. purging a node_modules tree that was
# installed from the host OS through the bind mount.
(cd "$repo_root/web" && CI=true pnpm install --frozen-lockfile)


# The build user's home has no shell skeleton files (useradd ran with
# --no-create-home), so login shells would skip ~/.bashrc; give them the
# standard Debian chain.
if [ ! -f "$HOME/.profile" ]; then
    cat >"$HOME/.profile" <<'EOF'
# ~/.profile (added by .devcontainer/post-create.sh)
if [ -n "$BASH_VERSION" ] && [ -f "$HOME/.bashrc" ]; then
    . "$HOME/.bashrc"
fi
EOF
fi
