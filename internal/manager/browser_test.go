package manager

import (
	"context"
	"fmt"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBrowserDurableAdmission(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	downloads := t.TempDir()
	m, err := Open(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.Message{Version: 1, ID: "browser-1", Type: "download", URL: "https://example.invalid/file?secret=value", Filename: "file.bin"}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for range 8 {
		wg.Go(func() {
			id, e := m.AcceptBrowser(ctx, request, downloads)
			if e != nil {
				t.Error(e)
			}
			ids <- id
		})
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("duplicate job")
		}
	}
	jobs, err := m.Status(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].State != "queued" {
		t.Fatal(jobs, err)
	}
	changed := request
	changed.URL = "https://example.invalid/other"
	if _, err = m.AcceptBrowser(ctx, changed, downloads); err == nil {
		t.Fatal("ID conflict accepted")
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = Open(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if id, err := m.AcceptBrowser(ctx, request, downloads); err != nil || id != first {
		t.Fatal(id, err)
	}
	if err = m.Cancel(ctx, first, false); err != nil {
		t.Fatal(err)
	}
	if err = m.Remove(ctx, first); err != nil {
		t.Fatal(err)
	}
	if id, err := m.AcceptBrowser(ctx, request, downloads); err != nil || id != first {
		t.Fatal("removed receipt lost", id, err)
	}
	if err = os.WriteFile(filepath.Join(downloads, "file.bin"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	request.ID = "browser-2"
	id, err := m.AcceptBrowser(ctx, request, downloads)
	if err != nil {
		t.Fatal(err)
	}
	j, err := m.store.Get(ctx, id)
	if err != nil || filepath.Base(j.Destination) != "file (1).bin" {
		t.Fatal(j, err)
	}
	request.ID = "browser-3"
	if _, err = m.AcceptBrowser(ctx, request, downloads); err != nil {
		t.Fatal(err)
	}
	for _, r := range []protocol.Message{{}, {Version: 1, ID: "ping", Type: "ping"}} {
		if _, err = m.AcceptBrowser(ctx, r, downloads); err == nil {
			t.Fatal("invalid admission")
		}
	}
	request.ID = "relative"
	if _, err = m.AcceptBrowser(ctx, request, "relative"); err == nil {
		t.Fatal("relative directory")
	}
	if _, err = m.AcceptBrowser(ctx, request, filepath.Join(downloads, "missing")); err == nil {
		t.Fatal("missing directory")
	}
	for n := 0; n < 100; n++ {
		name := "file.bin"
		if n > 0 {
			name = fmt.Sprintf("file (%d).bin", n)
		}
		if err = os.WriteFile(filepath.Join(downloads, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = m.AcceptBrowser(ctx, request, downloads); err == nil {
		t.Fatal("unbounded collisions")
	}
}
func TestBrowserSessionLifecycle(t *testing.T) {
	m, err := Open(t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := protocol.Message{Version: 1, ID: "s", Type: "download", URL: "https://example.invalid/a", Filename: "a"}
	dir := t.TempDir()
	if _, err = s.AcceptBrowser(r, dir); err != nil {
		t.Fatal(err)
	}
	s.Sleep()
	if _, err = s.AcceptBrowser(r, dir); err == nil {
		t.Fatal("sleep admission")
	}
	s.Wake()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcceptBrowser(r, dir); err == nil {
		t.Fatal("closed admission")
	}
	if _, err = m.AcceptBrowser(context.Background(), r, dir); err == nil {
		t.Fatal("closed store")
	}
}

func TestBrowserPortableDestinationReservation(t *testing.T) {
	m, err := Open(t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	ctx := context.Background()
	dir := t.TempDir()
	for _, pair := range [][2]string{{"File.zip", "file.zip"}, {"é.zip", "e\u0301.zip"}} {
		request := protocol.Message{Version: 1, ID: "first-" + fmt.Sprint(len(pair[0])), Type: "download", URL: "https://example.invalid/file", Filename: pair[0]}
		if _, err := m.AcceptBrowser(ctx, request, dir); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Create(ctx, Options{URL: request.URL, Destination: filepath.Join(dir, pair[1])}); err == nil {
			t.Fatal("desktop/CLI bypassed reservation")
		}
		request.ID = "second-" + fmt.Sprint(len(pair[0]))
		request.Filename = pair[1]
		id, err := m.AcceptBrowser(ctx, request, dir)
		if err != nil {
			t.Fatal(err)
		}
		j, err := m.store.Get(ctx, id)
		if err != nil || !strings.Contains(j.Destination, " (1).zip") {
			t.Fatal(j, err)
		}
	}
}

func TestBrowserHLSAdmissionKeepsFileReceiptCompatibilityAndModeIdentity(t *testing.T) {
	ctx := context.Background()
	m, err := Open(t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	directory := t.TempDir()
	request := protocol.Message{Version: 1, ID: "hls-request", Type: "download", URL: "https://example.invalid/master.m3u8", Filename: "movie.m3u8", Mode: "hls"}
	first, err := m.AcceptBrowser(ctx, request, directory)
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.AcceptBrowser(ctx, request, directory)
	if err != nil || again != first {
		t.Fatal("media retry duplicated", again, err)
	}
	jobs, err := m.Status(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].Kind != "hls" || filepath.Base(jobs[0].Destination) != "movie.mp4" {
		t.Fatal(jobs, err)
	}
	request.Mode = ""
	if _, err = m.AcceptBrowser(ctx, request, directory); err == nil {
		t.Fatal("same ID switched from media to file")
	}
	request.Mode = "hls"
	request.ID = "collision"
	id, err := m.AcceptBrowser(ctx, request, directory)
	if err != nil {
		t.Fatal(err)
	}
	j, err := m.store.Get(ctx, id)
	if err != nil || filepath.Base(j.Destination) != "movie (1).mp4" {
		t.Fatal(j.Destination, err)
	}
	for index, name := range []string{".m3u8", strings.Repeat("x", 180)} {
		request.ID = fmt.Sprintf("fallback-%d", index)
		request.Filename = name
		if _, err = m.AcceptBrowser(ctx, request, directory); err != nil {
			t.Fatal(err)
		}
	}
}
