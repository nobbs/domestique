#!/bin/sh
# Point a linked worktree's .cache at the main checkout's, so every worktree
# shares one Go build cache instead of growing its own multi-gigabyte copy.
# .mise.toml sets GOCACHE to {{config_root}}/.cache, which CI needs kept
# repo-relative for actions/cache; the symlink redirects the bytes and leaves
# that literal path alone.
set -eu

main=$(cd "$(git rev-parse --git-common-dir)/.." && pwd)

# The main checkout owns the real directory; a worktree already linked is done.
if [ "$main" = "$PWD" ] || [ -L .cache ]; then
	exit 0
fi

mkdir -p "$main/.cache"
rm -rf .cache
ln -s "$main/.cache" .cache
