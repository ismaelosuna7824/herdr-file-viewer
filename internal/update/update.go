// Package update checks GitHub for a newer release of the plugin so the UI can
// nudge the user to update. It is best-effort: any error (offline, rate limit)
// just means "no update to report".
package update

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub owner/repo the releases live in.
const Repo = "ismaelosuna7824/herdr-file-viewer"

// Latest returns the tag name of the newest GitHub release (e.g. "v0.1.4").
func Latest(ctx context.Context) (string, error) {
	url := "https://api.github.com/repos/" + Repo + "/releases/latest"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.TagName, nil
}

// IsNewer reports whether latest is a strictly higher semver than current.
// Non-semver values (e.g. "dev" from an un-tagged local build) return false, so
// development builds are never nagged.
func IsNewer(latest, current string) bool {
	l, ok1 := parse(latest)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parse turns "vX.Y.Z" (or "X.Y.Z") into [3]int, ok=false if malformed.
func parse(v string) ([3]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
