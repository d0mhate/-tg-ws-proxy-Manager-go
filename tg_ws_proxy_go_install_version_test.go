package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerUpdateRefreshesLauncherScriptFromRelease(t *testing.T) {
	env := managerEnv(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.9\"}\n"))
		case "/binary":
			_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
		case "/v9.9.9/tg-ws-proxy-go.sh":
			_, _ = w.Write([]byte("#!/bin/sh\necho manager-release-marker\n"))
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()
	serverURL := "http://" + listener.Addr().String()

	var launcherPath, installDir string
	for _, item := range env {
		switch {
		case strings.HasPrefix(item, "LAUNCHER_PATH="):
			launcherPath = strings.TrimPrefix(item, "LAUNCHER_PATH=")
		case strings.HasPrefix(item, "INSTALL_DIR="):
			installDir = strings.TrimPrefix(item, "INSTALL_DIR=")
		}
	}
	env = append(env,
		"RELEASE_API_URL="+serverURL+"/release.json",
		"RELEASE_URL="+serverURL+"/binary",
		"SCRIPT_RELEASE_BASE_URL="+serverURL,
	)

	out, err := runManager(t, env, "update")
	if err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}

	tmpManagerPath := filepath.Join(installDir, "tg-ws-proxy-go.sh")
	if got := readTrimmed(t, tmpManagerPath); !strings.Contains(got, "manager-release-marker") {
		t.Fatalf("expected installed manager to come from release, got:\n%s", got)
	}
	if launcher := readTrimmed(t, launcherPath); !strings.Contains(launcher, tmpManagerPath) {
		t.Fatalf("launcher does not point to installed manager:\n%s", launcher)
	}
}

func TestManagerUpdateRefreshesLegacyCurrentScriptPath(t *testing.T) {
	env := managerEnv(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.9\"}\n"))
		case "/binary":
			_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
		case "/v9.9.9/tg-ws-proxy-go.sh":
			_, _ = w.Write([]byte("#!/bin/sh\necho manager-release-marker\n"))
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()
	serverURL := "http://" + listener.Addr().String()

	legacyScript := filepath.Join(t.TempDir(), "legacy", "tg-ws-proxy-go.sh")
	copyManagerBundle(t, legacyScript)

	launcherPath := envValue(env, "LAUNCHER_PATH")
	installDir := envValue(env, "INSTALL_DIR")
	if launcherPath == "" || installDir == "" {
		t.Fatal("missing launcher or install dir in env")
	}
	writeFile(t, launcherPath, "#!/bin/sh\nsh "+legacyScript+" \"$@\"\n", 0o755)

	env = append(env,
		"RELEASE_API_URL="+serverURL+"/release.json",
		"RELEASE_URL="+serverURL+"/binary",
		"SCRIPT_RELEASE_BASE_URL="+serverURL,
	)

	out, err := runManagerAtPath(t, env, legacyScript, "update")
	if err != nil {
		t.Fatalf("update from legacy script path failed: %v\n%s", err, out)
	}

	if got := readTrimmed(t, legacyScript); !strings.Contains(got, "manager-release-marker") {
		t.Fatalf("expected current legacy script path to be refreshed from release, got:\n%s", got)
	}

	tmpManagerPath := filepath.Join(installDir, "tg-ws-proxy-go.sh")
	if launcher := readTrimmed(t, launcherPath); !strings.Contains(launcher, tmpManagerPath) {
		t.Fatalf("launcher does not point to refreshed installed manager:\n%s", launcher)
	}
}

func TestManagerUpdateFailsWhenTaggedManagerScriptCannotBeDownloaded(t *testing.T) {
	env := managerEnv(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.9\"}\n"))
		case "/binary":
			_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()
	serverURL := "http://" + listener.Addr().String()

	launcherPath := envValue(env, "LAUNCHER_PATH")
	if launcherPath == "" {
		t.Fatal("missing launcher path in env")
	}

	env = append(env,
		"RELEASE_API_URL="+serverURL+"/release.json",
		"RELEASE_URL="+serverURL+"/binary",
		"SCRIPT_RELEASE_BASE_URL="+serverURL,
	)

	out, err := runManager(t, env, "update")
	if err == nil {
		t.Fatalf("expected update to fail when tagged manager script cannot be downloaded:\n%s", out)
	}
	if !strings.Contains(out, "Manager script update failed") {
		t.Fatalf("expected manager script download failure message, got:\n%s", out)
	}
	if _, statErr := os.Stat(launcherPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected launcher to stay absent after failed manager update, stat err=%v", statErr)
	}
}

func TestManagerInstallUsesPinnedReleaseTag(t *testing.T) {
	env := managerEnv(t)

	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if sourceBin == "" || sourceVersion == "" || binPath == "" || releaseTagPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.10\"}\n"))
		case "/download/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho latest-asset\n"))
		case "/scripts/v1.1.25/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho pinned-asset\n"))
		case "/scripts/v1.1.25/tg-ws-proxy-go.sh":
			managerScript, err := os.ReadFile("tg-ws-proxy-go.sh")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(managerScript)
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()
	serverURL := "http://" + listener.Addr().String()

	env = unsetEnvValue(env, "RELEASE_URL")
	env = append(env,
		"RELEASE_TAG=v1.1.25",
		"RELEASE_API_URL="+serverURL+"/release.json",
		"RELEASE_DOWNLOAD_BASE_URL="+serverURL+"/download",
		"SCRIPT_RELEASE_BASE_URL="+serverURL+"/scripts",
	)

	out, err := runManager(t, env, "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Binary installed") {
		t.Fatalf("unexpected install output:\n%s", out)
	}
	if got := readTrimmed(t, binPath); !strings.Contains(got, "pinned-asset") {
		t.Fatalf("expected pinned asset to be installed, got:\n%s", got)
	}
	if got := readTrimmed(t, releaseTagPath); got != "v1.1.25" {
		t.Fatalf("expected pinned release tag state, got %q", got)
	}
}

func TestManagerUpdateUsesPersistedPinnedReleaseTag(t *testing.T) {
	env := managerEnv(t)

	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	versionPath := envValue(env, "VERSION_FILE")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if sourceBin == "" || sourceVersion == "" || binPath == "" || versionPath == "" || releaseTagPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)
	writeFile(t, binPath, "#!/bin/sh\necho installed-old\n", 0o755)
	writeFile(t, versionPath, "v0.0.1\n", 0o644)
	writeFile(t, releaseTagPath, "v1.1.25\n", 0o644)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.99\"}\n"))
		case "/download/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho latest-asset\n"))
		case "/scripts/v1.1.25/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho pinned-update-asset\n"))
		case "/scripts/v1.1.25/tg-ws-proxy-go.sh":
			managerScript, err := os.ReadFile("tg-ws-proxy-go.sh")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(managerScript)
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()
	serverURL := "http://" + listener.Addr().String()

	env = unsetEnvValue(env, "RELEASE_URL")
	env = append(env,
		"RELEASE_API_URL="+serverURL+"/release.json",
		"RELEASE_DOWNLOAD_BASE_URL="+serverURL+"/download",
		"SCRIPT_RELEASE_BASE_URL="+serverURL+"/scripts",
	)

	out, err := runManager(t, env, "update")
	if err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Updated to v1.1.25") {
		t.Fatalf("expected pinned update output, got:\n%s", out)
	}
	if got := readTrimmed(t, binPath); !strings.Contains(got, "pinned-update-asset") {
		t.Fatalf("expected pinned asset to be installed during update, got:\n%s", got)
	}
	if got := readTrimmed(t, releaseTagPath); got != "v1.1.25" {
		t.Fatalf("expected persisted pinned release tag to remain, got %q", got)
	}
}

func TestManagerInstallWithReleaseTagLatestClearsPinnedReleaseTag(t *testing.T) {
	env := managerEnv(t)

	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if sourceBin == "" || sourceVersion == "" || binPath == "" || releaseTagPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)
	writeFile(t, releaseTagPath, "v1.1.25\n", 0o644)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.10\"}\n"))
		case "/download/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho latest-asset\n"))
		case "/scripts/v9.9.10/tg-ws-proxy-go.sh":
			managerScript, err := os.ReadFile("tg-ws-proxy-go.sh")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(managerScript)
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()
	serverURL := "http://" + listener.Addr().String()

	env = unsetEnvValue(env, "RELEASE_URL")
	env = append(env,
		"RELEASE_TAG=latest",
		"RELEASE_API_URL="+serverURL+"/release.json",
		"RELEASE_DOWNLOAD_BASE_URL="+serverURL+"/download",
		"SCRIPT_RELEASE_BASE_URL="+serverURL+"/scripts",
	)

	out, err := runManager(t, env, "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if got := readTrimmed(t, binPath); !strings.Contains(got, "latest-asset") {
		t.Fatalf("expected latest asset to be installed, got:\n%s", got)
	}
	if _, err := os.Stat(releaseTagPath); !os.IsNotExist(err) {
		t.Fatalf("expected pinned release tag file to be removed, stat err=%v", err)
	}
}

func TestManagerUpdateUsesPersistedPreviewBranch(t *testing.T) {
	env := managerEnv(t)

	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if sourceBin == "" || sourceVersion == "" || binPath == "" || updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)
	writeFile(t, updateChannelPath, "preview\n", 0o644)
	writeFile(t, previewBranchPath, "feature/auth-flow\n", 0o644)
	wrongDigest := sha256.Sum256([]byte("release-binary-from-another-source"))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.9\",\"assets\":[{\"name\":\"tg-ws-proxy-openwrt-mipsel_24kc\",\"digest\":\"sha256:" + hex.EncodeToString(wrongDigest[:]) + "\"}]}\n"))
		case "/preview/feature/auth-flow/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho preview-asset\n"))
		case "/preview/feature/auth-flow/tg-ws-proxy-go.sh":
			managerScript, err := os.ReadFile("tg-ws-proxy-go.sh")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(managerScript)
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()
	serverURL := "http://" + listener.Addr().String()

	env = unsetEnvValue(env, "RELEASE_URL")
	env = append(env,
		"PREVIEW_BASE_URL="+serverURL+"/preview",
		"RELEASE_API_URL="+serverURL+"/release.json",
	)

	out, err := runManager(t, env, "update")
	if err != nil {
		t.Fatalf("preview update failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Preview branch: feature/auth-flow") {
		t.Fatalf("expected preview branch output, got:\n%s", out)
	}
	if got := readTrimmed(t, binPath); !strings.Contains(got, "preview-asset") {
		t.Fatalf("expected preview asset to be installed during update, got:\n%s", got)
	}
}
