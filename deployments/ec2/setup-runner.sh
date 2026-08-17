#!/usr/bin/env bash
# Cài GitHub Actions self-hosted runner lên chính máy EC2.
#
# Vì sao cần: security group chỉ mở port 22 cho IP văn phòng/nhà, nên runner của
# GitHub không SSH vào được. Đặt runner ngay trên máy thì bước deploy chạy nội
# bộ — không phải mở thêm cổng nào, và EC2 chỉ `docker compose pull`, không build.
#
# Dùng:
#   bash deployments/ec2/setup-runner.sh <REPO_URL> <REGISTRATION_TOKEN> [RUNNER_NAME]
#
# Lấy REGISTRATION_TOKEN ở: Settings -> Actions -> Runners -> New self-hosted
# runner (token chỉ sống 1 giờ). Mỗi repo cần một runner riêng, nhưng cùng chạy
# được trên một máy — đặt RUNNER_NAME khác nhau.
#
# Ví dụ:
#   bash deployments/ec2/setup-runner.sh https://github.com/quangdung93/docs-hub-api AXXXX api
set -euo pipefail

REPO_URL="${1:?thiếu REPO_URL, ví dụ https://github.com/owner/repo}"
TOKEN="${2:?thiếu REGISTRATION_TOKEN lấy ở Settings -> Actions -> Runners}"
NAME="${3:-$(hostname)-$(basename "$REPO_URL")}"

RUNNER_VERSION="2.321.0"
BASE_DIR="/opt/actions-runner/${NAME}"

log() { printf '\033[36m==>\033[0m %s\n' "$*"; }

log "Cài runner '${NAME}' cho ${REPO_URL}"

sudo mkdir -p "$BASE_DIR"
sudo chown "$USER:$USER" "$BASE_DIR"
cd "$BASE_DIR"

if [ ! -f ./config.sh ]; then
	ARCH=$(uname -m)
	case "$ARCH" in
	x86_64) RUNNER_ARCH="x64" ;;
	aarch64) RUNNER_ARCH="arm64" ;;
	*) echo "❌ Kiến trúc $ARCH chưa hỗ trợ" >&2; exit 1 ;;
	esac

	log "Tải actions-runner ${RUNNER_VERSION} (${RUNNER_ARCH})"
	curl -fsSL -o runner.tar.gz \
		"https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz"
	tar xzf runner.tar.gz && rm runner.tar.gz
fi

# --labels khớp với `runs-on` trong .github/workflows/deploy.yml.
log "Đăng ký runner"
./config.sh --unattended --replace \
	--url "$REPO_URL" \
	--token "$TOKEN" \
	--name "$NAME" \
	--labels self-hosted,docs-hub-ec2 \
	--work _work

log "Cài thành systemd service (tự chạy lại sau reboot)"
sudo ./svc.sh install "$USER"
sudo ./svc.sh start

# Runner chạy dưới user hiện tại nên user đó phải gọi được docker.
if ! id -nG "$USER" | grep -qw docker; then
	log "Thêm $USER vào group docker"
	sudo usermod -aG docker "$USER"
	echo "⚠️  Chạy 'sudo systemctl restart actions.runner.*' sau khi logout/login để runner nhận group mới."
fi

log "Xong. Kiểm tra ở Settings -> Actions -> Runners (trạng thái phải là Idle)."
sudo ./svc.sh status | head -5
