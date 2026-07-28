#!/usr/bin/env bash
# Đổi module path từ "document-hub-api" sang path git chính thức trong 1 lệnh.
# Dùng: ./scripts/rename-module.sh git.fpt.net/isc/document-hub/document-hub-api
set -euo pipefail

NEW_MODULE="${1:-}"
OLD_MODULE="document-hub-api"

if [ -z "$NEW_MODULE" ]; then
  echo "Usage: $0 <new-module-path>"
  echo "Ví dụ: $0 git.fpt.net/isc/document-hub/document-hub-api"
  exit 1
fi

echo "▶ Đổi module: $OLD_MODULE -> $NEW_MODULE"

# 1) go.mod
go mod edit -module "$NEW_MODULE"

# 2) Thay import trong mọi file .go (bỏ qua vendor).
#    Dùng ranh giới để tránh thay nhầm chuỗi khác (chỉ thay import path).
grep -rl --include='*.go' "\"$OLD_MODULE" . \
  | grep -v '/vendor/' \
  | xargs sed -i.bak "s|\"$OLD_MODULE|\"$NEW_MODULE|g"

# 3) Thay trong file cấu hình công cụ tham chiếu module.
for f in .golangci.yml .mockery.yaml; do
  [ -f "$f" ] && sed -i.bak "s|$OLD_MODULE|$NEW_MODULE|g" "$f"
done

# 4) Dọn file backup .bak.
find . -name '*.bak' -not -path './vendor/*' -delete

echo "✅ Xong. Kiểm tra lại: go build ./..."
