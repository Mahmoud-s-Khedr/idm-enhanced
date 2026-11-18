package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// staged inspects a disposable index snapshot, preserving both index and worktree.
func staged() error {
	b, e := output("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z")
	if e != nil {
		return e
	}
	if len(b) == 0 {
		fmt.Println("No staged files")
		return nil
	}
	if e = command("git", "diff", "--cached", "--check"); e != nil {
		return e
	}
	dir, e := os.MkdirTemp("", "idm-index-")
	if e != nil {
		return e
	}
	defer cleanup(dir)
	if e = command("git", "checkout-index", "--all", "--prefix="+dir+string(os.PathSeparator)); e != nil {
		return e
	}
	// Assets are generated and absent from the index. This lint-only placeholder
	// satisfies go:embed without reading unstaged frontend source or build output.
	assetDir := filepath.Join(dir, "frontend", "dist")
	if e = os.MkdirAll(assetDir, 0755); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(assetDir, "index.html"), []byte("<!doctype html>"), 0600); e != nil {
		return e
	}
	var goFiles, tsFiles, formatFiles []string
	for _, p := range strings.Split(strings.TrimRight(string(b), "\x00"), "\x00") {
		if strings.HasSuffix(p, ".go") {
			goFiles = append(goFiles, p)
		}
		// MPEG-TS fixtures share TypeScript's suffix but contain binary media.
		mediaFixture := strings.HasPrefix(p, "testdata/hls/") && strings.HasSuffix(p, ".ts")
		if !mediaFixture && !strings.HasPrefix(p, "frontend/bindings/") && (strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".tsx") || strings.HasSuffix(p, ".mjs") || strings.HasSuffix(p, ".cjs") || strings.HasSuffix(p, ".js")) {
			tsFiles = append(tsFiles, p)
		}
		formatFiles = append(formatFiles, p)
	}
	run := func(name string, args ...string) error {
		c := exec.Command(name, args...)
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}
	if len(goFiles) > 0 {
		c := exec.Command("gofmt", append([]string{"-l"}, goFiles...)...)
		c.Dir = dir
		b, e = c.Output()
		if e != nil {
			return e
		}
		if len(b) > 0 {
			return fmt.Errorf("staged Go formatting required: %s", b)
		}
	}
	nodeBin := filepath.Join(root, "node_modules", ".bin")
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".cmd"
	}
	if e = run(filepath.Join(nodeBin, "prettier"+ext), append([]string{"--check", "--ignore-unknown"}, formatFiles...)...); e != nil {
		return e
	}
	// Keep dependency resolution in the snapshot; no install or rewrite is allowed.
	if e = os.Symlink(filepath.Join(root, "node_modules"), filepath.Join(dir, "node_modules")); e != nil {
		return e
	}
	if len(tsFiles) > 0 {
		if e = run(filepath.Join(nodeBin, "eslint"+ext), append([]string{"--max-warnings=0"}, tsFiles...)...); e != nil {
			return e
		}
	}
	if len(goFiles) > 0 {
		if e = run(binary("golangci-lint"), "run", "./..."); e != nil {
			return e
		}
	}
	return run(binary("gitleaks"), "dir", ".", "--redact", "--no-banner")
}
func registration(remove bool) error {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	action := "--register"
	if remove {
		action = "--unregister"
	}
	return command(filepath.Join(root, "build", "native-host"+suffix), action)
}
