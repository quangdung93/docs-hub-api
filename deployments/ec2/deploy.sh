#!/usr/bin/env bash
# Tự deploy docs-hub-api + docs-hub-web trên VPS khi nhánh main có commit mới.
#
# Vì sao có script này: VPS CloudZ không ra được GitHub Actions/GHCR nên job
# deploy của CI không chạy được. Thay vì GitHub đẩy xuống, máy tự kéo về:
# systemd timer (deployments/ec2/systemd/) gọi script này vài phút một lần.
#
# Dùng:
#   deploy.sh              # chỉ deploy repo nào có commit mới
#   deploy.sh --force      # deploy lại kể cả khi không có gì mới
#   deploy.sh --force api  # chỉ api (hoặc: web)
#
# Build thất bại thì container cũ vẫn chạy nguyên. Build xong mà service không
# lên thì tự quay về image trước đó (tag :prev).
set -euo pipefail

API_DIR="${API_DIR:-/home/web/docs-hub-api}"
WEB_DIR="${WEB_DIR:-/home/web/docs-hub-web}"
BRANCH="${DEPLOY_BRANCH:-main}"
LOCK_FILE="${LOCK_FILE:-/tmp/docs-hub-deploy.lock}"

log() { printf '%s \033[36m==>\033[0m %s\n' "$(date '+%F %T')" "$*"; }
err() { printf '%s \033[31m!!\033[0m %s\n' "$(date '+%F %T')" "$*" >&2; }

# Trả 0 nếu repo cần deploy; đồng thời đưa working tree về đúng origin/BRANCH.
# reset --hard không đụng file untracked/ignored nên .env.ec2 được giữ nguyên.
sync_repo() {
	local dir="$1" force="$2" before after
	# Gọi trong `if` nên set -e không có tác dụng: phải tự bắt lỗi.
	if ! git -C "$dir" fetch --quiet origin "$BRANCH"; then
		err "$(basename "$dir"): git fetch lỗi, bỏ qua lượt này"
		return 1
	fi
	before=$(git -C "$dir" rev-parse HEAD)
	after=$(git -C "$dir" rev-parse "origin/$BRANCH")
	if [ "$before" = "$after" ] && [ "$force" != 1 ]; then
		return 1
	fi
	log "$(basename "$dir"): ${before:0:7} -> ${after:0:7}"
	git -C "$dir" reset --quiet --hard "origin/$BRANCH" || return 1
}

# Chờ URL trả mã khớp pattern (tối đa ~90s).
wait_http() {
	local url="$1" pattern="$2" code
	for _ in $(seq 1 45); do
		code=$(curl -s -o /dev/null -w '%{http_code}' -m 5 "$url" || true)
		[[ "$code" =~ $pattern ]] && return 0
		sleep 2
	done
	return 1
}

deploy_api() {
	local compose=(docker compose -f deployments/ec2/docker-compose.yml --env-file .env.ec2)
	cd "$API_DIR"

	docker image inspect docs-hub-api:ec2 >/dev/null 2>&1 &&
		docker tag docs-hub-api:ec2 docs-hub-api:prev
	# migrate/api/worker dùng chung một image: chỉ build một lần, build song
	# song ba service sẽ đụng nhau ở bước export ("image already exists").
	log "api: build image"
	"${compose[@]}" build api || { err "api: build lỗi, giữ nguyên bản đang chạy"; return 1; }

	log "api: migration + khởi động"
	if "${compose[@]}" up -d --no-build migrate api worker &&
		wait_http localhost:9090/readyz '^200$'; then
		log "api: ✅ readyz OK"
	else
		err "api: không lên — log 50 dòng cuối, quay về image trước"
		"${compose[@]}" logs --tail 50 migrate api || true
		if docker image inspect docs-hub-api:prev >/dev/null 2>&1; then
			docker tag docs-hub-api:prev docs-hub-api:ec2
			"${compose[@]}" up -d --no-build api worker || true
		fi
		return 1
	fi

	# Idempotent: admin có rồi thì bỏ qua.
	"${compose[@]}" run --rm --no-deps --entrypoint /app/seed migrate \
		-config /app/configs/config.ec2.yaml || err "api: seed lỗi (không chặn deploy)"
	"${compose[@]}" ps worker
}

deploy_web() {
	local compose=(docker compose -f deployments/ec2/docker-compose.yml --env-file .env.ec2)
	cd "$WEB_DIR"

	docker image inspect docs-hub-web:ec2 >/dev/null 2>&1 &&
		docker tag docs-hub-web:ec2 docs-hub-web:prev
	log "web: build image"
	"${compose[@]}" build web || { err "web: build lỗi, giữ nguyên bản đang chạy"; return 1; }

	log "web: khởi động"
	# `/` chuyển hướng sang /login nên chấp nhận mọi mã 2xx/3xx.
	if "${compose[@]}" up -d --no-build web && wait_http localhost:3000/ '^[23]'; then
		log "web: ✅ đang phục vụ"
	else
		err "web: không lên — log 50 dòng cuối, quay về image trước"
		"${compose[@]}" logs --tail 50 web || true
		if docker image inspect docs-hub-web:prev >/dev/null 2>&1; then
			docker tag docs-hub-web:prev docs-hub-web:ec2
			"${compose[@]}" up -d --no-build web || true
		fi
		return 1
	fi
}

main() {
	local force=0 target=all rc=0
	for arg in "$@"; do
		case "$arg" in
		--force) force=1 ;;
		api | web | all) target="$arg" ;;
		*) err "tham số lạ: $arg"; exit 2 ;;
		esac
	done

	# Timer và lệnh tay có thể chạy trùng lúc: lần sau chờ lần trước xong.
	exec 9>"$LOCK_FILE"
	flock 9

	if [ "$target" != web ] && sync_repo "$API_DIR" "$force"; then
		deploy_api || rc=1
	fi
	if [ "$target" != api ] && sync_repo "$WEB_DIR" "$force"; then
		deploy_web || rc=1
	fi

	docker image prune -f --filter "until=168h" >/dev/null || true
	return "$rc"
}

# Gói toàn bộ trong main và gọi ở dòng cuối: `git reset` có thể ghi đè chính
# file này khi đang chạy, bash đã parse xong hàm nên không đọc phải nội dung mới.
main "$@"
exit $?
