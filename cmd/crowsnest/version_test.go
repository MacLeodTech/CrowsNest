package main

import "testing"

func TestVersionString(t *testing.T) {
	old := version
	defer func() { version = old }()
	for in, want := range map[string]string{"0.16.30": "0.16.30", "v1.2.3": "1.2.3"} {
		version = in
		if got := versionString(); got != want {
			t.Errorf("version %q: got %q, want %q", in, got, want)
		}
	}
	version = "dev"
	if got := versionString(); got != "dev" && len(got) < len("dev-abcdefg") {
		t.Errorf("dev build reported %q", got)
	}
}
