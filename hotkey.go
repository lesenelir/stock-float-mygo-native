package main

import "strings"

// accelerator turns the config's shortcut, written like ctrl+opt+s, into
// MyGo's accelerator syntax, Ctrl+Alt+S. It takes modifiers, then one letter
// or digit, and needs Control, Option or Command, so that the shortcut
// cannot shadow ordinary typing; ok is false otherwise, and for "".
func accelerator(spec string) (acc string, ok bool) {
	parts := strings.Split(strings.ToLower(spec), "+")
	key := strings.TrimSpace(parts[len(parts)-1])
	if len(key) != 1 || !(key[0] >= 'a' && key[0] <= 'z' || key[0] >= '0' && key[0] <= '9') {
		return "", false
	}
	var mods []string
	strong := false
	for _, part := range parts[:len(parts)-1] {
		var mod string
		switch strings.TrimSpace(part) {
		case "ctrl", "control":
			mod, strong = "Ctrl", true
		case "opt", "option", "alt":
			mod, strong = "Alt", true
		case "cmd", "command":
			mod, strong = "Cmd", true
		case "shift":
			mod = "Shift"
		default:
			return "", false
		}
		mods = append(mods, mod)
	}
	if !strong {
		return "", false
	}
	return strings.Join(append(mods, strings.ToUpper(key)), "+"), true
}
