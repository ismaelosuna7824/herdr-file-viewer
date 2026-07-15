package viewer

// terminalHyperlinks deliberately leaves visible URLs untouched. Herdr already
// detects http(s) text as clickable; wrapping the same text in OSC 8 creates a
// second activation target and makes one Ctrl-click open two browser tabs.
func terminalHyperlinks(s string) string {
	return s
}
