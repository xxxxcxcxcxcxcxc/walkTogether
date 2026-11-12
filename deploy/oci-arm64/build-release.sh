#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
build_dir="$repo_root/build/oci-arm64"
mkdir -p "$build_dir"

cd "$repo_root/frontend"
npm ci
npm run build

cd "$repo_root/backend"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$build_dir/walktogether-api" ./cmd/api
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$build_dir/walktogether-bot" ./cmd/bot
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$build_dir/walktogether-worker" ./cmd/worker

mkdir -p "$build_dir/frontend"
cp -R "$repo_root/frontend/dist/." "$build_dir/frontend/"
printf 'Release artifacts: %s\n' "$build_dir"
