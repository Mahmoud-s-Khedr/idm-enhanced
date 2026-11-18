package nativehost

import (
	"errors"
	"golang.org/x/sys/windows/registry"
)

func registryEntry(key, path string, remove bool) error {
	if key == "" {
		return nil
	}
	existing, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err == nil {
		value, _, readErr := existing.GetStringValue("")
		closeErr := existing.Close()
		if readErr != nil || closeErr != nil {
			return errors.Join(readErr, closeErr)
		}
		if value != path {
			return errors.New("another native host owns the registry entry")
		}
	} else if !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	if remove {
		err := registry.DeleteKey(registry.CURRENT_USER, key)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	return errors.Join(k.SetStringValue("", path), k.Close())
}
