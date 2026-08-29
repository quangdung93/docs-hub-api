//go:build integration

package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

type integrationStore struct{ objects map[string][]byte }

func (s *integrationStore) Put(_ context.Context, key string, data []byte, contentType string) (port.StoredObject, error) {
	return s.PutReader(context.Background(), key, bytes.NewReader(data), int64(len(data)), contentType)
}
func (s *integrationStore) PutReader(_ context.Context, key string, reader io.Reader, _ int64, contentType string) (port.StoredObject, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return port.StoredObject{}, err
	}
	s.objects[key] = data
	return port.StoredObject{Key: key, Size: int64(len(data)), ContentType: contentType}, nil
}
func (s *integrationStore) Get(_ context.Context, key string) ([]byte, error) {
	return s.objects[key], nil
}
func (s *integrationStore) GetReader(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("object không tồn tại")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *integrationStore) Stat(_ context.Context, key string) (port.StoredObject, error) {
	data, ok := s.objects[key]
	if !ok {
		return port.StoredObject{}, fmt.Errorf("object không tồn tại")
	}
	return port.StoredObject{Key: key, Size: int64(len(data))}, nil
}
func (*integrationStore) PresignedPutURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (*integrationStore) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (s *integrationStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

type integrationEmbedder struct{}

func (integrationEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i + 1), 0.5, -0.5}
	}
	return out, nil
}

func TestProcessor_UploadDenChunks(t *testing.T) {
	// TEST_POSTGRES_DSN chứ không phải TEST_DATABASE_DSN: cả hai CI đều đặt tên
	// trước, nên suốt thời gian qua test này luôn bị Skip, chưa từng chạy lần nào.
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("cần TEST_POSTGRES_DSN để chạy test này")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	userID, projectID := uuid.New(), uuid.New()
	versionID, documentID := uuid.New(), uuid.New()
	revisionID, jobID := uuid.New(), uuid.New()
	objectKey := "integration/" + revisionID.String() + "/fixture.md"
	content := []byte("# Tổng quan\nDòng một\nDòng hai\n\n# Chi tiết\nDòng ba")

	require.NoError(t, db.Exec(`INSERT INTO users(id,email,full_name,password_hash) VALUES(?,?,?,?)`,
		userID, userID.String()+"@test.local", "Integration", "hash").Error)
	require.NoError(t, db.Exec(`INSERT INTO projects(id,code,name,owner_id) VALUES(?,?,?,?)`,
		projectID, "it-"+projectID.String(), "Integration", userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO project_members(id,project_id,user_id,role) VALUES(?,?,?,'owner')`,
		uuid.New(), projectID, userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO project_versions(id,project_id,label,sequence_no,status,created_by)
		VALUES(?,?,?,1,'draft',?)`, versionID, projectID, "v1", userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO documents(id,project_id,title,document_key,created_by)
		VALUES(?,?,?,?,?)`, documentID, projectID, "Fixture", documentID.String(), userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO document_revisions(
		id,document_id,project_id,project_version_id,revision_no,file_name,media_type,size_bytes,
		sha256,object_key,status,created_by) VALUES(?,?,?,?,1,?,'text/markdown',?,repeat('a',64),?,'queued',?)`,
		revisionID, documentID, projectID, versionID, "fixture.md", len(content), objectKey, userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO ingestion_jobs(id,document_revision_id,status) VALUES(?,?,'pending')`,
		jobID, revisionID).Error)

	t.Cleanup(func() {
		db.Exec("DELETE FROM ingestion_jobs WHERE id=?", jobID)
		db.Exec("DELETE FROM document_revisions WHERE id=?", revisionID)
		db.Exec("DELETE FROM documents WHERE id=?", documentID)
		db.Exec("DELETE FROM project_versions WHERE id=?", versionID)
		db.Exec("DELETE FROM project_members WHERE project_id=?", projectID)
		db.Exec("DELETE FROM projects WHERE id=?", projectID)
		db.Exec("DELETE FROM users WHERE id=?", userID)
	})

	store := &integrationStore{objects: map[string][]byte{objectKey: content}}
	processor := NewProcessor(db, store, integrationEmbedder{}, Config{
		ChunkLines: 4, OverlapLines: 1, BatchSize: 2,
		EmbeddingModel: "integration-model", EmbeddingDimension: 3,
	})
	processed, err := processor.ProcessNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed)

	var revision struct{ Status, CanonicalTextKey, ParserVersion, EmbeddingModel string }
	require.NoError(t, db.Table("document_revisions").Select(
		"status,canonical_text_key,parser_version,embedding_model").Where("id=?", revisionID).Scan(&revision).Error)
	require.Equal(t, "ready", revision.Status)
	require.Equal(t, "markdown-v1", revision.ParserVersion)
	require.Equal(t, "integration-model", revision.EmbeddingModel)
	require.Equal(t, content, store.objects[revision.CanonicalTextKey])

	var chunkCount int64
	require.NoError(t, db.Table("document_chunks").Where("document_revision_id=?", revisionID).Count(&chunkCount).Error)
	require.EqualValues(t, 2, chunkCount)
	var jobStatus string
	require.NoError(t, db.Table("ingestion_jobs").Select("status").Where("id=?", jobID).Scan(&jobStatus).Error)
	require.Equal(t, "succeeded", jobStatus)
}
