#!/bin/sh
set -eu

# .envはPiの実行設定用。このスクリプトではシェルの環境変数を使います。
image_name=${IMAGE_NAME:-ghcr.io/taro0079/go-llm-chef}
image_tag=${1:-latest}
platforms=${PLATFORMS:-linux/arm64}
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

if [ "$#" -gt 1 ]; then
    echo "Usage: $0 [tag]" >&2
    exit 1
fi

case "$image_name" in
    ghcr.io/*) ;;
    *) echo "IMAGE_NAME must start with ghcr.io/" >&2; exit 1 ;;
esac

printf 'Building and pushing %s:%s (%s)\n' "$image_name" "$image_tag" "$platforms"
exec docker buildx build \
    --platform "$platforms" \
    --tag "$image_name:$image_tag" \
    --push \
    "$project_dir"
