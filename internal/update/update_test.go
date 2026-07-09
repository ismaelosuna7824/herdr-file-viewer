package update

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.4", "v0.1.3", true},
		{"v0.2.0", "v0.1.9", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.1.3", "v0.1.3", false},
		{"v0.1.2", "v0.1.3", false},
		{"0.1.4", "0.1.3", true}, // no 'v'
		{"v0.1.4", "dev", false}, // dev build → never nagged
		{"garbage", "v0.1.3", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q,%q)=%v want %v", c.latest, c.current, got, c.want)
		}
	}
}
