package release_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"tg-ws-proxy/internal/release"
)

func mustHex(b []byte) string { return hex.EncodeToString(b) }

func sha256Of(data []byte) string {
	h := sha256.Sum256(data)
	return mustHex(h[:])
}

func serveRelease(t *testing.T, rel any) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(rel)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
}

func TestFetchLatest(t *testing.T) {
	tests := []struct {
		name       string
		payload    any
		wantTag    string
		wantAssets int
		wantErr    bool
	}{
		{
			name: "full release with digest",
			payload: map[string]any{
				"tag_name": "v1.2.3",
				"assets": []map[string]any{
					{"name": "proxy-linux-amd64", "browser_download_url": "https://example.com/dl", "digest": "sha256:" + sha256Of([]byte("data"))},
				},
			},
			wantTag:    "v1.2.3",
			wantAssets: 1,
		},
		{
			name:       "empty assets",
			payload:    map[string]any{"tag_name": "v0.1.0", "assets": []any{}},
			wantTag:    "v0.1.0",
			wantAssets: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := serveRelease(t, tc.payload)
			defer srv.Close()

			rel, err := release.FetchLatest(context.Background(), srv.URL)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rel.Tag != tc.wantTag {
				t.Errorf("tag: want %q got %q", tc.wantTag, rel.Tag)
			}
			if len(rel.Assets) != tc.wantAssets {
				t.Errorf("assets: want %d got %d", tc.wantAssets, len(rel.Assets))
			}
		})
	}
}

func TestFetchLatest_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := release.FetchLatest(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestAssetByName(t *testing.T) {
	rel := &release.Release{
		Tag: "v1.0.0",
		Assets: []release.Asset{
			{Name: "proxy-linux-amd64", DownloadURL: "https://example.com/a"},
			{Name: "proxy-linux-arm", DownloadURL: "https://example.com/b"},
		},
	}

	a, ok := rel.AssetByName("proxy-linux-arm")
	if !ok {
		t.Fatal("expected asset found")
	}
	if a.DownloadURL != "https://example.com/b" {
		t.Errorf("wrong asset returned: %+v", a)
	}

	_, ok = rel.AssetByName("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestParseDigest(t *testing.T) {
	good := "sha256:" + sha256Of([]byte("test"))
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: good, want: sha256Of([]byte("test"))},
		{input: "sha256:" + "A" + sha256Of([]byte("x"))[1:], want: "a" + sha256Of([]byte("x"))[1:]},
		{input: "md5:abc", wantErr: true},
		{input: "sha256:short", wantErr: true},
		{input: "nodivider", wantErr: true},
	}

	for _, tc := range tests {
		got, err := release.ParseDigest(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDigest(%q): expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDigest(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDigest(%q): want %q got %q", tc.input, tc.want, got)
		}
	}
}

func TestSHA256File(t *testing.T) {
	data := []byte("hello tg-ws-proxy")
	want := sha256Of(data)

	f, err := os.CreateTemp(t.TempDir(), "sha256test-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := release.SHA256File(f.Name())
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	if got != want {
		t.Errorf("want %s got %s", want, got)
	}

	_, err = release.SHA256File("/nonexistent/path")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
