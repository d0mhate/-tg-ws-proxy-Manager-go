package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTrimmed(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(data))
}

func managerEnv(t *testing.T) []string {
	t.Helper()

	root := t.TempDir()
	sourceBin := filepath.Join(root, "source", "tg-ws-proxy-openwrt")
	sourceVersion := sourceBin + ".version"
	sourceManager := filepath.Join(root, "source", "tg-ws-proxy-go.sh")
	managerScriptPath := filepath.Join(root, "manager", "tg-ws-proxy-go.sh")
	releaseAPI := filepath.Join(root, "release.json")
	installDir := filepath.Join(root, "tmp-install")
	persistStateDir := filepath.Join(root, "persist-state")
	initScriptPath := filepath.Join(root, "init.d", "tg-ws-proxy-go")
	launcherPath := filepath.Join(root, "bin", "tgm")
	rcCommonPath := filepath.Join(root, "etc", "rc.common")
	rcDir := filepath.Join(root, "rc.d")
	openwrtRelease := filepath.Join(root, "etc", "openwrt_release")
	persistA := filepath.Join(root, "persist-a")
	persistB := filepath.Join(root, "persist-b")
	scriptBase := filepath.Join(root, "scripts")
	scriptReleasePath := filepath.Join(scriptBase, "v9.9.9", "tg-ws-proxy-go.sh")

	writeFile(t, sourceBin, "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, sourceVersion, "v9.9.9\n", 0o644)
	writeFile(t, releaseAPI, "{\"tag_name\":\"v9.9.9\"}\n", 0o644)
	writeFile(t, rcCommonPath, "#!/bin/sh\nscript=\"$1\"\ncmd=\"$2\"\nname=\"$(basename \"$script\")\"\nrc_dir=\"${RC_D_DIR:-/etc/rc.d}\"\nmkdir -p \"$rc_dir\"\ncase \"$cmd\" in\nenable)\n  ln -sf \"$script\" \"$rc_dir/S95$name\"\n  ;;\ndisable)\n  rm -f \"$rc_dir\"/*\"$name\"\n  ;;\nstop)\n  exit 0\n  ;;\n*)\n  exit 0\n  ;;\nesac\n", 0o755)
	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='mipsel_24kc'\n", 0o644)
	managerScript, err := os.ReadFile("tg-ws-proxy-go.sh")
	if err != nil {
		t.Fatalf("read manager script: %v", err)
	}
	writeFile(t, managerScriptPath, string(managerScript), 0o755)
	writeFile(t, scriptReleasePath, string(managerScript), 0o755)

	env := append([]string{}, os.Environ()...)
	env = append(env,
		"RELEASE_API_URL=file://"+releaseAPI,
		"RELEASE_URL=file://"+sourceBin,
		"SCRIPT_RELEASE_BASE_URL=file://"+scriptBase,
		"SOURCE_BIN="+sourceBin,
		"SOURCE_VERSION_FILE="+sourceVersion,
		"SOURCE_MANAGER_SCRIPT="+sourceManager,
		"MANAGER_SCRIPT_PATH="+managerScriptPath,
		"INSTALL_DIR="+installDir,
		"BIN_PATH="+filepath.Join(installDir, "tg-ws-proxy"),
		"VERSION_FILE="+filepath.Join(installDir, "version"),
		"PERSIST_STATE_DIR="+persistStateDir,
		"PERSIST_PATH_FILE="+filepath.Join(persistStateDir, "install_dir"),
		"PERSIST_VERSION_FILE="+filepath.Join(persistStateDir, "version"),
		"PERSIST_CONFIG_FILE="+filepath.Join(persistStateDir, "autostart.conf"),
		"INIT_SCRIPT_PATH="+initScriptPath,
		"LAUNCHER_PATH="+launcherPath,
		"OPENWRT_RELEASE_FILE="+openwrtRelease,
		"RC_COMMON_PATH="+rcCommonPath,
		"RC_D_DIR="+rcDir,
		"PERSISTENT_DIR_CANDIDATES="+persistA+" "+persistB,
		"REQUIRED_TMP_KB=1",
		"PERSISTENT_SPACE_HEADROOM_KB=0",
		"LISTEN_HOST=0.0.0.0",
		"LISTEN_PORT=1081",
		"VERBOSE=1",
	)
	return env
}

func managerScriptPath(env []string) string {
	scriptPath := envValue(env, "MANAGER_SCRIPT_PATH")
	if scriptPath != "" {
		return scriptPath
	}
	return "tg-ws-proxy-go.sh"
}

func runManager(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", append([]string{managerScriptPath(env)}, args...)...)
	cmd.Dir = "."
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runManagerAtPath(t *testing.T, env []string, scriptPath string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", append([]string{scriptPath}, args...)...)
	cmd.Dir = "."
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runManagerMenu(t *testing.T, env []string, input string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", managerScriptPath(env))
	cmd.Dir = "."
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	updated := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			if !replaced {
				updated = append(updated, prefix+value)
				replaced = true
			}
			continue
		}
		updated = append(updated, item)
	}
	if !replaced {
		updated = append(updated, prefix+value)
	}
	return updated
}

func unsetEnvValue(env []string, key string) []string {
	prefix := key + "="
	updated := make([]string, 0, len(env))
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		updated = append(updated, item)
	}
	return updated
}

func buildFakeProxyBinary(t *testing.T, path string) {
	t.Helper()

	source := filepath.Join(t.TempDir(), "main.go")
	writeFile(t, source, `package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
}
`, 0o644)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fake proxy dir: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", path, source)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake proxy binary: %v\n%s", err, string(out))
	}
}

func waitForMenuText(t *testing.T, env []string, want string) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	lastOut := ""
	for time.Now().Before(deadline) {
		out, err := runManagerMenu(t, env, "\n")
		if err == nil && strings.Contains(out, want) {
			return out
		}
		lastOut = out
		time.Sleep(150 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for menu text %q\nlast output:\n%s", want, lastOut)
	return ""
}

func TestManagerEnableAutostartInstallsPersistentCopy(t *testing.T) {
	env := managerEnv(t)

	out, err := runManager(t, env, "enable-autostart")
	if err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Persistent copy installed automatically") || !strings.Contains(out, "Autostart enabled") {
		t.Fatalf("expected success output, got:\n%s", out)
	}

	var persistDir, managerPath, launcherPath, statePath, versionPath string
	for _, item := range env {
		switch {
		case strings.HasPrefix(item, "PERSISTENT_DIR_CANDIDATES="):
			persistDir = strings.Split(strings.TrimPrefix(item, "PERSISTENT_DIR_CANDIDATES="), " ")[0]
		case strings.HasPrefix(item, "LAUNCHER_PATH="):
			launcherPath = strings.TrimPrefix(item, "LAUNCHER_PATH=")
		case strings.HasPrefix(item, "PERSIST_PATH_FILE="):
			statePath = strings.TrimPrefix(item, "PERSIST_PATH_FILE=")
		case strings.HasPrefix(item, "PERSIST_VERSION_FILE="):
			versionPath = strings.TrimPrefix(item, "PERSIST_VERSION_FILE=")
		}
	}
	managerPath = filepath.Join(persistDir, "tg-ws-proxy-go.sh")

	if _, err := os.Stat(filepath.Join(persistDir, "tg-ws-proxy")); err != nil {
		t.Fatalf("expected persistent binary: %v", err)
	}
	if _, err := os.Stat(managerPath); err != nil {
		t.Fatalf("expected copied manager script: %v", err)
	}
	if got := readTrimmed(t, statePath); got != persistDir {
		t.Fatalf("unexpected persistent dir state: %q", got)
	}
	if got := readTrimmed(t, versionPath); got != "v9.9.9" {
		t.Fatalf("unexpected persistent version: %q", got)
	}
	if launcher := readTrimmed(t, launcherPath); !strings.Contains(launcher, managerPath) {
		t.Fatalf("launcher does not point to persistent manager:\n%s", launcher)
	}
}

func TestManagerDisableAutostartRemovesPersistentCopy(t *testing.T) {
	env := managerEnv(t)

	out, err := runManager(t, env, "enable-autostart")
	if err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	var persistDir, configPath, initScriptPath, rcDir, launcherPath string
	for _, item := range env {
		switch {
		case strings.HasPrefix(item, "PERSISTENT_DIR_CANDIDATES="):
			persistDir = strings.Split(strings.TrimPrefix(item, "PERSISTENT_DIR_CANDIDATES="), " ")[0]
		case strings.HasPrefix(item, "PERSIST_CONFIG_FILE="):
			configPath = strings.TrimPrefix(item, "PERSIST_CONFIG_FILE=")
		case strings.HasPrefix(item, "INIT_SCRIPT_PATH="):
			initScriptPath = strings.TrimPrefix(item, "INIT_SCRIPT_PATH=")
		case strings.HasPrefix(item, "RC_D_DIR="):
			rcDir = strings.TrimPrefix(item, "RC_D_DIR=")
		case strings.HasPrefix(item, "LAUNCHER_PATH="):
			launcherPath = strings.TrimPrefix(item, "LAUNCHER_PATH=")
		}
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "BIN='"+filepath.Join(persistDir, "tg-ws-proxy")+"'") {
		t.Fatalf("config missing binary path:\n%s", config)
	}
	if !strings.Contains(config, "HOST='0.0.0.0'") || !strings.Contains(config, "PORT='1081'") || !strings.Contains(config, "VERBOSE='1'") {
		t.Fatalf("config missing runtime settings:\n%s", config)
	}
	if _, err := os.Stat(initScriptPath); err != nil {
		t.Fatalf("expected init script: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(rcDir, "S95"+filepath.Base(initScriptPath))); err != nil {
		t.Fatalf("expected rc.d symlink: %v", err)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "persist   : installed") || !strings.Contains(statusOut, "autostart : enabled") {
		t.Fatalf("status did not report persistent autostart state:\n%s", statusOut)
	}

	disableOut, err := runManager(t, env, "disable-autostart")
	if err != nil {
		t.Fatalf("disable-autostart failed: %v\n%s", err, disableOut)
	}
	if !strings.Contains(disableOut, "Autostart disabled and persistent copy removed") {
		t.Fatalf("unexpected disable output:\n%s", disableOut)
	}
	if _, err := os.Lstat(filepath.Join(rcDir, "S95"+filepath.Base(initScriptPath))); !os.IsNotExist(err) {
		t.Fatalf("expected rc.d symlink to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(persistDir, "tg-ws-proxy")); !os.IsNotExist(err) {
		t.Fatalf("expected persistent binary to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(launcherPath); !os.IsNotExist(err) {
		t.Fatalf("expected launcher to be removed when no tmp install remains, stat err=%v", err)
	}
}

func TestManagerAutostartConfigAutoSyncsCurrentSettings(t *testing.T) {
	env := managerEnv(t)

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	configPath := ""
	for _, item := range env {
		if strings.HasPrefix(item, "PERSIST_CONFIG_FILE=") {
			configPath = strings.TrimPrefix(item, "PERSIST_CONFIG_FILE=")
			break
		}
	}

	syncedEnv := append([]string{}, env...)
	syncedEnv = append(syncedEnv, "LISTEN_HOST=127.0.0.1", "LISTEN_PORT=2090", "VERBOSE=0")

	out, err := runManager(t, syncedEnv, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "HOST='127.0.0.1'") || !strings.Contains(config, "PORT='2090'") || !strings.Contains(config, "VERBOSE='0'") {
		t.Fatalf("expected autostart config to sync current settings, got:\n%s", config)
	}
}

func TestManagerEnableAutostartFailsWithoutPersistentSpace(t *testing.T) {
	env := append([]string{}, managerEnv(t)...)
	env = append(env, "PERSISTENT_SPACE_HEADROOM_KB=999999999")

	out, err := runManager(t, env, "enable-autostart")
	if err == nil {
		t.Fatalf("expected enable-autostart to fail when no persistent path has enough space:\n%s", out)
	}
	if !strings.Contains(out, "No suitable persistent path found") {
		t.Fatalf("expected no-space message, got:\n%s", out)
	}
}

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
	managerScript, err := os.ReadFile("tg-ws-proxy-go.sh")
	if err != nil {
		t.Fatalf("read manager script: %v", err)
	}
	writeFile(t, legacyScript, string(managerScript), 0o755)

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

func TestManagerStatusIgnoresFalsePositivePgrepMatches(t *testing.T) {
	env := managerEnv(t)

	root := t.TempDir()
	fakeBinDir := filepath.Join(root, "fake-bin")
	procRoot := filepath.Join(root, "proc")
	binPath := ""
	otherBin := filepath.Join(root, "other", "unrelated")

	for _, item := range env {
		if strings.HasPrefix(item, "BIN_PATH=") {
			binPath = strings.TrimPrefix(item, "BIN_PATH=")
			break
		}
	}
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	writeFile(t, binPath, "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, otherBin, "#!/bin/sh\nexit 0\n", 0o755)
	if err := os.MkdirAll(filepath.Join(procRoot, "222"), 0o755); err != nil {
		t.Fatalf("mkdir proc 222: %v", err)
	}
	if err := os.Symlink(otherBin, filepath.Join(procRoot, "222", "exe")); err != nil {
		t.Fatalf("symlink proc 222 exe: %v", err)
	}

	writeFile(t, filepath.Join(fakeBinDir, "pgrep"), "#!/bin/sh\nprintf '222\n'\n", 0o755)
	writeFile(t, filepath.Join(fakeBinDir, "readlink"), "#!/bin/sh\nif [ \"$1\" = \"-f\" ]; then\n  shift\nfi\ntarget=\"$1\"\nif [ -L \"$target\" ]; then\n  link=\"$(/bin/readlink \"$target\")\"\n  case \"$link\" in\n    /*) printf '%s\\n' \"$link\" ;;\n    *) dir=\"$(cd \"$(dirname \"$target\")\" && pwd -P)\"; printf '%s/%s\\n' \"$dir\" \"$link\" ;;\n  esac\n  exit 0\nfi\ndir=\"$(cd \"$(dirname \"$target\")\" 2>/dev/null && pwd -P)\" || exit 1\nprintf '%s/%s\\n' \"$dir\" \"$(basename \"$target\")\"\n", 0o755)

	env = append(env,
		"PATH="+fakeBinDir+":"+os.Getenv("PATH"),
		"PROC_ROOT="+procRoot,
	)

	out, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "process   : stopped") || !strings.Contains(out, "pid       : -") {
		t.Fatalf("expected unrelated pgrep hit to be ignored, got:\n%s", out)
	}
	if strings.Contains(out, "222") {
		t.Fatalf("expected false pgrep match to be filtered out, got:\n%s", out)
	}
}

func TestManagerMainMenuShowsSimplifiedActions(t *testing.T) {
	env := managerEnv(t)

	out, err := runManagerMenu(t, env, "\n")
	if err != nil {
		t.Fatalf("menu failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "1) Setup / Update") ||
		!strings.Contains(out, "2) Start proxy") ||
		!strings.Contains(out, "3) Enable autostart") ||
		!strings.Contains(out, "5) Advanced") {
		t.Fatalf("expected simplified top-level menu, got:\n%s", out)
	}

	if strings.Contains(out, "Show quick commands") || strings.Contains(out, "Remove binary and runtime files") {
		t.Fatalf("expected advanced-only actions to be absent from top-level menu:\n%s", out)
	}
}

func TestManagerMainMenuReflectsAutostartState(t *testing.T) {
	env := managerEnv(t)

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	out, err := runManagerMenu(t, env, "\n")
	if err != nil {
		t.Fatalf("menu failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "3) Disable autostart") {
		t.Fatalf("expected menu to reflect enabled autostart, got:\n%s", out)
	}
}

func TestManagerMainMenuReflectsRunningProxyStateTransitions(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	buildFakeProxyBinary(t, binPath)

	startCmd := exec.Command("sh", "tg-ws-proxy-go.sh", "start")
	startCmd.Dir = "."
	startCmd.Env = env
	var startOut bytes.Buffer
	startCmd.Stdout = &startOut
	startCmd.Stderr = &startOut
	if err := startCmd.Start(); err != nil {
		t.Fatalf("start command failed to launch: %v", err)
	}

	waitForMenuText(t, env, "2) Stop proxy")

	stopOut, err := runManager(t, env, "stop")
	if err != nil {
		t.Fatalf("stop failed: %v\n%s", err, stopOut)
	}
	if !strings.Contains(stopOut, "Proxy stopped") {
		t.Fatalf("expected stop confirmation, got:\n%s", stopOut)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- startCmd.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil {
			t.Fatalf("start command exited with error: %v\n%s", err, startOut.String())
		}
	case <-time.After(5 * time.Second):
		_ = startCmd.Process.Kill()
		t.Fatalf("timed out waiting for started proxy command to exit\n%s", startOut.String())
	}

	out := waitForMenuText(t, env, "2) Start proxy")
	if !strings.Contains(out, "proxy     : stopped") {
		t.Fatalf("expected stopped summary after stop, got:\n%s", out)
	}
}

func TestManagerMainMenuReflectsAutostartStateTransitions(t *testing.T) {
	env := managerEnv(t)

	enableOut, err := runManagerMenu(t, env, "3\n\n\n")
	if err != nil {
		t.Fatalf("menu enable-autostart failed: %v\n%s", err, enableOut)
	}
	if !strings.Contains(enableOut, "Autostart enabled") {
		t.Fatalf("expected autostart enable output, got:\n%s", enableOut)
	}

	out := waitForMenuText(t, env, "3) Disable autostart")
	if !strings.Contains(out, "autostart : enabled") {
		t.Fatalf("expected enabled autostart summary, got:\n%s", out)
	}

	disableOut, err := runManagerMenu(t, env, "3\n\n\n")
	if err != nil {
		t.Fatalf("menu disable-autostart failed: %v\n%s", err, disableOut)
	}
	if !strings.Contains(disableOut, "Autostart disabled and persistent copy removed") {
		t.Fatalf("expected autostart disable output, got:\n%s", disableOut)
	}

	out = waitForMenuText(t, env, "3) Enable autostart")
	if !strings.Contains(out, "autostart : disabled") {
		t.Fatalf("expected disabled autostart summary, got:\n%s", out)
	}
}

func TestManagerStartFailsWithoutBinary(t *testing.T) {
	env := managerEnv(t)

	out, err := runManager(t, env, "start")
	if err == nil {
		t.Fatalf("expected start to fail without binary:\n%s", out)
	}
	if !strings.Contains(out, "binary is not installed") {
		t.Fatalf("expected missing binary message, got:\n%s", out)
	}
}

func TestManagerStartFailsWhenPortBusy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	env := managerEnv(t)
	env = append(env,
		"LISTEN_HOST=127.0.0.1",
		fmt.Sprintf("LISTEN_PORT=%d", listener.Addr().(*net.TCPAddr).Port),
	)

	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}
	buildFakeProxyBinary(t, binPath)

	out, err := runManager(t, env, "start")
	if err == nil {
		t.Fatalf("expected start to fail when port is busy:\n%s", out)
	}
	if !strings.Contains(out, "Port") || !strings.Contains(out, "is already busy") {
		t.Fatalf("expected busy port message, got:\n%s", out)
	}
}

func TestManagerAdvancedRemoveResetsMenuState(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	buildFakeProxyBinary(t, binPath)
	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	removeOut, err := runManagerMenu(t, env, "5\n5\n\n\n\n")
	if err != nil {
		t.Fatalf("advanced remove failed: %v\n%s", err, removeOut)
	}
	if !strings.Contains(removeOut, "Binary launcher autostart and downloaded files removed") {
		t.Fatalf("expected remove confirmation, got:\n%s", removeOut)
	}

	out := waitForMenuText(t, env, "2) Start proxy")
	if !strings.Contains(out, "3) Enable autostart") {
		t.Fatalf("expected clean top-level menu after remove, got:\n%s", out)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "tmp bin   : not installed") || !strings.Contains(statusOut, "persist   : not installed") {
		t.Fatalf("expected removed state in status, got:\n%s", statusOut)
	}
}

func TestManagerToggleVerboseUpdatesSummaryStatusAndAutostartConfig(t *testing.T) {
	env := setEnvValue(managerEnv(t), "VERBOSE", "0")
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	menuOut, err := runManagerMenu(t, env, "5\n2\n\n\n")
	if err != nil {
		t.Fatalf("toggle verbose via menu failed: %v\n%s", err, menuOut)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "VERBOSE='1'") {
		t.Fatalf("expected autostart config to switch verbose on, got:\n%s", config)
	}

	checkEnv := unsetEnvValue(env, "VERBOSE")

	statusOut, err := runManager(t, checkEnv, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "verbose   : on") {
		t.Fatalf("expected status to show verbose on, got:\n%s", statusOut)
	}

	out := waitForMenuText(t, checkEnv, "verbose   : on")
	if !strings.Contains(out, "3) Disable autostart") {
		t.Fatalf("expected menu to keep autostart enabled after verbose toggle, got:\n%s", out)
	}
}

func TestManagerEnableAutostartFailureLeavesCleanState(t *testing.T) {
	env := setEnvValue(managerEnv(t), "PERSISTENT_SPACE_HEADROOM_KB", "999999999")
	initScriptPath := envValue(env, "INIT_SCRIPT_PATH")
	persistPathFile := envValue(env, "PERSIST_PATH_FILE")

	out, err := runManager(t, env, "enable-autostart")
	if err == nil {
		t.Fatalf("expected enable-autostart failure:\n%s", out)
	}
	if _, err := os.Stat(initScriptPath); !os.IsNotExist(err) {
		t.Fatalf("expected no init script after failed enable, stat err=%v", err)
	}
	if _, err := os.Stat(persistPathFile); !os.IsNotExist(err) {
		t.Fatalf("expected no persistent state after failed enable, stat err=%v", err)
	}

	menuOut := waitForMenuText(t, env, "3) Enable autostart")
	if !strings.Contains(menuOut, "autostart : disabled") {
		t.Fatalf("expected menu to stay disabled after failed enable, got:\n%s", menuOut)
	}
}

func TestManagerDisableAutostartNoopWhenNotConfigured(t *testing.T) {
	env := managerEnv(t)

	out, err := runManager(t, env, "disable-autostart")
	if err != nil {
		t.Fatalf("disable-autostart no-op failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Autostart is not configured") {
		t.Fatalf("expected no-op autostart message, got:\n%s", out)
	}
}

func TestManagerRestartStartsStoppedProxyAndMenuShowsStop(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	buildFakeProxyBinary(t, binPath)

	restartCmd := exec.Command("sh", "tg-ws-proxy-go.sh", "restart")
	restartCmd.Dir = "."
	restartCmd.Env = env
	var restartOut bytes.Buffer
	restartCmd.Stdout = &restartOut
	restartCmd.Stderr = &restartOut
	if err := restartCmd.Start(); err != nil {
		t.Fatalf("restart command failed to launch: %v", err)
	}

	out := waitForMenuText(t, env, "2) Stop proxy")
	if !strings.Contains(out, "proxy     : running") {
		t.Fatalf("expected menu to show running proxy after restart, got:\n%s", out)
	}

	stopOut, err := runManager(t, env, "stop")
	if err != nil {
		t.Fatalf("stop after restart failed: %v\n%s", err, stopOut)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- restartCmd.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil {
			t.Fatalf("restart command exited with error: %v\n%s", err, restartOut.String())
		}
	case <-time.After(5 * time.Second):
		_ = restartCmd.Process.Kill()
		t.Fatalf("timed out waiting for restart command to exit\n%s", restartOut.String())
	}
}

func TestManagerStatusAndMenuStayInSync(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	buildFakeProxyBinary(t, binPath)
	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	startCmd := exec.Command("sh", "tg-ws-proxy-go.sh", "start")
	startCmd.Dir = "."
	startCmd.Env = env
	var startOut bytes.Buffer
	startCmd.Stdout = &startOut
	startCmd.Stderr = &startOut
	if err := startCmd.Start(); err != nil {
		t.Fatalf("start command failed: %v", err)
	}
	defer func() {
		_, _ = runManager(t, env, "stop")
		_ = startCmd.Wait()
	}()

	menuOut := waitForMenuText(t, env, "2) Stop proxy")
	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}

	if !strings.Contains(menuOut, "proxy     : running") || !strings.Contains(statusOut, "process   : running") {
		t.Fatalf("menu/status disagree on running state\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
	if !strings.Contains(menuOut, "autostart : enabled") || !strings.Contains(statusOut, "autostart : enabled") {
		t.Fatalf("menu/status disagree on autostart state\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
	if !strings.Contains(menuOut, "verbose   : on") || !strings.Contains(statusOut, "verbose   : on") {
		t.Fatalf("menu/status disagree on verbose state\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
}

func TestManagerRecoveryWithLauncherButNoBinaryKeepsMenuSane(t *testing.T) {
	env := managerEnv(t)
	launcherPath := envValue(env, "LAUNCHER_PATH")
	if launcherPath == "" {
		t.Fatal("LAUNCHER_PATH not found in env")
	}
	writeFile(t, launcherPath, "#!/bin/sh\nexit 0\n", 0o755)

	menuOut := waitForMenuText(t, env, "2) Start proxy")
	if !strings.Contains(menuOut, "3) Enable autostart") {
		t.Fatalf("expected clean menu with launcher-only state, got:\n%s", menuOut)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "tmp bin   : not installed") || !strings.Contains(statusOut, "launcher  : "+launcherPath) {
		t.Fatalf("expected status to show launcher without binary, got:\n%s", statusOut)
	}
}

func TestManagerRecoveryWithInitScriptButNoPersistentBinaryCanReenableAutostart(t *testing.T) {
	env := managerEnv(t)
	initScriptPath := envValue(env, "INIT_SCRIPT_PATH")
	rcDir := envValue(env, "RC_D_DIR")
	if initScriptPath == "" || rcDir == "" {
		t.Fatal("missing init script paths in env")
	}

	writeFile(t, initScriptPath, "#!/bin/sh\nexit 0\n", 0o755)
	linkPath := filepath.Join(rcDir, "S95"+filepath.Base(initScriptPath))
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("mkdir rc.d: %v", err)
	}
	if err := os.Symlink(initScriptPath, linkPath); err != nil {
		t.Fatalf("symlink rc.d: %v", err)
	}

	menuOut := waitForMenuText(t, env, "3) Enable autostart")
	if !strings.Contains(menuOut, "autostart : disabled") {
		t.Fatalf("expected broken autostart not to look enabled, got:\n%s", menuOut)
	}

	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}
	buildFakeProxyBinary(t, binPath)

	out, err := runManager(t, env, "enable-autostart")
	if err != nil {
		t.Fatalf("enable-autostart repair failed: %v\n%s", err, out)
	}

	menuOut = waitForMenuText(t, env, "3) Disable autostart")
	if !strings.Contains(menuOut, "autostart : enabled") {
		t.Fatalf("expected repaired autostart to look enabled, got:\n%s", menuOut)
	}
}

func TestManagerRecoveryWithPersistentCopyButAutostartDisabled(t *testing.T) {
	env := managerEnv(t)
	persistDir := strings.Split(envValue(env, "PERSISTENT_DIR_CANDIDATES"), " ")[0]
	persistPathFile := envValue(env, "PERSIST_PATH_FILE")
	persistVersionFile := envValue(env, "PERSIST_VERSION_FILE")
	if persistDir == "" || persistPathFile == "" || persistVersionFile == "" {
		t.Fatal("missing persistent env paths")
	}

	buildFakeProxyBinary(t, filepath.Join(persistDir, "tg-ws-proxy"))
	writeFile(t, filepath.Join(persistDir, "tg-ws-proxy-go.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, persistPathFile, persistDir+"\n", 0o644)
	writeFile(t, persistVersionFile, "v9.9.9\n", 0o644)

	menuOut := waitForMenuText(t, env, "3) Enable autostart")
	if !strings.Contains(menuOut, "autostart : disabled") {
		t.Fatalf("expected persistent copy without rc enable to stay disabled, got:\n%s", menuOut)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "persist   : installed") || !strings.Contains(statusOut, "autostart : not configured") {
		t.Fatalf("expected status to show persistent-only state, got:\n%s", statusOut)
	}
}
