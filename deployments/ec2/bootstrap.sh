#!/usr/bin/env bash
# Chuẩn bị một máy EC2 trắng để chạy docs-hub-api: cài Docker + compose plugin,
# thêm user hiện tại vào group docker, bật swap nếu RAM dưới 4GB.
#
# Chạy MỘT LẦN trên EC2:  bash deployments/ec2/bootstrap.sh
# Hỗ trợ: Amazon Linux 2023, Ubuntu 22.04/24.04.
set -euo pipefail

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }

if [ "$(id -u)" -eq 0 ]; then
	SUDO=""
else
	SUDO="sudo"
fi

# ------------------------------------------------------------------ Docker
if command -v docker >/dev/null 2>&1; then
	log "Docker đã có: $(docker --version)"
else
	# shellcheck disable=SC1091
	. /etc/os-release
	case "${ID}" in
	amzn)
		log "Amazon Linux — cài docker qua dnf"
		$SUDO dnf install -y docker
		;;
	ubuntu | debian)
		$SUDO apt-get update -y
		$SUDO apt-get install -y ca-certificates curl gnupg
		# Repo chính thức của Docker chỉ có một số codename; bản Ubuntu quá mới
		# (ví dụ 26.04 "resolute") chưa được hỗ trợ -> dùng gói của distro.
		if curl -fsI "https://download.docker.com/linux/${ID}/dists/${VERSION_CODENAME}/Release" >/dev/null 2>&1; then
			log "Ubuntu/Debian ${VERSION_CODENAME} — cài docker từ repo chính thức"
			$SUDO install -m 0755 -d /etc/apt/keyrings
			curl -fsSL "https://download.docker.com/linux/${ID}/gpg" |
				$SUDO gpg --dearmor -o /etc/apt/keyrings/docker.gpg
			$SUDO chmod a+r /etc/apt/keyrings/docker.gpg
			echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${ID} ${VERSION_CODENAME} stable" |
				$SUDO tee /etc/apt/sources.list.d/docker.list >/dev/null
			$SUDO apt-get update -y
			$SUDO apt-get install -y docker-ce docker-ce-cli containerd.io \
				docker-buildx-plugin docker-compose-plugin
		else
			log "Repo Docker chưa hỗ trợ ${VERSION_CODENAME} — dùng gói của distro"
			$SUDO apt-get install -y docker.io docker-compose-v2 docker-buildx
		fi
		;;
	*)
		echo "❌ Distro '${ID}' chưa được hỗ trợ, cài Docker thủ công rồi chạy lại." >&2
		exit 1
		;;
	esac
	$SUDO systemctl enable --now docker
fi

# ------------------------------------------------- compose plugin (Amazon Linux)
if ! docker compose version >/dev/null 2>&1; then
	log "Cài docker compose plugin"
	COMPOSE_VERSION="v2.32.4"
	ARCH="$(uname -m)"
	PLUGIN_DIR="/usr/libexec/docker/cli-plugins"
	$SUDO mkdir -p "$PLUGIN_DIR"
	$SUDO curl -fsSL \
		"https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-${ARCH}" \
		-o "${PLUGIN_DIR}/docker-compose"
	$SUDO chmod +x "${PLUGIN_DIR}/docker-compose"
fi
log "compose: $(docker compose version)"

# ------------------------------------------------------------------ group docker
if [ -n "$SUDO" ] && ! id -nG "$USER" | grep -qw docker; then
	log "Thêm $USER vào group docker (cần logout/login lại để có hiệu lực)"
	$SUDO usermod -aG docker "$USER"
	NEED_RELOGIN=1
fi

# ------------------------------------------------------------------ swap
# Build Go trong container ngốn RAM; máy 2GB dễ bị OOM-kill giữa chừng.
MEM_MB=$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo)
if [ "$MEM_MB" -lt 4096 ] && [ ! -f /swapfile ]; then
	log "RAM ${MEM_MB}MB < 4GB — tạo swapfile 4GB"
	$SUDO fallocate -l 4G /swapfile || $SUDO dd if=/dev/zero of=/swapfile bs=1M count=4096
	$SUDO chmod 600 /swapfile
	$SUDO mkswap /swapfile
	$SUDO swapon /swapfile
	echo '/swapfile none swap sw 0 0' | $SUDO tee -a /etc/fstab >/dev/null
fi

log "Xong. Bước tiếp theo:"
echo "  1. cp .env.ec2.example .env.ec2 && vi .env.ec2   # điền secret"
echo "  2. make ec2-up                                    # build + chạy"
echo "  3. curl -s localhost:9090/healthz                 # kiểm tra"
if [ -n "${NEED_RELOGIN:-}" ]; then
	echo
	echo "⚠️  Thoát SSH và vào lại trước khi chạy make ec2-up (group docker mới có hiệu lực)."
fi
