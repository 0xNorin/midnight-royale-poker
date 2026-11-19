//go:build !windows

package ui

import "errors"

func platformReadCodeClipboard() (string, error) {
	return "", errors.New("native clipboard unsupported")
}

func platformWriteClipboard(content string) error {
	return errors.New("native clipboard unsupported")
}
