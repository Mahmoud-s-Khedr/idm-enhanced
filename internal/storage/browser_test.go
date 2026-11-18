package storage

import (
	"context"
	"testing"
)

func TestBrowserAtomicReceipts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE browser_requests;" + releasedJobs(2) + "PRAGMA user_version=3;"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	original := migrationM5
	migrationM5 = "CREATE TABLE broken_m5(x); invalid;"
	_, err = Open(dir, false)
	migrationM5 = original
	if err == nil {
		t.Fatal("invalid migration")
	}
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	j := Job{Version: 1, ID: "browser", State: "queued"}
	if err = s.SaveBrowser(ctx, j, "request", "hash"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveBrowser(ctx, Job{Version: 1, ID: "duplicate", State: "queued"}, "request", "hash"); err == nil {
		t.Fatal("duplicate receipt")
	}
	jobs, err := s.List(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	if err = s.SaveBrowser(ctx, Job{Version: 9, ID: "bad"}, "bad", "hash"); err == nil {
		t.Fatal("invalid record")
	}
	if _, id, err := s.BrowserRequest(ctx, "bad"); err != nil || id != "" {
		t.Fatal("receipt committed without job", id, err)
	}
	if err = s.Delete(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, id, err := s.BrowserRequest(ctx, "request"); err != nil || id != j.ID {
		t.Fatal(id, err)
	}
	if _, err = s.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<9999) INSERT INTO browser_requests SELECT 'limit-'||x,'hash','job' FROM n`); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveBrowser(ctx, j, "overflow", "hash"); err == nil {
		t.Fatal("unbounded receipts")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveBrowser(ctx, j, "closed", "hash"); err == nil {
		t.Fatal("closed write")
	}
}
