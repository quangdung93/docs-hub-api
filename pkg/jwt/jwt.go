// Package jwt ký và xác thực JSON Web Token.
//
// Hỗ trợ HS256 (secret đối xứng) và RS256 (cặp khóa RSA). Gói này thuần, không
// phụ thuộc internal/*, để tái sử dụng được ngoài repo.
//
// Vì sao có RS256: với HS256, ai verify được token thì cũng KÝ được token — muốn
// dịch vụ khác tự kiểm tra chữ ký thì phải đưa họ secret, đồng nghĩa trao luôn
// quyền phát hành token. RS256 tách hai việc đó: chỉ service này giữ khóa riêng
// để ký, còn ai cũng lấy được khóa công khai qua JWKS để verify.
package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Lỗi chuẩn hóa để caller phân biệt nguyên nhân.
var (
	ErrInvalidToken = errors.New("token không hợp lệ")
	ErrExpiredToken = errors.New("token đã hết hạn")
)

// Thuật toán ký được hỗ trợ.
const (
	AlgHS256 = "HS256"
	AlgRS256 = "RS256"
)

// Claims là payload của access token.
type Claims struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	jwtlib.RegisteredClaims
}

// Manager ký/verify token với cấu hình cố định.
type Manager struct {
	alg       string
	secret    []byte          // chỉ dùng khi HS256
	privKey   *rsa.PrivateKey // chỉ dùng khi RS256
	keyID     string          // "kid" trong header, để client biết chọn khóa nào
	issuer    string
	accessTTL time.Duration
}

// Config là tham số khởi tạo Manager.
type Config struct {
	// Algorithm là HS256 hoặc RS256. Bỏ trống thì mặc định HS256.
	Algorithm string
	// Secret dùng cho HS256.
	Secret string
	// PrivateKeyPEM là khóa riêng RSA dạng PEM, dùng cho RS256.
	// Khóa công khai được suy ra từ đây, không cần cấu hình riêng.
	PrivateKeyPEM string
	Issuer        string
	AccessTTL     time.Duration
}

// NewManager tạo Manager. Lỗi nếu thiếu khóa tương ứng với thuật toán đã chọn.
func NewManager(cfg Config) (*Manager, error) {
	alg := cfg.Algorithm
	if alg == "" {
		alg = AlgHS256
	}

	m := &Manager{alg: alg, issuer: cfg.Issuer, accessTTL: cfg.AccessTTL}

	switch alg {
	case AlgHS256:
		if cfg.Secret == "" {
			return nil, errors.New("jwt: secret rỗng")
		}
		m.secret = []byte(cfg.Secret)
	case AlgRS256:
		key, err := parseRSAPrivateKey(cfg.PrivateKeyPEM)
		if err != nil {
			return nil, err
		}
		m.privKey = key
		m.keyID = thumbprint(&key.PublicKey)
	default:
		return nil, fmt.Errorf("jwt: thuật toán không hỗ trợ: %s", alg)
	}
	return m, nil
}

// Algorithm trả về thuật toán đang dùng.
func (m *Manager) Algorithm() string { return m.alg }

// Sign tạo access token cho user với roles cho trước.
func (m *Manager) Sign(userID, email string, roles []string, now time.Time) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Roles:  roles,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(m.accessTTL)),
		},
	}

	var (
		token *jwtlib.Token
		key   any
	)
	if m.alg == AlgRS256 {
		token = jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
		// kid để bên verify chọn đúng khóa khi JWKS có nhiều khóa (lúc xoay khóa).
		token.Header["kid"] = m.keyID
		key = m.privKey
	} else {
		token = jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
		key = m.secret
	}

	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("ký token thất bại: %w", err)
	}
	return signed, nil
}

// Verify xác thực token và trả về claims. Phân biệt hết hạn vs không hợp lệ.
func (m *Manager) Verify(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwtlib.ParseWithClaims(tokenString, claims, m.keyFor, jwtlib.WithIssuer(m.issuer))

	switch {
	case err == nil:
		return claims, nil
	case errors.Is(err, jwtlib.ErrTokenExpired):
		return nil, ErrExpiredToken
	default:
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
}

// keyFor chọn khóa verify và CHẶN token ký bằng thuật toán khác với cấu hình.
//
// Bắt buộc kiểm tra: nếu chấp nhận thuật toán tùy ý thì kẻ tấn công đổi header
// sang HS256 rồi ký bằng chính khóa công khai (vốn ai cũng lấy được qua JWKS) —
// lỗ hổng "algorithm confusion" kinh điển.
func (m *Manager) keyFor(t *jwtlib.Token) (any, error) {
	if m.alg == AlgRS256 {
		if _, ok := t.Method.(*jwtlib.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("%w: thuật toán ký không mong đợi", ErrInvalidToken)
		}
		return &m.privKey.PublicKey, nil
	}
	if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("%w: thuật toán ký không mong đợi", ErrInvalidToken)
	}
	return m.secret, nil
}

// JWK là một khóa công khai theo RFC 7517.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS là bộ khóa công khai theo RFC 7517.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// PublicJWKS trả về khóa công khai để bên khác tự verify chữ ký.
//
// Trả bộ RỖNG khi đang dùng HS256: secret đối xứng không thể công bố, đưa ra là
// trao luôn quyền ký token.
func (m *Manager) PublicJWKS() JWKS {
	if m.alg != AlgRS256 || m.privKey == nil {
		return JWKS{Keys: []JWK{}}
	}
	pub := &m.privKey.PublicKey
	return JWKS{Keys: []JWK{{
		Kty: "RSA",
		Use: "sig",
		Alg: AlgRS256,
		Kid: m.keyID,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}

// parseRSAPrivateKey đọc khóa riêng PEM, chấp nhận cả PKCS#1 lẫn PKCS#8 vì hai
// định dạng này sinh ra bởi các lệnh openssl khác nhau và rất dễ nhầm.
func parseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	if pemData == "" {
		return nil, errors.New("jwt: thiếu khóa riêng RSA cho RS256")
	}
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("jwt: khóa riêng RSA không phải định dạng PEM hợp lệ")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("jwt: đọc khóa riêng RSA thất bại: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("jwt: khóa riêng không phải RSA")
	}
	return key, nil
}

// thumbprint sinh "kid" từ chính nội dung khóa công khai theo RFC 7638.
//
// Suy ra từ khóa thay vì để người dùng tự đặt: kid luôn khớp với khóa, đổi khóa
// là kid tự đổi theo, không sợ quên cập nhật.
func thumbprint(pub *rsa.PublicKey) string {
	payload, _ := json.Marshal(map[string]string{
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		"kty": "RSA",
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
	})
	sum := sha256.Sum256(payload)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GenerateRSAKeyPEM sinh khóa riêng RSA mới dạng PEM (PKCS#8).
//
// Dùng cho môi trường local và test, KHÔNG dùng ở production: mỗi lần chạy sinh
// khóa khác nhau nên token cũ mất hiệu lực và nhiều instance không verify được
// token của nhau.
func GenerateRSAKeyPEM(bits int) (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", fmt.Errorf("sinh khóa RSA: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("mã hóa khóa RSA: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}
