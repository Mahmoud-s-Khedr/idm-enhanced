// Command quality owns the repository's validation entry points.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type tool struct {
	Module  string `json:"module"`
	Version string `json:"version"`
}
type versions struct {
	Go    string          `json:"go"`
	Node  string          `json:"node"`
	PNPM  string          `json:"pnpm"`
	Tools map[string]tool `json:"tools"`
}

var pins versions
var root string

func main() {
	var err error
	root, err = os.Getwd()
	must(err)
	data, err := os.ReadFile("tools/versions.json")
	must(err)
	must(json.Unmarshal(data, &pins))
	// Explicit bootstrap is the only command that may install tools.
	local := filepath.Join(root, ".tools", "bin")
	node := filepath.Join(root, ".tools", "node-v"+pins.Node+"-"+runtime.GOOS+"-"+nodeArch(), "bin")
	must(os.Setenv("PATH", strings.Join([]string{node, filepath.Join(root, ".tools", "pnpm", "node_modules", ".bin"), local, os.Getenv("PATH")}, string(os.PathListSeparator))))
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "bootstrap" && args[0] != "bootstrap-deps" {
		must(os.Setenv("GOPROXY", "off"))
		must(os.Setenv("GOTOOLCHAIN", "local"))
	}

	if len(args) == 0 {
		fatal(errors.New("usage: quality doctor|bootstrap|check|check:full|task <name>|gate-probes"))
	}
	switch args[0] {
	case "doctor":
		err = doctor()
	case "bootstrap":
		err = bootstrap()
	case "bootstrap-deps":
		err = bootstrapDeps()
	case "check", "check:full":
		names := []string{"versions", "build", "format", "lint-go", "lint-ts", "types", "deadcode", "architecture", "clones", "test-clones", "mod-drift", "test-go", "test-tools", "test-ts", "bindings", "budgets"}
		if args[0] == "check:full" {
			names = append(names, "race", "integration", "media-smoke", "secrets", "vuln")
		}
		err = sequence(names)
	case "task":
		if len(args) != 2 {
			err = errors.New("task requires exactly one name")
		} else {
			err = task(args[1])
		}
	case "gate-probes":
		err = probes()
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fatal(err)
	}
}
func must(err error) {
	if err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "FAIL:", err); os.Exit(1) }
func nodeArch() string {
	if runtime.GOARCH == "amd64" {
		return "x64"
	}
	return runtime.GOARCH
}
func command(name string, args ...string) error {
	fmt.Println("+", name, strings.Join(args, " "))
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}
func output(name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Stderr = os.Stderr
	return c.Output()
}

// goTest rejects accidentally empty required suites before running them.
func goTest(args ...string) error {
	listArgs := append([]string{"test", "-vet=off", "-list", "^(Test|Example|Fuzz)"}, args...)
	b, err := output("go", listArgs...)
	if err != nil {
		return err
	}
	count := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Example") || strings.HasPrefix(line, "Fuzz") {
			count++
		}
	}
	if count == 0 {
		return errors.New("required Go test suite is empty")
	}
	fmt.Printf("Required Go tests discovered: %d\n", count)
	return command("go", append([]string{"test", "-vet=off"}, args...)...)
}
func sequence(names []string) error {
	for _, n := range names {
		fmt.Println("TASK", n)
		if err := task(n); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
	}
	return nil
}
func binary(n string) string {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	return filepath.Join(root, ".tools", "bin", n+suffix)
}
func pnpm(args ...string) error {
	// Run installed binaries directly: pnpm exec may auto-install when a disposable
	// snapshot has a different workspace path, violating offline gate semantics.
	if len(args) > 1 && args[0] == "exec" {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".cmd"
		}
		return command(filepath.Join(root, "node_modules", ".bin", args[1]+ext), args[2:]...)
	}

	return command(pnpmExecutable(), args...)
}
func pnpmExecutable() string {
	if runtime.GOOS == "windows" {
		return "pnpm.cmd"
	}
	return "pnpm"
}
func task(n string) error {
	switch n {
	case "versions":
		return checkVersions()
	case "format":
		if err := goFormat(); err != nil {
			return err
		}
		return pnpm("exec", "prettier", "--check", ".")
	case "lint-go":
		return command(binary("golangci-lint"), "run", "./...")
	case "lint-ts":
		return pnpm("exec", "eslint", ".", "--max-warnings=0")
	case "types":
		return pnpm("exec", "tsc", "-p", "tsconfig.json")
	case "deadcode":
		return pnpm("exec", "knip")
	case "architecture":
		return pnpm("exec", "depcruise", "--config", ".dependency-cruiser.cjs", "frontend/src", "extensions/shared/src")
	case "clones":
		return pnpm("exec", "jscpd", "frontend/src", "extensions/shared/src")
	case "test-clones":
		if err := command(binary("golangci-lint"), "run", "--config", ".golangci-tests.yml", "--issues-exit-code=0", "./..."); err != nil {
			return err
		}
		return pnpm("exec", "jscpd", "--config", ".jscpd-tests.json", "frontend/src", "extensions/shared/src")
	case "mod-drift":
		return modDrift()
	case "test-go":
		if err := goTest("-count=1", "-coverprofile=coverage.go.prof", "./internal/..."); err != nil {
			return err
		}
		return coverage()
	case "test-tools":
		return goTest("-count=1", "./tools/...")
	case "test-ts":
		return pnpm("exec", "vitest", "run", "--coverage")
	case "race":
		if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			return fmt.Errorf("race detector unsupported on %s/%s; require a supported CI runner", runtime.GOOS, runtime.GOARCH)
		}
		return goTest("-race", "-count=1", "./internal/...", "./tools/...")
	case "integration":
		return goTest("-count=1", "-tags=integration", "./integration/...")
	case "media-smoke":
		for _, tool := range []string{"ffmpeg", "ffprobe"} {
			b, e := output(tool, "-version")
			if e != nil {
				return e
			}
			fmt.Println(strings.SplitN(string(b), "\n", 2)[0])
		}
		return goTest("-count=1", "-tags=media", "./integration/media")
	case "bindings":
		if err := command("go", "run", "./tools/contract", "-check"); err != nil {
			return err
		}
		return bindings(false)
	case "generate":
		return bindings(true)
	case "build":
		if err := pnpm("exec", "vite", "build", "--config", "vite.config.ts"); err != nil {
			return err
		}
		if err := pnpm("exec", "vite", "build", "--config", "vite.extensions.config.ts"); err != nil {
			return err
		}
		if err := command("go", "build", "-trimpath", "-o", "build/", "./cmd/..."); err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			return command("go", "build", "-trimpath", "-ldflags=-H=windowsgui", "-o", "build/desktop.exe", "./cmd/desktop")
		}
		return nil
	case "budgets":
		return budgets()
	case "secrets":
		return command(binary("gitleaks"), "dir", ".", "--redact", "--no-banner")
	case "history-secrets":
		return command(binary("gitleaks"), "git", ".", "--redact", "--no-banner")
	case "vuln":
		if err := command(binary("govulncheck"), "./..."); err != nil {
			return err
		}
		if runtime.GOOS != "windows" {
			fmt.Println("+ govulncheck Windows target ./...")
			c := exec.Command(binary("govulncheck"), "./...")
			c.Env = append(os.Environ(), "GOOS=windows", "CGO_ENABLED=0")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
		}
		return javascriptAudit()
	case "staged":
		return staged()
	case "package":
		format := os.Getenv("PACKAGE_FORMAT")
		if format == "" {
			format = runtime.GOOS
		}
		return command("go", "run", "./tools/package", "-format", format)
	case "desktop-smoke":
		return command("node", "tools/desktop-smoke.mjs")
	case "browser-smoke":
		return command("node", "tools/browser-smoke.mjs")
	case "platform-info":
		fmt.Printf("Native runner: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		if err := command("go", "env", "GOOS", "GOARCH", "GOVERSION"); err != nil {
			return err
		}
		if err := command("node", "--version"); err != nil {
			return err
		}
		if err := pnpm("--version"); err != nil {
			return err
		}
		if err := command("go", "list", "-m", "github.com/wailsapp/wails/v3"); err != nil {
			return err
		}
		switch runtime.GOOS {
		case "linux":
			return command("pkg-config", "--modversion", "gtk4", "webkitgtk-6.0")
		case "darwin":
			if err := command("sw_vers"); err != nil {
				return err
			}
			return command("xcrun", "--show-sdk-version")
		case "windows":
			return command("cmd", "/c", "ver")
		}
		return fmt.Errorf("unsupported native platform %s", runtime.GOOS)
	case "register":
		return registration(false)
	case "unregister":
		return registration(true)
	case "aggregate":
		return aggregate(os.Getenv("QUALITY_NEEDS"))
	default:
		return fmt.Errorf("unknown task %q", n)
	}
}
func checkVersions() error {
	checks := []struct {
		name string
		args []string
		want string
	}{{"go", []string{"version"}, "go" + pins.Go}, {"node", []string{"--version"}, "v" + pins.Node}, {pnpmExecutable(), []string{"--version"}, pins.PNPM}}
	for _, c := range checks {
		b, e := output(c.name, c.args...)
		if e != nil {
			return e
		}
		got := strings.TrimSpace(string(b))
		if c.name == "go" {
			f := strings.Fields(got)
			if len(f) < 3 {
				return errors.New("unrecognized Go version output")
			}
			got = strings.SplitN(f[2], "-", 2)[0]
		}
		if got != c.want {
			return fmt.Errorf("%s: expected %s, got %s", c.name, c.want, b)
		}
	}
	for n, p := range pins.Tools {
		args := []string{"--version"}
		if n == "wails3" {
			args = []string{"version"}
		}
		if n == "gitleaks" {
			args = []string{"version"}
		}
		b, e := exec.Command(binary(n), args...).CombinedOutput()
		if e != nil {
			return e
		}
		if !hasVersion(string(b), p.Version) {
			return fmt.Errorf("%s expected %s, got %s", n, p.Version, b)
		}
	}
	b, e := output("go", "list", "-m", "-f", "{{.Version}}", "github.com/wailsapp/wails/v3")
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(b)) != pins.Tools["wails3"].Version {
		return errors.New("wails library and CLI pins must match")
	}
	b, e = os.ReadFile("frontend/package.json")
	if e != nil {
		return e
	}
	var frontend struct{ Dependencies map[string]string }
	if e = json.Unmarshal(b, &frontend); e != nil {
		return e
	}
	if frontend.Dependencies["@wailsio/runtime"] != strings.TrimPrefix(pins.Tools["wails3"].Version, "v") {
		return errors.New("wails runtime and CLI pins must match")
	}
	return nil
}

var semver = regexp.MustCompile(`v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?`)

func hasVersion(output, want string) bool {
	for _, v := range semver.FindAllString(output, -1) {
		if strings.TrimPrefix(v, "v") == strings.TrimPrefix(want, "v") {
			return true
		}
	}
	return false
}
func doctor() error {
	fmt.Printf("Platform: %s/%s\nPins: Go %s, Node %s, pnpm %s\n", runtime.GOOS, runtime.GOARCH, pins.Go, pins.Node, pins.PNPM)
	versionErr := checkVersions()
	if versionErr != nil {
		fmt.Println("Tool diagnosis:", versionErr)
	}
	var nativeErr error
	if runtime.GOOS == "linux" {
		nativeErr = command("pkg-config", "--modversion", "gtk4", "webkitgtk-6.0")
		for _, b := range []string{"firefox", "chromium-browser", "google-chrome"} {
			p, e := exec.LookPath(b)
			fmt.Printf("Browser %s: %s (%v)\n", b, p, e)
		}
	} else {
		fmt.Println("Native messaging registration and local IPC available; run browser smoke on a supported browser installation.")
	}
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		p, e := exec.LookPath(tool)
		fmt.Printf("Media runtime %s (required for media-smoke): %s (%v)\n", tool, p, e)
	}
	return errors.Join(versionErr, nativeErr)
}
func bootstrap() error {
	if strings.SplitN(runtime.Version(), "-", 2)[0] != "go"+pins.Go {
		return fmt.Errorf("install Go %s first", pins.Go)
	}
	if err := os.MkdirAll(filepath.Join(root, ".tools", "bin"), 0755); err != nil {
		return err
	}
	for n, p := range pins.Tools {
		args := []string{"install"}
		if n == "gitleaks" {
			args = append(args, "-ldflags=-X github.com/zricethezav/gitleaks/v8/version.Version="+p.Version)
		}
		args = append(args, p.Module+"@"+p.Version)
		c := exec.Command("go", args...)
		c.Env = append(os.Environ(), "GOBIN="+filepath.Join(root, ".tools", "bin"))
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		fmt.Println("Install", n, p.Version)
		if err := c.Run(); err != nil {
			return err
		}
	}
	if err := bootstrapDeps(); err != nil {
		return err
	}
	return pnpm("exec", "lefthook", "install")
}
func goFormat() error {
	var files []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".tools" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".go") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("go source suite is empty")
	}
	b, e := output("gofmt", append([]string{"-l"}, files...)...)
	if e != nil {
		return e
	}
	if len(b) > 0 {
		return fmt.Errorf("gofmt required:\n%s", b)
	}
	return nil
}
func modDrift() error {
	// -diff reports proposed changes and never rewrites the manifests.
	return command("go", "mod", "tidy", "-diff")
}

func coverage() error {
	b, e := os.ReadFile("coverage.go.prof")
	if e != nil {
		return e
	}
	type count struct{ total, covered int }
	counts := map[string]count{}
	for _, line := range strings.Split(string(b), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		n, e := strconv.Atoi(f[1])
		if e != nil {
			return e
		}
		hits, e := strconv.Atoi(f[2])
		if e != nil {
			return e
		}
		pkg := filepath.ToSlash(filepath.Dir(strings.Split(f[0], ":")[0]))
		c := counts[pkg]
		c.total += n
		if hits > 0 {
			c.covered += n
		}
		counts[pkg] = c
	}
	pkgs, e := output("go", "list", "./internal/...")
	if e != nil {
		return e
	}
	if len(strings.Fields(string(pkgs))) == 0 {
		return errors.New("required Go logic suite empty")
	}
	for _, p := range strings.Fields(string(pkgs)) {

		c := counts[p]
		if c.total == 0 {
			return fmt.Errorf("%s has no measured statements", p)
		}
		pct := 100 * float64(c.covered) / float64(c.total)
		fmt.Printf("%s %.1f%%\n", p, pct)
		minimum := 80
		for _, critical := range []string{"/internal/engine", "/internal/manager", "/internal/storage", "/internal/media"} {
			if strings.HasSuffix(p, critical) {
				minimum = 90
			}
		}
		if pct < float64(minimum) {
			return fmt.Errorf("%s below %d%%", p, minimum)
		}
	}
	return nil
}
func bindings(write bool) error {
	dest := "frontend/bindings"
	if !write {
		d, e := os.MkdirTemp("", "idm-bindings-")
		if e != nil {
			return e
		}
		defer cleanup(d)
		dest = d
	}
	if e := command(binary("wails3"), "generate", "bindings", "-ts", "-noevents", "-d", dest, "./cmd/desktop"); e != nil {
		return e
	}
	if write {
		return nil
	}
	a, e := tree("frontend/bindings")
	if e != nil {
		return e
	}
	b, e := tree(dest)
	if e != nil {
		return e
	}
	if len(a) == 0 || len(a) != len(b) {
		return errors.New("generated binding file set drift; run task generate")
	}
	for p, v := range a {
		if !bytes.Equal(v, b[p]) {
			return fmt.Errorf("binding drift %s; run task generate", p)
		}
	}
	return nil
}
func tree(dir string) (map[string][]byte, error) {
	m := map[string][]byte{}
	e := filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			r, e := filepath.Rel(dir, p)
			if e != nil {
				return e
			}
			m[r] = b
		}
		return nil
	})
	return m, e
}

type budget struct {
	Raw  int `json:"raw"`
	Gzip int `json:"gzip"`
}

func budgets() error {
	b, e := os.ReadFile("tools/budgets.json")
	if e != nil {
		return e
	}
	var limits map[string]budget
	if e = json.Unmarshal(b, &limits); e != nil {
		return e
	}
	if len(limits) != 3 {
		return errors.New("desktop, Firefox, Chromium budgets required")
	}
	keys := make([]string, 0, len(limits))
	for k := range limits {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, dir := range keys {
		files, e := tree(dir)
		if e != nil {
			return e
		}
		raw, zipped, n := 0, 0, 0
		for p, b := range files {
			ext := filepath.Ext(p)
			if ext != ".js" && ext != ".css" {
				continue
			}
			n++
			raw += len(b)
			var z bytes.Buffer
			g := gzip.NewWriter(&z)
			if _, e = g.Write(b); e != nil {
				return e
			}
			if e = g.Close(); e != nil {
				return e
			}
			zipped += z.Len()
		}
		if n == 0 {
			return fmt.Errorf("%s contains no application assets", dir)
		}
		v := limits[dir]
		fmt.Printf("%s raw=%d/%d gzip=%d/%d\n", dir, raw, v.Raw, zipped, v.Gzip)
		if raw > v.Raw || zipped > v.Gzip {
			return fmt.Errorf("bundle budget exceeded: %s", dir)
		}
	}
	return nil
}
func aggregate(s string) error {
	var needs map[string]struct {
		Result string `json:"result"`
	}
	if e := json.Unmarshal([]byte(s), &needs); e != nil {
		return e
	}
	for _, name := range []string{"quality", "native", "linux-packages"} {
		n, ok := needs[name]
		if !ok || n.Result != "success" {
			return fmt.Errorf("mandatory job %s: %q", name, n.Result)
		}
	}
	if len(needs) != 3 {
		return errors.New("unexpected CI dependency set")
	}
	return nil
}

func cleanup(path string) {
	if e := os.RemoveAll(path); e != nil {
		fmt.Fprintln(os.Stderr, "Temporary cleanup failed:", e)
	}
}

// bootstrapDeps is the explicit network setup used by native-build jobs.
func bootstrapDeps() error {
	// Prefetch the complete pinned module graph, including dependency tests used
	// by tidy. A temporary modfile keeps download-only sums out of the source lock.
	dir, err := os.MkdirTemp("", "idm-bootstrap-")
	if err != nil {
		return err
	}
	defer cleanup(dir)
	for _, name := range []string{"go.mod", "go.sum"} {
		b, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			return e
		}
	}
	if err := command("go", "mod", "download", "-modfile="+filepath.Join(dir, "go.mod"), "all"); err != nil {
		return err
	}
	return pnpm("install", "--frozen-lockfile")
}
