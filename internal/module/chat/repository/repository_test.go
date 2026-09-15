package repository

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

func TestMapConversations_ActiveScopeJSONNullLaNil(t *testing.T) {
	items, err := mapConversations([]conversationRow{{
		ID:          uuid.NewString(),
		ProjectID:   uuid.NewString(),
		UserID:      uuid.NewString(),
		ActiveScope: []byte("null"),
	}})

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].ActiveScope)
}

func TestMapConversations_ActiveScopeHopLe(t *testing.T) {
	versionID := uuid.New()
	items, err := mapConversations([]conversationRow{{
		ID:          uuid.NewString(),
		ProjectID:   uuid.NewString(),
		UserID:      uuid.NewString(),
		ActiveScope: []byte(`{"mode":"versions","version_ids":["` + versionID.String() + `"]}`),
	}})

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, &retrievaldomain.Scope{
		Mode:       retrievaldomain.ScopeVersions,
		VersionIDs: []uuid.UUID{versionID},
	}, items[0].ActiveScope)
}
