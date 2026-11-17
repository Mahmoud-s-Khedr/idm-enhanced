//go:build !darwin

package localfs

func SystemAlias(string) bool { return false }
