package storage

import (
	"context"
	"testing"
)

// Rebuild the actual released jobs constraint while preserving fixture records.
func releasedJobs(version int) string {
	sql := "DROP TABLE IF EXISTS media_checkpoints;ALTER TABLE jobs RENAME TO saved_jobs;" + migration + "INSERT INTO jobs SELECT * FROM saved_jobs; DROP TABLE saved_jobs;"
	if version >= 2 {
		sql += migrationM3
	}
	return sql
}
func TestM7MigrationPreservesFileCheckpointsAndReceipts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	j := Job{Version: 1, ID: "legacy", State: "paused", Durable: 2, Intervals: []Interval{{Start: 0, End: 2, Digest: "kept"}}}
	if err = s.SaveBrowser(ctx, j, "receipt", "fingerprint"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(releasedJobs(2) + "PRAGMA user_version=4;"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	original := migrationM7
	migrationM7 = "CREATE TABLE broken_media(x);invalid;"
	_, err = Open(dir, false)
	migrationM7 = original
	if err == nil {
		t.Fatal("broken migration succeeded")
	}
	ro, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	var version, count int
	if err = ro.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		t.Fatal(version, err)
	}
	if err = ro.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='broken_media'").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = ro.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := s.Get(ctx, j.ID)
	if err != nil || got.Durable != 2 || len(got.Intervals) != 1 {
		t.Fatal(got, err)
	}
	if hash, id, err := s.BrowserRequest(ctx, "receipt"); err != nil || hash != "fingerprint" || id != j.ID {
		t.Fatal(hash, id, err)
	}
	for _, state := range []string{"analyzing", "assembling"} {
		got.State = state
		got.Kind = "hls"
		got.MediaLoaded = true
		got.Media = []byte(`{"version":1}`)
		if err = s.Save(ctx, got); err != nil {
			t.Fatal(err)
		}
	}
	got, err = s.Get(ctx, j.ID)
	if err != nil || string(got.Media) != `{"version":1}` {
		t.Fatal(got, err)
	}
}

func TestMediaCheckpointTransactionsAndLazySnapshots(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	j := Job{Version: 1, ID: "media", Kind: "hls", State: "paused", MediaLoaded: true, MediaObjects: 2, MediaComplete: 1, MediaBytes: 3, Media: []byte(`{"Version":1,"private":"source-query"}`)}
	if err = s.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.List(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].MediaLoaded || len(jobs[0].Media) != 0 || jobs[0].MediaComplete != 1 {
		t.Fatal("snapshot loaded full checkpoint", jobs, err)
	}
	jobs[0].QueueOrder = 7
	if err = s.SaveBatch(ctx, jobs); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, j.ID)
	if err != nil || string(got.Media) != string(j.Media) || !got.MediaLoaded || got.QueueOrder != 7 {
		t.Fatal(got, err)
	}
	for _, invalid := range []struct {
		kind string
		data []byte
	}{{"hls", []byte("{")}, {"hls", make([]byte, (4<<20)+1)}, {"", []byte(`{}`)}} {
		broken := got
		broken.Kind = invalid.kind
		broken.Media = invalid.data
		broken.MediaComplete = 2
		if err = s.Save(ctx, broken); err == nil {
			t.Fatal("invalid checkpoint committed")
		}
		after, err := s.Get(ctx, j.ID)
		if err != nil || after.MediaComplete != 1 || string(after.Media) != string(j.Media) {
			t.Fatal("summary committed without checkpoint", after, err)
		}
	}
	got.Media = nil
	got.MediaObjects = 0
	got.MediaComplete = 0
	got.MediaBytes = 0
	if err = s.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	after, err := s.Get(ctx, j.ID)
	if err != nil || len(after.Media) != 0 {
		t.Fatal(after, err)
	}
	if err = s.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM media_checkpoints").Scan(&count); err != nil || count != 0 {
		t.Fatal("orphaned media checkpoint", count, err)
	}
	if _, err = s.Get(ctx, j.ID); err == nil {
		t.Fatal("removed job returned")
	}
}
