package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRepository(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	j := Job{Version: 1, ID: "job", State: "queued", Length: -1}
	if err = s.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir, false); err == nil {
		t.Fatal("concurrent owner")
	}
	ro, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := ro.List(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("%v %v", jobs, err)
	}
	if err = ro.Save(ctx, j); err == nil {
		t.Fatal("writable status")
	}
	if err = ro.Close(); err != nil {
		t.Fatal(err)
	}
	j.State = "paused"
	j.Durable = 10
	j.Generation = 1
	if err = s.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, j.ID)
	if err != nil || got.Durable != 10 {
		t.Fatal(got, err)
	}
	if _, err = s.Get(ctx, "missing"); err == nil {
		t.Fatal("missing accepted")
	}
	j.Version = 2
	if err = s.Save(ctx, j); err == nil {
		t.Fatal("unsupported version")
	}
	j.Version = 1
	j.State = "invalid"
	if err = s.Save(ctx, j); err == nil {
		t.Fatal("constraint ignored")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err = s.Save(cancelled, got); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.Get(ctx, "job")
	if err != nil || got.State != "paused" {
		t.Fatal(got, err)
	}
	// A migration failure rolls back all earlier DDL and user_version.
	if _, err = s.db.Exec("DROP TABLE jobs; PRAGMA user_version=0"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	original := migration
	migration = "CREATE TABLE transient(x); invalid sql; PRAGMA user_version=1;"
	if _, err = Open(dir, false); err == nil {
		t.Fatal("migration should fail")
	}
	migration = original
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='transient'").Scan(&count); err != nil || count != 0 {
		t.Fatal("migration did not roll back", err)
	}
	if _, err = s.db.Exec("PRAGMA user_version=3"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, readonly := range []bool{true, false} {
		if _, err = Open(dir, readonly); err == nil {
			t.Fatal("new schema accepted")
		}
	}
}
func TestUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(filepath.Join(dir, "missing"), true); err == nil {
		t.Fatal("created missing status database")
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(state, "jobs.db")); err != nil {
		t.Skip(err)
	}
	if _, err := Open(state, false); err == nil {
		t.Fatal("followed database symlink")
	}
	if err := os.Remove(filepath.Join(state, "jobs.db")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(state, "owner.lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(state, "owner.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(state, false); err == nil {
		t.Fatal("followed lock symlink")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(state, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link, false); err == nil {
		t.Fatal("followed state symlink")
	}
}
func TestBrokenRecords(t *testing.T) {
	s, err := Open(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, record := range []string{"bad", `{"Version":2}`} {
		if _, err := s.db.Exec("INSERT OR REPLACE INTO jobs VALUES('bad',1,'queued',0,0,?)", record); err != nil {
			t.Fatal(err)
		}
		if _, err := s.List(context.Background()); err == nil {
			t.Fatal("accepted corrupt record")
		}
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background()); err == nil {
		t.Fatal("closed db")
	}
	if err := s.Save(context.Background(), Job{Version: 1}); err == nil {
		t.Fatal("closed save")
	}
}
func TestSettingsAndIOFailures(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.configure("busy_timeout", "1", "2"); err == nil {
		t.Fatal("did not verify settings")
	}
	if err = s.configure("missing_setting", "1", "1"); err == nil {
		t.Fatal("missing setting accepted")
	}
	if err = s.configure("not valid", "1", "1"); err == nil {
		t.Fatal("invalid pragma accepted")
	}
	if err = s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.configure("busy_timeout", "1", "1"); err == nil {
		t.Fatal("closed settings")
	}
	if _, err = s.Get(ctx, "x"); err == nil {
		t.Fatal("closed get")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err = os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(filepath.Join(file, "child"), false); err == nil {
		t.Fatal("mkdir through file")
	}
	for _, name := range []string{"jobs.db", "jobs.db-wal", "jobs.db-shm"} {
		dir := t.TempDir()
		if err = os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err = Open(dir, false); err == nil {
			t.Fatal("accepted directory", name)
		}
	}
	bad := t.TempDir()
	if err = os.WriteFile(filepath.Join(bad, "jobs.db"), []byte("not sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(bad, false); err == nil {
		t.Fatal("accepted invalid database")
	}
	locked := t.TempDir()
	if err = os.Chmod(locked, 0500); err != nil {
		t.Fatal(err)
	}
	// Protect restores owned directory permissions for ordinary writes.
	s, err = Open(locked, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestReadOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode test; Windows uses a current-user DACL")
	}
	dir := t.TempDir()
	s, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopening never repairs permissions in a read-only status command.
	if err = os.Chmod(filepath.Join(dir, "jobs.db"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir, true); err == nil && os.PathSeparator != '\\' {
		t.Fatal("unsafe status file")
	}
	if err = os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir, true); err == nil && os.PathSeparator != '\\' {
		t.Fatal("unsafe status directory")
	}
}

func TestM2UpgradeAndAtomicReorder(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild the actual released M2 schema, then open it with M3.
	if _, err = s.db.Exec("DROP TABLE jobs;" + migration); err != nil {
		t.Fatal(err)
	}
	j := Job{Version: 1, ID: "legacy", State: "paused", Durable: 2, Intervals: []Interval{{Start: 0, End: 2, Digest: "digest"}}}
	if err = s.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := Open(dir, true)
	if err != nil {
		t.Fatal("M2 read-only status", err)
	}
	if err = ro.Close(); err != nil {
		t.Fatal(err)
	}
	original := migrationM3
	migrationM3 = "CREATE TABLE temporary_m3(x); invalid;"
	if _, err = Open(dir, false); err == nil {
		t.Fatal("broken migration accepted")
	}
	migrationM3 = original
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	got, err := s.Get(ctx, j.ID)
	if err != nil || got.Durable != 2 || len(got.Intervals) != 1 {
		t.Fatal("upgrade lost checkpoint", got, err)
	}
	var version, count int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatal(version, err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='temporary_m3'").Scan(&count); err != nil || count != 0 {
		t.Fatal("migration rollback", count, err)
	}
	got.State = "retry_wait"
	got.QueueOrder = 4
	if err = s.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	changed := got
	changed.QueueOrder = 1
	invalid := Job{Version: 1, ID: "bad", State: "invalid"}
	if err = s.SaveBatch(ctx, []Job{changed, invalid}); err == nil {
		t.Fatal("invalid batch accepted")
	}
	saved, err := s.Get(ctx, j.ID)
	if err != nil || saved.QueueOrder != 4 {
		t.Fatal("partial reorder committed", saved, err)
	}
}
