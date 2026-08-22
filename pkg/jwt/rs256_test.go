package jwt_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/pkg/jwt"
)

// rsaManager tạo Manager RS256 với khóa sinh tại chỗ.
func rsaManager(t *testing.T) *jwt.Manager {
	t.Helper()
	// 2048 bit là mức tối thiểu còn được coi là an toàn; sinh khóa nhỏ hơn cho
	// nhanh sẽ không phản ánh đúng hành vi thật.
	pem, err := jwt.GenerateRSAKeyPEM(2048)
	require.NoError(t, err)

	m, err := jwt.NewManager(jwt.Config{
		Algorithm:     jwt.AlgRS256,
		PrivateKeyPEM: pem,
		Issuer:        "test-issuer",
		AccessTTL:     15 * time.Minute,
	})
	require.NoError(t, err)
	return m
}

func TestRS256_KyVaVerify(t *testing.T) {
	m := rsaManager(t)
	require.Equal(t, jwt.AlgRS256, m.Algorithm())

	token, err := m.Sign("user-1", "a@docshub.io.vn", []string{"admin"}, time.Now())
	require.NoError(t, err)

	claims, err := m.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.UserID)
	assert.Equal(t, []string{"admin"}, claims.Roles)
}

// TestRS256_HeaderCoKid: client cần kid để chọn đúng khóa trong JWKS khi có
// nhiều khóa (lúc xoay khóa).
func TestRS256_HeaderCoKid(t *testing.T) {
	m := rsaManager(t)
	token, err := m.Sign("user-1", "a@docshub.io.vn", nil, time.Now())
	require.NoError(t, err)

	parsed, _, err := jwtlib.NewParser().ParseUnverified(token, jwtlib.MapClaims{})
	require.NoError(t, err)
	assert.Equal(t, "RS256", parsed.Header["alg"])

	kid, _ := parsed.Header["kid"].(string)
	require.NotEmpty(t, kid, "thiếu kid thì client không biết dùng khóa nào")
	assert.Equal(t, m.PublicJWKS().Keys[0].Kid, kid, "kid trong token phải khớp kid trong JWKS")
}

// TestPublicJWKS_DungChuan kiểm tra hình dạng theo RFC 7517.
func TestPublicJWKS_DungChuan(t *testing.T) {
	m := rsaManager(t)
	set := m.PublicJWKS()

	require.Len(t, set.Keys, 1)
	k := set.Keys[0]
	assert.Equal(t, "RSA", k.Kty)
	assert.Equal(t, "sig", k.Use)
	assert.Equal(t, "RS256", k.Alg)
	assert.NotEmpty(t, k.Kid)

	// n/e phải là base64url KHÔNG padding — có "=" là client chuẩn parse hỏng.
	for name, v := range map[string]string{"n": k.N, "e": k.E} {
		assert.NotEmpty(t, v, name)
		assert.NotContains(t, v, "=", name+" phải là base64url không padding")
		_, err := base64.RawURLEncoding.DecodeString(v)
		assert.NoError(t, err, name+" phải giải mã được bằng base64url")
	}
}

// TestPublicJWKS_HS256TraVeRong: secret đối xứng KHÔNG được công bố — ai có nó
// thì ký được token, không chỉ verify.
func TestPublicJWKS_HS256TraVeRong(t *testing.T) {
	m, err := jwt.NewManager(jwt.Config{
		Algorithm: jwt.AlgHS256,
		Secret:    "test-secret",
		Issuer:    "test-issuer",
		AccessTTL: time.Minute,
	})
	require.NoError(t, err)
	assert.Empty(t, m.PublicJWKS().Keys)
}

// TestRS256_ChanAlgorithmConfusion là ca quan trọng nhất của RS256.
//
// Khóa công khai ai cũng lấy được qua JWKS. Nếu server chấp nhận thuật toán tùy
// theo header của token, kẻ tấn công đổi alg sang HS256 rồi ký bằng chính khóa
// công khai đó — và server sẽ coi là hợp lệ. Phải chặn.
func TestRS256_ChanAlgorithmConfusion(t *testing.T) {
	m := rsaManager(t)
	jwk := m.PublicJWKS().Keys[0]

	// Giả lập: kẻ tấn công tự ký token HS256 bằng dữ liệu khóa CÔNG KHAI.
	forged := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"user_id": "ke-tan-cong",
		"iss":     "test-issuer",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, err := forged.SignedString([]byte(jwk.N))
	require.NoError(t, err)

	_, err = m.Verify(signed)
	require.Error(t, err, "server RS256 KHÔNG được chấp nhận token ký bằng HS256")
	assert.ErrorIs(t, err, jwt.ErrInvalidToken)
}

// TestRS256_TokenCuaKhoaKhac: token ký bằng khóa khác phải bị từ chối.
func TestRS256_TokenCuaKhoaKhac(t *testing.T) {
	m1, m2 := rsaManager(t), rsaManager(t)

	token, err := m1.Sign("user-1", "a@docshub.io.vn", nil, time.Now())
	require.NoError(t, err)

	_, err = m2.Verify(token)
	require.Error(t, err)
	assert.NotEqual(t, m1.PublicJWKS().Keys[0].Kid, m2.PublicJWKS().Keys[0].Kid,
		"hai khóa khác nhau phải cho kid khác nhau")
}

func TestNewManager_LoiCauHinh(t *testing.T) {
	t.Run("RS256 thiếu khóa riêng", func(t *testing.T) {
		_, err := jwt.NewManager(jwt.Config{Algorithm: jwt.AlgRS256, Issuer: "i", AccessTTL: time.Minute})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "thiếu khóa riêng")
	})

	t.Run("RS256 khóa không phải PEM", func(t *testing.T) {
		_, err := jwt.NewManager(jwt.Config{
			Algorithm: jwt.AlgRS256, PrivateKeyPEM: "khong-phai-pem",
			Issuer: "i", AccessTTL: time.Minute,
		})
		require.Error(t, err)
	})

	t.Run("thuật toán lạ", func(t *testing.T) {
		_, err := jwt.NewManager(jwt.Config{Algorithm: "ES256", Issuer: "i", AccessTTL: time.Minute})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "không hỗ trợ")
	})

	t.Run("bỏ trống algorithm thì mặc định HS256", func(t *testing.T) {
		m, err := jwt.NewManager(jwt.Config{Secret: "s", Issuer: "i", AccessTTL: time.Minute})
		require.NoError(t, err)
		assert.Equal(t, jwt.AlgHS256, m.Algorithm())
	})
}

// TestGenerateRSAKeyPEM_DungDinhDang: khóa sinh ra phải nạp lại được.
func TestGenerateRSAKeyPEM_DungDinhDang(t *testing.T) {
	pem, err := jwt.GenerateRSAKeyPEM(2048)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(pem, "-----BEGIN PRIVATE KEY-----"))

	_, err = jwt.NewManager(jwt.Config{
		Algorithm: jwt.AlgRS256, PrivateKeyPEM: pem, Issuer: "i", AccessTTL: time.Minute,
	})
	require.NoError(t, err)
}
