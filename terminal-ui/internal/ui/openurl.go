package ui

// openExternalURL opens url in the user's default external browser. It
// never renders the page inside the game or terminal window. The actual
// launch is platform-specific (openurl_windows.go / openurl_other.go).
var openExternalURL = platformOpenExternalURL
