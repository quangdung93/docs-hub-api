package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestProjectSeeds_CoIDHopLeVaKhongTrung(t *testing.T) {
	seen := map[uuid.UUID]bool{}
	for _, project := range projectSeeds {
		assertSeedID(t, seen, project.id)
		require.NotEmpty(t, project.versions)
		for _, version := range project.versions {
			assertSeedID(t, seen, version.id)
		}
		for _, change := range project.changeRequests {
			assertSeedID(t, seen, change.id)
		}
	}
}

func assertSeedID(t *testing.T, seen map[uuid.UUID]bool, value string) {
	t.Helper()
	id, err := uuid.Parse(value)
	require.NoError(t, err)
	require.False(t, seen[id], "seed ID bị trùng: %s", value)
	seen[id] = true
}
