package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// probes never mutates the caller's working tree or index.
func probes() error {
	if e := os.MkdirAll("build", 0755); e != nil {
		return e
	}
	binaryPath := filepath.Join(root, "build", "quality-probe")
	if e := command("go", "build", "-o", binaryPath, "./tools/quality"); e != nil {
		return e
	}
	dir, e := os.MkdirTemp("", "idm-probes-")
	if e != nil {
		return e
	}
	defer cleanup(dir)
	e = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "build" || d.Name() == "coverage" {
				return filepath.SkipDir
			}
			if d.Name() == "node_modules" || d.Name() == ".tools" {
				if e = os.Symlink(p, target); e != nil {
					return e
				}
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return os.Symlink(p, target)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(target, b, 0644)
	})
	if e != nil {
		return e
	}
	run := func(args ...string) ([]byte, error) {
		c := exec.Command(binaryPath, args...)
		c.Dir = dir
		return c.CombinedOutput()
	}
	// A failing clean baseline must never count as proof of a working gate.
	gates := []string{"build", "format", "lint-go", "lint-ts", "types", "architecture", "clones", "deadcode", "mod-drift", "test-go", "test-ts", "bindings", "budgets", "secrets"}
	for _, g := range gates {
		b, e := run("task", g)
		if e != nil {
			return fmt.Errorf("probe baseline %s failed: %s", g, b)
		}
	}
	type probe struct{ name, task, path, append string }
	sample := ""
	for i := range 24 {
		sample += fmt.Sprintf("  values.push(input + %d);\n", i)
	}
	probes := []probe{
		{"format", "format", "internal/protocol/protocol.go", "\nfunc unformatted( ) { }\n"},
		{"lint", "lint-ts", "extensions/shared/src/connection.ts", "\nconst unusedProbe = 1;\n"},
		{"types", "types", "extensions/shared/src/connection.ts", "\nconst typeProbe: number = 'wrong'; void typeProbe;\n"},
		{"ts-boundary", "architecture", "extensions/shared/src/connection.ts", "\nimport '../../../frontend/src/App';\n"},
		{"ts-clone", "clones", "extensions/shared/src/clone-probe.ts", "export function first(input: number) { const values: number[] = [];\n" + sample + "return values; }\nexport function second(input: number) { const values: number[] = [];\n" + sample + "return values; }\n"},
		{"unused-dependency", "deadcode", "package.json", ""},
		{"manifest-drift", "mod-drift", "go.mod", "\nrequire example.com/unused v1.0.0\n"},
		{"secret", "secrets", "secret-probe.txt", ""},
		{"go-coverage", "test-go", "internal/protocol/protocol.go", "\nfunc UncoveredProbe(x int) int {\n" + uncovered() + "return x\n}\n"},
		{"ts-coverage", "test-ts", "extensions/shared/src/connection.ts", "\nexport function uncoveredProbe(x: number) {\n" + strings.ReplaceAll(strings.ReplaceAll(uncovered(), "if x == ", "if (x === "), " { x = ", ") { x = ") + "return x;\n}\n"},
		{"bundle", "budgets", "frontend/dist/probe.js", strings.Repeat("x", 1024*1024)},
		{"bindings", "bindings", "internal/desktop/service.go", "\nfunc(s *Service) NewContractProbe() string { return \"drift\" }\n"},
	}
	var report strings.Builder
	var details strings.Builder
	expected := map[string]string{"format": "gofmt required", "lint": "no-unused-vars", "types": "TS2322", "ts-boundary": "extension-no-frontend", "ts-clone": "clones", "unused-dependency": "unused-probe", "manifest-drift": "example.com/unused", "secret": "leaks found", "go-coverage": "below 80%", "ts-coverage": "Coverage", "bundle": "bundle budget exceeded", "bindings": "binding drift"}

	for _, p := range probes {
		path := filepath.Join(dir, p.path)
		old, readErr := os.ReadFile(path)
		data := append(append([]byte(nil), old...), []byte(p.append)...)
		if p.name == "unused-dependency" {
			data = bytes.Replace(old, []byte(`"devDependencies": {`), []byte(`"devDependencies": {"unused-probe": "1.0.0",`), 1)
		}
		if p.name == "secret" {
			data = []byte("synthetic_test_token = " + "ghp_" + "q8R3yN7mV2xP9kL5sT4wB6cF1dH0jZ8uA3eC\n")
		}
		if e = os.WriteFile(path, data, 0644); e != nil {
			return e
		}
		b, probeErr := run("task", p.task)
		if readErr == nil {
			e = os.WriteFile(path, old, 0644)
		} else {
			e = os.Remove(path)
		}
		if e != nil {
			return e
		}
		fmt.Fprintf(&details, "PROBE %s\n%s\n", p.name, b)
		if e = os.WriteFile("build/gate-probe-details.log", []byte(details.String()), 0644); e != nil {
			return e
		}
		if probeErr == nil {
			return fmt.Errorf("probe %s was accepted", p.name)
		}
		if !strings.Contains(strings.ToLower(string(b)), strings.ToLower(expected[p.name])) {
			return fmt.Errorf("probe %s failed for unexpected reason: %s", p.name, b)
		}
		fmt.Fprintln(&report, "PASS rejected", p.name)
		fmt.Printf("PASS rejected %s\n", p.name)
		if len(b) == 0 {
			return fmt.Errorf("probe %s failed without diagnostics", p.name)
		}
	}
	// Go depguard and dupl must reject independently of type errors.
	for _, mode := range []string{"boundary", "clone"} {
		path := filepath.Join(dir, "internal/engine/probe.go")
		if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			return e
		}
		s := "package engine\n"
		if mode == "boundary" {
			s += "import _ \"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop\"\n"
		} else {
			body := ""
			for i := range 30 {
				body += fmt.Sprintf("x += %d\n", i)
			}
			s += "func One(x int)int{\n" + body + "return x\n}\nfunc Two(x int)int{\n" + body + "return x\n}\n"
		}
		if e = os.WriteFile(path, []byte(s), 0644); e != nil {
			return e
		}
		b, err := run("task", "lint-go")
		if e = os.RemoveAll(filepath.Dir(path)); e != nil {
			return e
		}
		if err == nil || !bytes.Contains(b, []byte(map[string]string{"boundary": "depguard", "clone": "dupl"}[mode])) {
			return fmt.Errorf("go %s probe not rejected by intended analyzer: %s", mode, b)
		}
		fmt.Fprintln(&report, "PASS rejected go-"+mode)
	}
	for _, result := range []string{"failure", "cancelled", "skipped", ""} {
		c := exec.Command(binaryPath, "task", "aggregate")
		c.Dir = dir
		c.Env = append(os.Environ(), `QUALITY_NEEDS={"quality":{"result":"success"},"native":{"result":"`+result+`"}}`)
		if e = c.Run(); e == nil {
			return errors.New("CI aggregate accepted non-success")
		}
	}
	fmt.Fprintln(&report, "PASS CI aggregation rejects failed/cancelled/skipped/missing results")
	if e = probeStaged(dir, binaryPath); e != nil {
		return e
	}
	fmt.Fprintln(&report, "PASS partially staged rejection preserves index and worktree")
	return os.WriteFile("build/gate-probes.txt", []byte(report.String()), 0644)
}
func uncovered() string {
	s := ""
	for i := range 150 {
		s += fmt.Sprintf("if x == %d { x = %d };\n", i, i+1)
	}
	return s
}
func probeStaged(dir, bin string) error {
	git := func(args ...string) ([]byte, error) {
		c := exec.Command("git", args...)
		c.Dir = dir
		return c.CombinedOutput()
	}
	if b, e := git("init", "-b", "main"); e != nil {
		return fmt.Errorf("git init: %s", b)
	}
	// Ignore linked dependency/tool directories; only index source files.
	if b, e := git("add", "."); e != nil {
		return fmt.Errorf("git add: %s", b)
	}
	path := filepath.Join(dir, "internal/protocol/protocol.go")
	before, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if e = os.WriteFile(path, append(before, []byte("\nfunc stagedBad( ) {}\n")...), 0644); e != nil {
		return e
	}
	if _, e = git("add", "internal/protocol/protocol.go"); e != nil {
		return e
	}
	if e = os.WriteFile(path, append(before, []byte("\n// unstaged content\n")...), 0644); e != nil {
		return e
	}
	index, e := git("write-tree")
	if e != nil {
		return e
	}
	work, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	c := exec.Command(bin, "task", "staged")
	c.Dir = dir
	if e = c.Run(); e == nil {
		return errors.New("hook accepted malformed staged file")
	}
	after, e := git("write-tree")
	if e != nil {
		return e
	}
	actual, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if !bytes.Equal(index, after) || !bytes.Equal(work, actual) {
		return errors.New("hook mutated partially staged content")
	}
	return nil
}
