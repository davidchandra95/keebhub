package migrations_test

import (
	"testing"

	"github.com/davidchandra95/keebhub/internal/testutil/testdatabase"
)

func TestMigrationsApply(t *testing.T) {
	database := testdatabase.Open(t)
	if database.MigrationVersion != 3 {
		t.Errorf("version = %d, want 3", database.MigrationVersion)
	}
}
