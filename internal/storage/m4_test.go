package storage

import (
	"context"
	"testing"
)

func TestM4PreferencesMigration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := s.Preferences(ctx); err != nil || len(b) != 0 {
		t.Fatal(string(b), err)
	}
	for _, b := range [][]byte{[]byte("invalid"), make([]byte, 4097)} {
		if s.SavePreferences(ctx, b) == nil {
			t.Fatal("invalid preferences accepted")
		}
	}
	if err = s.SavePreferences(ctx, []byte(`{"workers":2}`)); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := s.Preferences(ctx); err != nil || string(b) != `{"workers":2}` {
		t.Fatal(string(b), err)
	}
	// Reconstruct the released M3 database. A failing M4 migration must leave its
	// checkpoints, version and read-only status intact.
	j := Job{Version: 1, ID: "m3", State: "paused", Durable: 4, Intervals: []Interval{{Start: 0, End: 4, Digest: "kept"}}}
	if err = s.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE browser_requests; DROP TABLE preferences; PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	original := migrationM4
	migrationM4 = "CREATE TABLE temporary_m4(x); invalid;"
	_, err = Open(dir, false)
	migrationM4 = original
	if err == nil {
		t.Fatal("failed migration accepted")
	}
	ro, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	var version, count int
	if err = ro.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatal(version, err)
	}
	if err = ro.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='temporary_m4'").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = ro.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, j.ID)
	if err != nil || got.Durable != 4 || len(got.Intervals) != 1 {
		t.Fatal(got, err)
	}
	if err = s.Delete(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, j.ID); err == nil {
		t.Fatal("history not removed")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Preferences(ctx); err == nil {
		t.Fatal("closed store readable")
	}
	if s.SavePreferences(ctx, []byte(`{}`)) == nil || s.Delete(ctx, "m3") == nil {
		t.Fatal("closed store writable")
	}
}
