// Command package assembles alpha installers from already-built binaries.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var version = "0.6.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	format := flag.String("format", runtime.GOOS, "deb, rpm, windows, or extensions")
	flag.StringVar(&version, "version", version, "numeric package version (also used by controlled upgrade fixtures)")
	destination := flag.String("out", "build/packages", "artifact output directory")
	flag.Parse()
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
		return fmt.Errorf("invalid package version")
	}
	out, err := filepath.Abs(*destination)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		return err
	}
	if err = extensions(out); err != nil {
		return err
	}
	switch *format {
	case "extensions":
	case "windows":
		root, err := os.Getwd()
		if err != nil {
			return err
		}
		compiler := os.Getenv("ISCC")
		if compiler == "" {
			compiler = `C:\Program Files (x86)\Inno Setup 6\ISCC.exe`
		}
		if err = command(compiler, "/DSourceRoot="+root, "/DAppVersion="+version, "/O"+out, "packaging/windows.iss"); err != nil {
			return err
		}
	case "deb", "rpm":
		if err = linuxPackage(out, *format); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported package format %s", *format)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	var sums strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SHA256SUMS" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(out, entry.Name()))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), entry.Name())
	}
	return os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(sums.String()), 0644)
}
func command(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
func extensions(out string) error {
	for _, name := range []string{"firefox", "chromium"} {
		suffix := "chrome.zip"
		if name == "firefox" {
			suffix = "firefox-unsigned.xpi"
		}
		path := filepath.Join(out, "idm-enhanced-"+version+"-"+suffix)
		if err := archive(filepath.Join("extensions", name, "dist"), path); err != nil {
			return err
		}
	}
	return nil
}
func archive(dir, dest string) (result error) {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); result == nil {
			result = err
		}
	}()
	w := zip.NewWriter(f)
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entry, err := w.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		_, err = entry.Write(b)
		return err
	})
	closeErr := w.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func put(root, path string, data []byte, mode fs.FileMode) error {
	dest := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, mode)
}
func linuxPackage(out, format string) error {
	work, err := os.MkdirTemp("", "idm-package-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	root := filepath.Join(work, "root")
	for _, name := range []string{"desktop", "native-host", "downloader"} {
		b, err := os.ReadFile(filepath.Join("build", name))
		if err != nil {
			return err
		}
		if err = put(root, "usr/lib/idm-enhanced/"+name, b, 0755); err != nil {
			return err
		}
	}
	files := map[string]string{
		"usr/bin/idm-enhanced":                        "#!/bin/sh\nexec /usr/lib/idm-enhanced/desktop \"$@\"\n",
		"usr/share/applications/idm-enhanced.desktop": "[Desktop Entry]\nType=Application\nName=IDM Enhanced\nExec=idm-enhanced\nIcon=folder-download\nTerminal=false\nCategories=Network;FileTransfer;\n",
	}
	for path, text := range files {
		mode := fs.FileMode(0644)
		if strings.HasPrefix(path, "usr/bin/") {
			mode = 0755
		}
		if err = put(root, path, []byte(text), mode); err != nil {
			return err
		}
	}
	for _, name := range []string{"LICENSE", "README.md"} {
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if err = put(root, "usr/share/doc/idm-enhanced/"+name, b, 0644); err != nil {
			return err
		}
	}
	if format == "deb" {
		control := "Package: idm-enhanced\nVersion: " + version + "\nArchitecture: amd64\nMaintainer: IDM Enhanced contributors\nSection: net\nPriority: optional\nDepends: libgtk-4-1, libwebkitgtk-6.0-4\nDescription: Local desktop HTTP download manager (alpha)\n"
		if err = put(root, "DEBIAN/control", []byte(control), 0644); err != nil {
			return err
		}
		return command("dpkg-deb", "--root-owner-group", "--build", root, filepath.Join(out, "idm-enhanced_"+version+"_amd64.deb"))
	}
	for _, dir := range []string{"BUILD", "RPMS", "SOURCES", "SPECS", "SRPMS", "BUILDROOT"} {
		if err = os.MkdirAll(filepath.Join(work, dir), 0755); err != nil {
			return err
		}
	}
	spec := fmt.Sprintf(`Name: idm-enhanced
Version: %s
Release: 1
Summary: Local desktop HTTP download manager (alpha)
License: MIT
Requires: gtk4, webkitgtk6.0
%%global debug_package %%{nil}
%%description
A local download manager with safe resume and explicit browser handoff.
%%install
mkdir -p %%{buildroot}
cp -a %s/usr %%{buildroot}/
%%files
/usr/bin/idm-enhanced
/usr/lib/idm-enhanced
/usr/share/applications/idm-enhanced.desktop
/usr/share/doc/idm-enhanced
`, version, root)
	path := filepath.Join(work, "SPECS", "idm.spec")
	if err = os.WriteFile(path, []byte(spec), 0644); err != nil {
		return err
	}
	if err = command("rpmbuild", "--define", "_topdir "+work, "--define", "_rpmdir "+out, "--define", "_rpmfilename %{NAME}-%{VERSION}-%{RELEASE}.%{ARCH}.rpm", "--target", "x86_64", "-bb", path); err != nil {
		return err
	}
	return nil
}
