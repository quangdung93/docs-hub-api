// Package tokenrevoke giữ quy ước khóa cho danh sách access token đã thu hồi.
//
// Access token là JWT xác thực bằng chữ ký nên bản thân nó không thu hồi được:
// logout xong token vẫn hợp lệ tới khi hết hạn. Ghi token vào một danh sách
// chặn ngắn hạn là cách duy nhất để logout có tác dụng ngay.
//
// Đặt riêng một package vì CẢ usecase (lúc ghi) lẫn middleware (lúc đọc) đều
// cần đúng một định dạng khóa — để mỗi bên tự dựng chuỗi thì chỉ cần lệch một
// ký tự là logout im lặng mất tác dụng, không có lỗi nào báo ra.
package tokenrevoke

import (
	"crypto/sha256"
	"encoding/hex"
)

// keyPrefix tách riêng khỏi các key khác trong Redis (rate limit, cache user...).
const keyPrefix = "auth:revoked:"

// Key trả về khóa Redis cho một access token.
//
// Băm SHA-256 thay vì lưu token thô: token là thông tin nhạy cảm, ai đọc được
// Redis sẽ dùng lại được ngay. Băm rồi thì chỉ tra cứu được, không tái sử dụng.
func Key(accessToken string) string {
	sum := sha256.Sum256([]byte(accessToken))
	return keyPrefix + hex.EncodeToString(sum[:])
}
