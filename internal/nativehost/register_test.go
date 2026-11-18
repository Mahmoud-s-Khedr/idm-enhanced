package nativehost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRegistration(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "native-host")
	if err := os.WriteFile(host, nil, 0700); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"linux", "darwin", "windows"} {
		items := targets(platform, filepath.Join(dir, platform), filepath.Join(dir, platform, "config"))
		// OS registry behavior is separately exercised on its native runner.
		for i := range items {
			items[i].key = ""
		}
		if err := apply(items, host, false); err != nil {
			t.Fatal(err)
		}
		if err := apply(items, host, false); err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			b, err := os.ReadFile(item.path)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err = json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			if m["path"] != host || m["name"] != Name {
				t.Fatal(m)
			}
		}
		if err := apply(items, host+"other", false); err == nil {
			t.Fatal("overwrote another install")
		}
		if err := apply(items, host+"other", true); err == nil {
			t.Fatal("removed another install")
		}
		if err := apply(items, host, true); err != nil {
			t.Fatal(err)
		}
		if err := apply(items, host, true); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(items[0].path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := apply(items, host, false); err == nil {
			t.Fatal("directory accepted as manifest")
		}
	}
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", dir)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
		if err := Register(host, false); err != nil {
			t.Fatal(err)
		}
		if err := Register(host, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := Register(filepath.Join(dir, "missing"), false); err == nil {
		t.Fatal("missing host")
	}
}
