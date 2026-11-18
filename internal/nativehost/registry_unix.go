//go:build !windows

package nativehost

func registryEntry(_, _ string, _ bool) error { return nil }
