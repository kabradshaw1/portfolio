//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// archiveScript is the production wrapper from
// java/k8s/configmaps/postgres-wal-scripts.yml; we keep it inline here so
// the test exercises the exact same logic.
const archiveScript = `#!/bin/sh
set -eu
SRC="$1"
FN="$2"
DST_DIR="/var/lib/postgresql/wal-archive"
DST="$DST_DIR/$FN"
TMP="$DST.tmp"
if [ -e "$DST" ]; then
  echo "pg-archive-wal: $FN already archived; refusing overwrite" >&2
  exit 1
fi
cp "$SRC" "$TMP"
sync "$TMP"
mv "$TMP" "$DST"
`

func TestWalArchiveAndIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()

	hostArchive := t.TempDir()
	if err := os.Chmod(hostArchive, 0o777); err != nil {
		t.Fatalf("chmod tmpdir: %v", err)
	}

	hostScript := filepath.Join(t.TempDir(), "pg-archive-wal.sh")
	if err := os.WriteFile(hostScript, []byte(archiveScript), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	pgContainer, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("appdb"),
		postgres.WithUsername("appuser"),
		postgres.WithPassword("apppass"),
		testcontainers.WithCmdArgs(
			"-c", "wal_level=replica",
			"-c", "archive_mode=on",
			"-c", "archive_command=/usr/local/bin/pg-archive-wal.sh %p %f",
			"-c", "archive_timeout=2",
		),
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.Binds = append(hc.Binds,
				hostArchive+":/var/lib/postgresql/wal-archive",
				hostScript+":/usr/local/bin/pg-archive-wal.sh:ro",
			)
		}),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `CREATE TABLE marker (n int)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO marker VALUES (1)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, `SELECT pg_switch_wal()`); err != nil {
		t.Fatalf("switch wal: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	var archived []os.DirEntry
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(hostArchive)
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		archived = nil
		for _, e := range entries {
			if !e.IsDir() && len(e.Name()) == 24 {
				archived = append(archived, e)
			}
		}
		if len(archived) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(archived) == 0 {
		t.Fatalf("no WAL segments archived within 30s")
	}

	srcSegment := filepath.Join(hostArchive, archived[0].Name())
	cmd := exec.CommandContext(ctx, "/bin/sh", hostScript, srcSegment, archived[0].Name())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("expected re-archive to fail with non-zero exit; got success: %s", string(out))
	}
}
