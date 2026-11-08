package api

// Library switch diagnostics are disabled. Keep the call sites inert so normal
// error handling and runtime lifecycle behavior are unaffected.
func librarySwitchLogger(string) func(string, ...any) {
	return func(string, ...any) {}
}
