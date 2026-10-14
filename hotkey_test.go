package main

import "testing"

func TestAcceleratorReadsTheConfigSyntax(t *testing.T) {
	for spec, want := range map[string]string{
		"ctrl+opt+s":         "Ctrl+Alt+S",
		"cmd+shift+9":        "Cmd+Shift+9",
		"Control + Option+K": "Ctrl+Alt+K",
		"alt+command+a":      "Alt+Cmd+A",
	} {
		if got, ok := accelerator(spec); !ok || got != want {
			t.Errorf("accelerator(%q) = %q, %v; want %q", spec, got, ok, want)
		}
	}
}

func TestAcceleratorRejectsShortcutsThatShadowTyping(t *testing.T) {
	for _, spec := range []string{"", "s", "shift+s", "ctrl+opt", "ctrl+opt+f1", "hyper+s", "ctrl+ü"} {
		if got, ok := accelerator(spec); ok {
			t.Errorf("accelerator(%q) = %q", spec, got)
		}
	}
}
