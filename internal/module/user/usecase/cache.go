package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"document-hub-api/internal/common/port"
	"document-hub-api/internal/module/user/domain"
)

const (
	userCacheTTL    = 5 * time.Minute
	userCachePrefix = "user:"
)

func userCacheKey(id uuid.UUID) string { return userCachePrefix + id.String() }

// cacheUser lưu user vào cache (best-effort: lỗi cache không làm hỏng nghiệp vụ).
func (s *service) cacheUser(ctx context.Context, u *domain.User) {
	b, err := json.Marshal(u)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, userCacheKey(u.ID), string(b), userCacheTTL)
}

// getCachedUser đọc user từ cache. Trả (nil, false) nếu miss hoặc lỗi.
func (s *service) getCachedUser(ctx context.Context, id uuid.UUID) (*domain.User, bool) {
	raw, err := s.cache.Get(ctx, userCacheKey(id))
	if err != nil {
		if !errors.Is(err, port.ErrCacheMiss) {
			// Lỗi hạ tầng cache: bỏ qua, coi như miss (không chặn nghiệp vụ).
			return nil, false
		}
		return nil, false
	}
	var u domain.User
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		return nil, false
	}
	return &u, true
}

// invalidateUser xóa cache khi user thay đổi.
func (s *service) invalidateUser(ctx context.Context, id uuid.UUID) {
	_ = s.cache.Del(ctx, userCacheKey(id))
}

// publishEvent phát sự kiện; lỗi -> bọc thành MQ_502 để caller (trong tx) rollback.
func (s *service) publishEvent(ctx context.Context, routingKey string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event %q: %w", routingKey, err)
	}
	if err := s.pub.Publish(ctx, port.Event{RoutingKey: routingKey, Body: body}); err != nil {
		return fmt.Errorf("publish event %q: %w", routingKey, err)
	}
	return nil
}
