package migrations_test

import (
	"testing"

	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

func TestMigrationsApply(t *testing.T) {
	database := testdatabase.Open(t)
	if database.MigrationVersion != 4 {
		t.Errorf("version = %d, want 4", database.MigrationVersion)
	}
}
