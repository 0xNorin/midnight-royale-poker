//go:build windows

package ui

import "golang.org/x/sys/windows"

// platformOpenExternalURL opens url in the user's default external browser via
// the native ShellExecuteW API. Unlike spawning "rundll32 url.dll" (whose
// Start only reports that the helper process was launched, never whether a
// browser actually opened), ShellExecuteW returns the real ShellExecute
// result: a broken browser association, a missing handler, or any other
// launch failure surfaces as a non-nil error so the caller can show it to
// the user instead of silently doing nothing.
func platformOpenExternalURL(url string) error {
	return windows.ShellExecute(0, nil, windows.StringToUTF16Ptr(url), nil, nil, windows.SW_SHOWNORMAL)
}
