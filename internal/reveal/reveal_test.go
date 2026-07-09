package reveal

import (
	"path/filepath"
	"testing"
)

func TestCommandPerPlatform(t *testing.T) {
	const path = "/home/u/proj/src/main.go"
	cases := []struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		{"darwin", "open", []string{"-R", path}},
		{"windows", "explorer", []string{"/select," + path}},
		{"linux", "xdg-open", []string{filepath.Dir(path)}},
		{"freebsd", "xdg-open", []string{filepath.Dir(path)}},
	}
	for _, c := range cases {
		name, args := command(c.goos, path)
		if name != c.wantName {
			t.Errorf("%s: name = %q, want %q", c.goos, name, c.wantName)
		}
		if len(args) != len(c.wantArgs) {
			t.Fatalf("%s: args = %v, want %v", c.goos, args, c.wantArgs)
		}
		for i := range args {
			if args[i] != c.wantArgs[i] {
				t.Errorf("%s: args[%d] = %q, want %q", c.goos, i, args[i], c.wantArgs[i])
			}
		}
	}
}
