package main

import (
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

func TestManagerInstallSelectsAarch64ReleaseAssetByDetectedArch(t *testing.T) {
	env := managerEnv(t)

	openwrtRelease := envValue(env, "OPENWRT_RELEASE_FILE")
	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	if openwrtRelease == "" || sourceBin == "" || sourceVersion == "" || binPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='aarch64_cortex-a53'\n", 0o644)
	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.10\"}\n"))
		case "/download/tg-ws-proxy-openwrt-aarch64":
			_, _ = w.Write([]byte("#!/bin/sh\necho aarch64-asset\n"))
		case "/download/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho mips-asset\n"))
		case "/download/tg-ws-proxy-openwrt":
			_, _ = w.Write([]byte("#!/bin/sh\necho legacy-asset\n"))
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

	if got := readTrimmed(t, binPath); !strings.Contains(got, "aarch64-asset") {
		t.Fatalf("expected aarch64 asset to be installed, got:\n%s", got)
	}
}

func TestManagerInstallSelectsLegacyMipsAssetByDetectedArch(t *testing.T) {
	env := managerEnv(t)

	openwrtRelease := envValue(env, "OPENWRT_RELEASE_FILE")
	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	if openwrtRelease == "" || sourceBin == "" || sourceVersion == "" || binPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='mipsel_24kc'\n", 0o644)
	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.10\"}\n"))
		case "/download/tg-ws-proxy-openwrt-aarch64":
			_, _ = w.Write([]byte("#!/bin/sh\necho aarch64-asset\n"))
		case "/download/tg-ws-proxy-openwrt-mipsel_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho mips-asset\n"))
		case "/download/tg-ws-proxy-openwrt":
			_, _ = w.Write([]byte("#!/bin/sh\necho legacy-asset\n"))
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

	if got := readTrimmed(t, binPath); !strings.Contains(got, "mips-asset") {
		t.Fatalf("expected mips asset to be installed, got:\n%s", got)
	}
}

func TestManagerInstallSelectsMips24kcReleaseAssetByDetectedArch(t *testing.T) {
	env := managerEnv(t)

	openwrtRelease := envValue(env, "OPENWRT_RELEASE_FILE")
	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	if openwrtRelease == "" || sourceBin == "" || sourceVersion == "" || binPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='mips_24kc'\n", 0o644)
	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.10\"}\n"))
		case "/download/tg-ws-proxy-openwrt-mips_24kc":
			_, _ = w.Write([]byte("#!/bin/sh\necho mips24kc-asset\n"))
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

	if got := readTrimmed(t, binPath); !strings.Contains(got, "mips24kc-asset") {
		t.Fatalf("expected mips_24kc asset to be installed, got:\n%s", got)
	}
}

func TestManagerInstallSelectsX8664ReleaseAssetByDetectedArch(t *testing.T) {
	env := managerEnv(t)

	openwrtRelease := envValue(env, "OPENWRT_RELEASE_FILE")
	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	if openwrtRelease == "" || sourceBin == "" || sourceVersion == "" || binPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='x86_64'\n", 0o644)
	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.10\"}\n"))
		case "/download/tg-ws-proxy-openwrt-x86_64":
			_, _ = w.Write([]byte("#!/bin/sh\necho x86_64-asset\n"))
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

	if got := readTrimmed(t, binPath); !strings.Contains(got, "x86_64-asset") {
		t.Fatalf("expected x86_64 asset to be installed, got:\n%s", got)
	}
}

func TestManagerInstallSelectsARMv7ReleaseAssetByDetectedArch(t *testing.T) {
	env := managerEnv(t)

	openwrtRelease := envValue(env, "OPENWRT_RELEASE_FILE")
	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	if openwrtRelease == "" || sourceBin == "" || sourceVersion == "" || binPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='arm_cortex-a7'\n", 0o644)
	writeFile(t, sourceVersion, "v0.0.1\n", 0o644)
	writeFile(t, sourceBin, "#!/bin/sh\necho stale\n", 0o755)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{\"tag_name\":\"v9.9.10\"}\n"))
		case "/download/tg-ws-proxy-openwrt-armv7":
			_, _ = w.Write([]byte("#!/bin/sh\necho armv7-asset\n"))
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

	if got := readTrimmed(t, binPath); !strings.Contains(got, "armv7-asset") {
		t.Fatalf("expected armv7 asset to be installed, got:\n%s", got)
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

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
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
	env = append(env, "PREVIEW_BASE_URL="+serverURL+"/preview")

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

func TestManagerConfigureUpdateSourceViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "5\n8\npreview\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("configure update source failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Update source saved: preview feature/auth-flow") {
		t.Fatalf("expected update source saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch, got %q", got)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "src mode  : preview") || !strings.Contains(statusOut, "ref       : feature/auth-flow") {
		t.Fatalf("expected status to reflect preview update source, got:\n%s", statusOut)
	}
}

func TestManagerConfigureUpdateSourceViaAdvancedMenuSupportsArrowSelection(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_ARROW_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "5\n8\n\033[B\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("configure update source with arrows failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Mode (use arrows, Enter to confirm):") {
		t.Fatalf("expected arrow selection prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Update source saved: preview feature/auth-flow") {
		t.Fatalf("expected preview update source saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch, got %q", got)
	}
}

func TestManagerConfigureUpdateSourceViaAdvancedMenuSupportsNumberedSelection(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "5\n8\n2\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("configure update source with numbered picker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Select mode [1-2] (Enter for release):") {
		t.Fatalf("expected numbered selection prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Update source saved: preview feature/auth-flow") {
		t.Fatalf("expected preview update source saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch, got %q", got)
	}
}

func TestManagerConfigureUpdateSourcePreviewBranchPromptShowsExampleAndCanReuseSavedBranch(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "5\n8\n2\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("initial configure update source failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Preview branch name (for example: preview-channel):") {
		t.Fatalf("expected example preview branch prompt, got:\n%s", out)
	}

	out, err = runManagerMenu(t, env, "5\n8\n2\n\n\n")
	if err != nil {
		t.Fatalf("reusing saved preview branch failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Preview branch name (Enter to keep feature/auth-flow):") {
		t.Fatalf("expected keep-current preview branch prompt, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch to stay unchanged, got %q", got)
	}
}

func TestManagerConfigureUpdateSourceCanResetToLatestRelease(t *testing.T) {
	env := managerEnv(t)
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if updateChannelPath == "" || previewBranchPath == "" || releaseTagPath == "" {
		t.Fatal("missing update source state files in env")
	}

	writeFile(t, updateChannelPath, "preview\n", 0o644)
	writeFile(t, previewBranchPath, "feature/auth-flow\n", 0o644)
	writeFile(t, releaseTagPath, "v1.1.25\n", 0o644)

	out, err := runManagerMenu(t, env, "5\n8\nrelease\nlatest\n\n\n")
	if err != nil {
		t.Fatalf("reset update source failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Update source saved: latest release") {
		t.Fatalf("expected latest release saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "release" {
		t.Fatalf("expected persisted update channel release, got %q", got)
	}
	if _, err := os.Stat(previewBranchPath); !os.IsNotExist(err) {
		t.Fatalf("expected preview branch state to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(releaseTagPath); !os.IsNotExist(err) {
		t.Fatalf("expected release tag pin to be removed, stat err=%v", err)
	}
}

func TestManagerConfigureUpdateSourceCanSelectPinnedReleaseTagFromNumberedMenu(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if updateChannelPath == "" || releaseTagPath == "" {
		t.Fatal("missing release update source state files in env")
	}

	out, err := runManagerMenu(t, env, "5\n8\n1\n3\n\n\n")
	if err != nil {
		t.Fatalf("configure release tag from numbered menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Select mode [1-2] (Enter for release):") {
		t.Fatalf("expected numbered mode picker, got:\n%s", out)
	}
	if !strings.Contains(out, "Select release ref [1-") {
		t.Fatalf("expected numbered release ref picker, got:\n%s", out)
	}
	if strings.Contains(out, "v1.1.28") {
		t.Fatalf("expected release picker to hide tags below v1.1.29, got:\n%s", out)
	}
	if !strings.Contains(out, "Update source saved: release v1.1.29") {
		t.Fatalf("expected pinned release tag saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "release" {
		t.Fatalf("expected persisted update channel release, got %q", got)
	}
	if got := readTrimmed(t, releaseTagPath); got != "v1.1.29" {
		t.Fatalf("expected persisted release tag v1.1.29, got %q", got)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "src mode  : release") || !strings.Contains(statusOut, "ref       : v1.1.29") {
		t.Fatalf("expected status to reflect pinned release tag, got:\n%s", statusOut)
	}

	menuOut, err := runManagerMenu(t, env, "\n")
	if err != nil {
		t.Fatalf("menu failed: %v\n%s", err, menuOut)
	}
	if !strings.Contains(menuOut, "track     : release/v1.1.29") {
		t.Fatalf("expected main menu track to show pinned release tag, got:\n%s", menuOut)
	}
}

func TestManagerConfigureUpdateSourceRejectsManualReleaseTagBelowMinimum(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if updateChannelPath == "" || releaseTagPath == "" {
		t.Fatal("missing release update source state files in env")
	}

	out, err := runManagerMenu(t, env, "5\n8\n1\n4\nv1.1.28\n\n\n")
	if err != nil {
		t.Fatalf("manual release tag selection should stay in menu flow, got error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Release tag must be v1.1.29 or newer") {
		t.Fatalf("expected minimum release tag validation message, got:\n%s", out)
	}
	if _, err := os.Stat(releaseTagPath); !os.IsNotExist(err) {
		t.Fatalf("expected no pinned release tag to be persisted, stat err=%v", err)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "ref       : latest") {
		t.Fatalf("expected status to stay on latest release after rejected old tag, got:\n%s", statusOut)
	}
}

func TestManagerInstallShowsRateLimitHintWhenAPIReturnsEmpty(t *testing.T) {
	env := managerEnv(t)

	sourceBin := envValue(env, "SOURCE_BIN")
	if sourceBin == "" {
		t.Fatal("missing SOURCE_BIN in env")
	}

	// Remove source binary so the script must download it
	if err := os.Remove(sourceBin); err != nil {
		t.Fatalf("remove source bin: %v", err)
	}

	// Point API to a file with a rate-limit response (no tag_name field)
	rateLimitFile := sourceBin + ".ratelimit.json"
	writeFile(t, rateLimitFile, "{\"message\":\"API rate limit exceeded\"}\n", 0o644)
	env = setEnvValue(env, "RELEASE_API_URL", "file://"+rateLimitFile)

	out, err := runManager(t, env, "install")
	if err == nil {
		t.Fatalf("expected install to fail when API returns empty tag, got output:\n%s", out)
	}
	if !strings.Contains(out, "Could not detect latest release version") {
		t.Fatalf("expected version detection error, got:\n%s", out)
	}
	if !strings.Contains(out, "rate limit") {
		t.Fatalf("expected rate limit hint in output, got:\n%s", out)
	}
}

func TestManagerUpdateShowsRateLimitHintWhenAPIReturnsEmpty(t *testing.T) {
	env := managerEnv(t)

	// Point API to a file with a rate-limit response (no tag_name field)
	rateLimitFile := t.TempDir() + "/ratelimit.json"
	writeFile(t, rateLimitFile, "{\"message\":\"API rate limit exceeded\"}\n", 0o644)
	env = setEnvValue(env, "RELEASE_API_URL", "file://"+rateLimitFile)

	out, err := runManager(t, env, "update")
	if err == nil {
		t.Fatalf("expected update to fail when API returns empty tag, got output:\n%s", out)
	}
	if !strings.Contains(out, "Could not detect latest release version") {
		t.Fatalf("expected version detection error, got:\n%s", out)
	}
	if !strings.Contains(out, "rate limit") {
		t.Fatalf("expected rate limit hint in output, got:\n%s", out)
	}
}
