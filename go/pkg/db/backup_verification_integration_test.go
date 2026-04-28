//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"
)

// loadVerifyScript reads pg-verify-backups.sh from the ConfigMap YAML and
// writes it to a temp file the test can mount into the verify container.
func loadVerifyScript(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Join(wd, "..", "..", "..")
	cmPath := filepath.Join(repoRoot, "java", "k8s", "configmaps", "postgres-verify-scripts.yml")
	raw, err := os.ReadFile(cmPath)
	if err != nil {
		t.Fatalf("read configmap: %v", err)
	}
	var doc struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal configmap: %v", err)
	}
	body, ok := doc.Data["pg-verify-backups.sh"]
	if !ok {
		t.Fatalf("pg-verify-backups.sh not found in configmap")
	}
	tmp := filepath.Join(t.TempDir(), "pg-verify-backups.sh")
	if err := os.WriteFile(tmp, []byte(body), 0o555); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return tmp
}

// pushgatewayMock records POST bodies keyed by request path.
type pushgatewayMock struct {
	mu     sync.Mutex
	bodies map[string]string
	server *httptest.Server
}

func newPushgatewayMock() *pushgatewayMock {
	m := &pushgatewayMock{bodies: map[string]string{}}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.bodies[r.URL.Path] = m.bodies[r.URL.Path] + string(body)
		m.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	return m
}

func (m *pushgatewayMock) Close() { m.server.Close() }

func (m *pushgatewayMock) URLForContainer() string {
	addr := m.server.Listener.Addr().String()
	parts := strings.Split(addr, ":")
	port := parts[len(parts)-1]
	return "http://host.docker.internal:" + port
}

func (m *pushgatewayMock) BodyFor(path string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bodies[path]
}

func TestBackupVerification_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()

	src, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("appdb"),
		postgres.WithUsername("appuser"),
		postgres.WithPassword("apppass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start source postgres: %v", err)
	}
	t.Cleanup(func() { _ = src.Terminate(ctx) })

	dsn, err := src.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE widgets (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO widgets (name)
		SELECT 'widget-' || g FROM generate_series(1, 25) g;
		ANALYZE widgets;
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	dumpHostDir := t.TempDir()
	dumpName := "appdb-" + time.Now().UTC().Format("2006-01-02") + ".dump"
	dumpInContainer := "/tmp/" + dumpName
	rc, _, err := src.Exec(ctx, []string{
		"pg_dump", "--format=custom",
		"-U", "appuser", "-d", "appdb",
		"-f", dumpInContainer,
	})
	if err != nil || rc != 0 {
		t.Fatalf("pg_dump exec rc=%d err=%v", rc, err)
	}
	r, err := src.CopyFileFromContainer(ctx, dumpInContainer)
	if err != nil {
		t.Fatalf("copy dump out: %v", err)
	}
	dumpPath := filepath.Join(dumpHostDir, dumpName)
	out, err := os.Create(dumpPath)
	if err != nil {
		t.Fatalf("create dump file: %v", err)
	}
	if _, err := io.Copy(out, r); err != nil {
		t.Fatalf("copy dump: %v", err)
	}
	out.Close()
	r.Close()

	pg := newPushgatewayMock()
	t.Cleanup(pg.Close)

	scriptPath := loadVerifyScript(t)
	verify, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:17-alpine",
			Cmd: []string{"sh", "-c",
				"apk add --no-cache curl >/dev/null && chown -R postgres:postgres /var/lib/postgresql/data && exec gosu postgres /scripts/pg-verify-backups.sh"},
			Env: map[string]string{
				"PUSHGATEWAY_URL": pg.URLForContainer(),
				"VERIFY_DBS":      "appdb",
				"DUMPS_DIR":       "/backups/postgres",
			},
			Files: []testcontainers.ContainerFile{
				{HostFilePath: scriptPath, ContainerFilePath: "/scripts/pg-verify-backups.sh", FileMode: 0o555},
				{HostFilePath: dumpPath, ContainerFilePath: "/backups/postgres/" + dumpName, FileMode: 0o444},
			},
			ExtraHosts: []string{"host.docker.internal:host-gateway"},
			WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start verify container: %v", err)
	}
	t.Cleanup(func() { _ = verify.Terminate(ctx) })

	state, err := verify.State(ctx)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.ExitCode != 0 {
		logs, _ := verify.Logs(ctx)
		buf, _ := io.ReadAll(logs)
		t.Fatalf("verify container exit=%d, logs:\n%s", state.ExitCode, string(buf))
	}

	got := pg.BodyFor("/metrics/job/postgres_backup_verify/instance/appdb")
	if !strings.Contains(got, "backup_verification_last_success_timestamp") {
		t.Errorf("missing last_success_timestamp in pushed body: %q", got)
	}
	if !strings.Contains(got, "backup_verification_restored_rows") {
		t.Errorf("missing restored_rows in pushed body: %q", got)
	}
	overall := pg.BodyFor("/metrics/job/postgres_backup_verify")
	if !strings.Contains(overall, "backup_verification_run_success 1") {
		t.Errorf("missing run_success=1 in overall body: %q", overall)
	}
}

func TestBackupVerification_FailureOnCorruptDump(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()

	dumpHostDir := t.TempDir()
	dumpName := "appdb-" + time.Now().UTC().Format("2006-01-02") + ".dump"
	dumpPath := filepath.Join(dumpHostDir, dumpName)
	if err := os.WriteFile(dumpPath, []byte("not a real pg_dump file"), 0o644); err != nil {
		t.Fatalf("write corrupt dump: %v", err)
	}

	pg := newPushgatewayMock()
	t.Cleanup(pg.Close)

	scriptPath := loadVerifyScript(t)
	verify, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:17-alpine",
			Cmd: []string{"sh", "-c",
				"apk add --no-cache curl >/dev/null && chown -R postgres:postgres /var/lib/postgresql/data && exec gosu postgres /scripts/pg-verify-backups.sh"},
			Env: map[string]string{
				"PUSHGATEWAY_URL": pg.URLForContainer(),
				"VERIFY_DBS":      "appdb",
				"DUMPS_DIR":       "/backups/postgres",
			},
			Files: []testcontainers.ContainerFile{
				{HostFilePath: scriptPath, ContainerFilePath: "/scripts/pg-verify-backups.sh", FileMode: 0o555},
				{HostFilePath: dumpPath, ContainerFilePath: "/backups/postgres/" + dumpName, FileMode: 0o444},
			},
			ExtraHosts: []string{"host.docker.internal:host-gateway"},
			WaitingFor: wait.ForExit().WithExitTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start verify: %v", err)
	}
	t.Cleanup(func() { _ = verify.Terminate(ctx) })

	state, err := verify.State(ctx)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.ExitCode == 0 {
		t.Fatalf("expected non-zero exit on corrupt dump, got 0")
	}

	got := pg.BodyFor("/metrics/job/postgres_backup_verify/instance/appdb")
	if !strings.Contains(got, "backup_verification_last_failure_timestamp") {
		t.Errorf("missing failure_timestamp metric: %q", got)
	}
	if !strings.Contains(got, `reason="pg_restore_failed"`) {
		t.Errorf("expected pg_restore_failed reason in body: %q", got)
	}
	overall := pg.BodyFor("/metrics/job/postgres_backup_verify")
	if !strings.Contains(overall, "backup_verification_run_success 0") {
		t.Errorf("expected run_success=0 in overall body: %q", overall)
	}
}
