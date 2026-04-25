package main

import (
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

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

func TestManagerInstallSelectsARMv7ReleaseAssetByDetectedArchVariant(t *testing.T) {
	env := managerEnv(t)

	openwrtRelease := envValue(env, "OPENWRT_RELEASE_FILE")
	sourceBin := envValue(env, "SOURCE_BIN")
	sourceVersion := envValue(env, "SOURCE_VERSION_FILE")
	binPath := envValue(env, "BIN_PATH")
	if openwrtRelease == "" || sourceBin == "" || sourceVersion == "" || binPath == "" {
		t.Fatal("missing required env paths")
	}

	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='arm_cortex-a7_neon-vfpv4'\n", 0o644)
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
		t.Fatalf("expected armv7 asset to be installed for arch variant, got:\n%s", got)
	}
}
