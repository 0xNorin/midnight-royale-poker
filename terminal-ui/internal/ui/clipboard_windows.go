//go:build windows

package ui

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	clipboardUnicodeText  = 13
	clipboardGMEMMoveable = 0x0002
)

var (
	clipboardUser32            = windows.NewLazySystemDLL("user32.dll")
	clipboardKernel32          = windows.NewLazySystemDLL("kernel32.dll")
	clipboardOpen              = clipboardUser32.NewProc("OpenClipboard")
	clipboardClose             = clipboardUser32.NewProc("CloseClipboard")
	clipboardEmpty             = clipboardUser32.NewProc("EmptyClipboard")
	clipboardSetData           = clipboardUser32.NewProc("SetClipboardData")
	clipboardIsFormatAvailable = clipboardUser32.NewProc("IsClipboardFormatAvailable")
	clipboardGetData           = clipboardUser32.NewProc("GetClipboardData")
	clipboardGlobalAlloc       = clipboardKernel32.NewProc("GlobalAlloc")
	clipboardGlobalFree        = clipboardKernel32.NewProc("GlobalFree")
	clipboardGlobalLock        = clipboardKernel32.NewProc("GlobalLock")
	clipboardGlobalUnlock      = clipboardKernel32.NewProc("GlobalUnlock")
	clipboardGlobalSize        = clipboardKernel32.NewProc("GlobalSize")
)

func platformReadCodeClipboard() (string, error) {
	opened := false
	for attempt := 0; attempt < 6; attempt++ {
		ok, _, _ := clipboardOpen.Call(0)
		if ok != 0 {
			opened = true
			break
		}
		if attempt < 5 {
			time.Sleep(15 * time.Millisecond)
		}
	}
	if !opened {
		return "", errors.New("open Windows clipboard")
	}
	defer clipboardClose.Call()

	available, _, _ := clipboardIsFormatAvailable.Call(clipboardUnicodeText)
	if available == 0 {
		return "", nil
	}
	handle, _, callErr := clipboardGetData.Call(clipboardUnicodeText)
	if handle == 0 {
		return "", fmt.Errorf("get Windows clipboard data: %w", callErr)
	}
	ptr, _, callErr := clipboardGlobalLock.Call(handle)
	if ptr == 0 {
		return "", fmt.Errorf("lock Windows clipboard data: %w", callErr)
	}
	defer clipboardGlobalUnlock.Call(handle)

	size, _, _ := clipboardGlobalSize.Call(handle)
	if size < 2 {
		return "", nil
	}
	units := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), int(size/2))
	end := 0
	for end < len(units) && units[end] != 0 {
		end++
	}
	return string(utf16.Decode(units[:end])), nil
}

// platformWriteClipboard writes UTF-16 text to the Windows clipboard via the
// native Win32 API (CF_UNICODETEXT). This is reliable in Windows Terminal and
// ConPTY, where terminal escape-sequence clipboard (OSC 52) may not work.
func platformWriteClipboard(content string) error {
	opened := false
	for attempt := 0; attempt < 6; attempt++ {
		ok, _, _ := clipboardOpen.Call(0)
		if ok != 0 {
			opened = true
			break
		}
		if attempt < 5 {
			time.Sleep(15 * time.Millisecond)
		}
	}
	if !opened {
		return errors.New("open Windows clipboard")
	}
	defer clipboardClose.Call()

	if empty, _, _ := clipboardEmpty.Call(); empty == 0 {
		return errors.New("empty Windows clipboard")
	}

	units := utf16.Encode([]rune(content))
	handle, _, _ := clipboardGlobalAlloc.Call(clipboardGMEMMoveable, uintptr((len(units)+1)*2))
	if handle == 0 {
		return errors.New("allocate Windows clipboard memory")
	}
	ptr, _, _ := clipboardGlobalLock.Call(handle)
	if ptr == 0 {
		clipboardGlobalFree.Call(handle)
		return errors.New("lock Windows clipboard memory")
	}
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(units)+1)
	copy(dst, units)
	dst[len(units)] = 0
	clipboardGlobalUnlock.Call(handle)

	if set, _, _ := clipboardSetData.Call(clipboardUnicodeText, handle); set == 0 {
		// SetClipboardData failed, so the system does not own the handle.
		clipboardGlobalFree.Call(handle)
		return errors.New("set Windows clipboard data")
	}
	return nil
}
