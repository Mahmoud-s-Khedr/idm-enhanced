// Package nativehost owns native messaging installation, separate from framing.
package nativehost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/extensions"
)

const Name = "com.idmenhanced.desktop"

type target struct {
	path    string
	firefox bool
	key     string
}

func targets(platform, home, config string) []target {
	switch platform {
	case "windows":
		base := filepath.Join(config, "idm-enhanced", "native-hosts")
		return []target{{filepath.Join(base, "firefox.json"), true, `Software\Mozilla\NativeMessagingHosts\` + Name}, {filepath.Join(base, "chrome.json"), false, `Software\Google\Chrome\NativeMessagingHosts\` + Name}, {filepath.Join(base, "chromium.json"), false, `Software\Chromium\NativeMessagingHosts\` + Name}}
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		return []target{{filepath.Join(base, "Mozilla", "NativeMessagingHosts", Name+".json"), true, ""}, {filepath.Join(base, "Google", "Chrome", "NativeMessagingHosts", Name+".json"), false, ""}, {filepath.Join(base, "Chromium", "NativeMessagingHosts", Name+".json"), false, ""}}
	default:
		return []target{{filepath.Join(home, ".mozilla", "native-messaging-hosts", Name+".json"), true, ""}, {filepath.Join(config, "google-chrome", "NativeMessagingHosts", Name+".json"), false, ""}, {filepath.Join(config, "chromium", "NativeMessagingHosts", Name+".json"), false, ""}}
	}
}
func manifest(host string, firefox bool) ([]byte, error) {
	var ids struct{ Firefox, Chromium string }
	if err := json.Unmarshal(extensions.Identities, &ids); err != nil {
		return nil, err
	}
	m := map[string]any{"name": Name, "description": "IDM Enhanced browser download bridge", "path": host, "type": "stdio"}
	if firefox {
		m["allowed_extensions"] = []string{ids.Firefox}
	} else {
		m["allowed_origins"] = []string{"chrome-extension://" + ids.Chromium + "/"}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	return append(b, '\n'), err
}

// Register affects only this user's registrations. A different executable's
// registration is never overwritten or removed. Repeated operations are safe.
func Register(host string, remove bool) error {
	host, err := filepath.Abs(host)
	if err != nil {
		return err
	}
	if !remove {
		if info, err := os.Stat(host); err != nil || !info.Mode().IsRegular() {
			return errors.New("native host executable is missing")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	return apply(targets(runtime.GOOS, home, config), host, remove)
}
func apply(items []target, host string, remove bool) error {
	// Preflight every manifest before changing any registration.
	for _, t := range items {
		old, err := os.ReadFile(t.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var current struct{ Name, Path string }
		if json.Unmarshal(old, &current) != nil || current.Name != Name || current.Path != host {
			return fmt.Errorf("another native host owns %s", t.path)
		}
	}
	for _, t := range items {
		data, err := manifest(host, t.firefox)
		if err != nil {
			return err
		}
		if remove {
			if err := registryEntry(t.key, t.path, true); err != nil {
				return err
			}
			if err := os.Remove(t.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(t.path), 0700); err != nil {
				return err
			}
			old, err := os.ReadFile(t.path)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if !bytes.Equal(old, data) {
				if err := os.WriteFile(t.path, data, 0600); err != nil {
					return err
				}
			}
			if err := registryEntry(t.key, t.path, false); err != nil {
				return err
			}
		}
	}
	return nil
}
