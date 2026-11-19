package ui

import tea "charm.land/bubbletea/v2"

type codeClipboardMsg struct {
	Content string
	Err     error
}

var readCodeClipboard = platformReadCodeClipboard

func readCodeClipboardCommand() tea.Cmd {
	return func() tea.Msg {
		content, err := readCodeClipboard()
		return codeClipboardMsg{Content: content, Err: err}
	}
}

// writeClipboard copies text to the native OS clipboard. It is a variable so
// tests can substitute a fake without touching the real clipboard.
var writeClipboard = platformWriteClipboard

// copyResultMsg reports the outcome of an asynchronous clipboard write. Token
// identifies the write so a late result cannot clobber a newer copy.
type copyResultMsg struct {
	Token uint64
	Err   error
}

// writeClipboardCommand performs a clipboard write off the UI loop and reports
// success or failure back to the Bubble Tea update loop.
func writeClipboardCommand(content string, token uint64) tea.Cmd {
	return func() tea.Msg {
		return copyResultMsg{Token: token, Err: writeClipboard(content)}
	}
}
