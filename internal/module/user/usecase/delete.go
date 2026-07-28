package usecase

import (
	"context"

	"github.com/google/uuid"
)

// Delete xóa mềm user theo optimistic lock. Không tác động dòng nào -> phân biệt
// version conflict vs not found (giống Update, qua resolveWriteConflict).
func (s *service) Delete(ctx context.Context, id uuid.UUID, version int) error {
	if err := s.repo.SoftDelete(ctx, id, version); err != nil {
		return s.resolveWriteConflict(ctx, id, err)
	}
	s.invalidateUser(ctx, id)
	return nil
}
