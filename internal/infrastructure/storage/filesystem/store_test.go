package filesystem

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

func TestStore_VongDoiObject(t *testing.T) {
	store, err := New(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()
	key := "projects/p1/documents/d1/file.txt"

	stored, err := store.PutReader(ctx, key, bytes.NewBufferString("xin chào"), int64(len("xin chào")), "text/plain")
	require.NoError(t, err)
	require.EqualValues(t, len("xin chào"), stored.Size)

	data, err := store.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "xin chào", string(data))

	info, err := store.Stat(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "text/plain; charset=utf-8", info.ContentType)

	require.NoError(t, store.Delete(ctx, key))
	_, err = store.Get(ctx, key)
	require.Error(t, err)
}

func TestStore_TuChoiThoatKhoiRootVaSaiKichThuoc(t *testing.T) {
	store, err := New(t.TempDir())
	require.NoError(t, err)
	_, err = store.Put(context.Background(), "../../secret", []byte("x"), "text/plain")
	require.Error(t, err)
	_, err = store.PutReader(context.Background(), "safe/file.txt", bytes.NewBufferString("abc"), 2, "text/plain")
	require.ErrorContains(t, err, "kích thước")
}

func TestStore_KhongHoTroPresign(t *testing.T) {
	store, err := New(t.TempDir())
	require.NoError(t, err)
	_, err = store.PresignedPutURL(context.Background(), "file", time.Minute)
	require.True(t, errors.Is(err, port.ErrPresignUnsupported))
}
