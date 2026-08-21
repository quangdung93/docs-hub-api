package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

func TestScopeValid_XORVersionAndChangeRequest(t *testing.T) {
	id := uuid.New()
	tests := []struct {
		name  string
		scope domain.Scope
		want  bool
	}{
		{"version hợp lệ", domain.Scope{VersionID: &id}, true},
		{"change request hợp lệ", domain.Scope{ChangeRequestID: &id}, true},
		{"thiếu scope", domain.Scope{}, false},
		{"hai scope cùng lúc", domain.Scope{VersionID: &id, ChangeRequestID: &id}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { require.Equal(t, tt.want, tt.scope.Valid()) })
	}
}
