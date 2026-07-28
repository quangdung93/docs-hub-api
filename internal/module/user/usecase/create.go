package usecase

import (
	"context"
	"fmt"

	"document-hub-api/internal/common/apperr"
	"document-hub-api/internal/module/user/domain"
)

// userCreatedEvent là payload sự kiện user.created.
type userCreatedEvent struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

const eventUserCreated = "user.created"

// Create tạo user mới:
//  1. Kiểm tra trùng email -> ErrDuplicateEmail (lỗi nghiệp vụ, HTTP 200).
//  2. Băm mật khẩu.
//  3. Trong 1 transaction: lưu user + publish user.created (publish lỗi -> rollback).
func (s *service) Create(ctx context.Context, in CreateInput) (*domain.User, error) {
	exists, err := s.repo.ExistsByEmail(ctx, in.Email, nil)
	if err != nil {
		return nil, apperr.Database("Lỗi kiểm tra email").WithCause(err)
	}
	if exists {
		return nil, domain.ErrDuplicateEmail.WithDetails(map[string]string{"field": "email"})
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, apperr.Internal("Không thể xử lý mật khẩu").WithCause(err)
	}

	user := domain.NewUser(in.Email, in.FullName, hash, in.Roles)

	err = s.tx.Do(ctx, func(txCtx context.Context) error {
		if createErr := s.repo.Create(txCtx, user); createErr != nil {
			return mapCreateError(createErr)
		}
		return s.publishEvent(txCtx, eventUserCreated,
			userCreatedEvent{UserID: user.ID.String(), Email: user.Email})
	})
	if err != nil {
		return nil, wrapTxError(err)
	}

	s.cacheUser(ctx, user)
	return user, nil
}

// mapCreateError chuyển lỗi repo khi tạo sang lỗi nghiệp vụ/kỹ thuật phù hợp.
func mapCreateError(err error) error {
	switch {
	case isRepoErr(err, domain.ErrDuplicate):
		return domain.ErrDuplicateEmail.WithDetails(map[string]string{"field": "email"})
	default:
		return apperr.Database("Lỗi tạo người dùng").WithCause(err)
	}
}

// wrapTxError giữ nguyên apperr đã phân loại; lỗi lạ -> bọc generic.
func wrapTxError(err error) error {
	if apperr.IsBusiness(err) || apperr.IsTechnical(err) {
		return err
	}
	return apperr.Database("Thao tác dữ liệu thất bại").WithCause(fmt.Errorf("tx: %w", err))
}
