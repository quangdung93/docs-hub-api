package usecase

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

func uploadTrung(t *testing.T, loi error) (*fakeStore, error) {
	t.Helper()
	actor, pid, vid := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeRepo{role: "editor", scope: true, createErr: loi}
	store := &fakeStore{}
	svc := New(repo, fakeTx{}, store, fakeClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	data := []byte("noi dung trung")
	_, _, _, err := svc.Upload(ctx, UploadInput{
		ProjectID: pid, Scope: domain.Scope{VersionID: &vid}, Title: "Tai lieu",
		FileName: "a.txt", MediaType: mimeTextPlain, SizeBytes: int64(len(data)),
		Reader: bytes.NewReader(data),
	})
	return store, err
}

// Mục #5: trùng nội dung là lỗi NGHIỆP VỤ (HTTP 200, success=false theo
// ADR-0002), không phải SYS_500. Trước bản sửa, lỗi 23505 của Postgres lọt thẳng
// qua apperr.Internal nên client tưởng hệ thống hỏng.
func TestUpload_TrungNoiDungTraLoiNghiepVu(t *testing.T) {
	_, err := uploadTrung(t, domain.ErrDuplicateContent)

	var business *apperr.BusinessError
	require.ErrorAs(t, err, &business, "phải là lỗi nghiệp vụ, không phải lỗi kỹ thuật")
	require.Equal(t, errcode.DuplicateContent, business.Code)
	require.False(t, business.Retryable, "gửi lại đúng file đó thì vẫn trùng")

	var technical *apperr.TechnicalError
	require.NotErrorAs(t, err, &technical, "không được lẫn sang nhánh 5xx")
}

// Lỗi nghiệp vụ vẫn phải dọn object vừa ghi lên storage — file đó không còn
// hàng revision nào trỏ tới, để lại là rác vĩnh viễn.
func TestUpload_TrungNoiDungVanXoaObjectVuaGhi(t *testing.T) {
	store, err := uploadTrung(t, domain.ErrDuplicateContent)

	require.Error(t, err)
	require.NotEmpty(t, store.deleted, "object mồ côi phải được xoá khỏi storage")
}

// Chốt chặn hướng ngược: lỗi database bình thường KHÔNG được đội lốt lỗi nghiệp
// vụ chỉ vì nằm chung một chỗ bắt lỗi.
func TestUpload_LoiKhacVanLaLoiKyThuat(t *testing.T) {
	_, err := uploadTrung(t, fmt.Errorf("mất kết nối database"))

	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, errcode.Sys500, technical.Code)
}
