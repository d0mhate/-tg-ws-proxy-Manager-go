package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
)

// Asset is a single file attached to a GitHub release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	// Digest is "sha256:<hex64>" when GitHub provides one, empty otherwise.
	Digest string `json:"digest"`
}

// Release holds the tag and asset list returned by the GitHub releases/latest API.
type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

// FetchLatest calls the GitHub releases/latest API at apiURL and decodes the response.
func FetchLatest(ctx context.Context, apiURL string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API: %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &rel, nil
}

// AssetByName returns the first asset whose name matches, or (Asset{}, false).
func (r *Release) AssetByName(name string) (Asset, bool) {
	i := slices.IndexFunc(r.Assets, func(a Asset) bool { return a.Name == name })
	if i < 0 {
		return Asset{}, false
	}
	return r.Assets[i], true
}

// ParseDigest parses "sha256:<hex64>" and returns the lowercase hex string.
// Returns an error for any other format.
func ParseDigest(digest string) (string, error) {
	algo, sum, ok := strings.Cut(digest, ":")
	if !ok || algo != "sha256" || len(sum) != 64 {
		return "", fmt.Errorf("unsupported digest %q", digest)
	}
	return strings.ToLower(sum), nil
}

// SHA256File returns the lowercase hex-encoded SHA256 of the file at path.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
