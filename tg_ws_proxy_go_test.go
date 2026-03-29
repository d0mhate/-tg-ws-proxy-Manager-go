package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

	writeFile(t, sourceBin, "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, sourceVersion, "v9.9.9\n", 0o644)
	writeFile(t, releaseAPI, "{\"tag_name\":\"v9.9.9\"}\n", 0o644)
	writeFile(t, rcCommonPath, "#!/bin/sh\nscript=\"$1\"\ncmd=\"$2\"\nname=\"$(basename \"$script\")\"\nrc_dir=\"${RC_D_DIR:-/etc/rc.d}\"\nmkdir -p \"$rc_dir\"\ncase \"$cmd\" in\nenable)\n  ln -sf \"$script\" \"$rc_dir/S95$name\"\n  ;;\ndisable)\n  rm -f \"$rc_dir\"/*\"$name\"\n  ;;\nstop)\n  exit 0\n  ;;\n*)\n  exit 0\n  ;;\nesac\n", 0o755)
	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='mipsel_24kc'\n", 0o644)

	env := append([]string{}, os.Environ()...)
	env = append(env,
		"RELEASE_API_URL=file://"+releaseAPI,
		"RELEASE_URL=file://"+sourceBin,
		"SOURCE_BIN="+sourceBin,
		"SOURCE_VERSION_FILE="+sourceVersion,
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

func runManager(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", append([]string{"tg-ws-proxy-go.sh"}, args...)...)
	cmd.Dir = "."
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runManagerMenu(t *testing.T, env []string, input string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", "tg-ws-proxy-go.sh")
	cmd.Dir = "."
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	return string(out), err
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

func TestMainMenuShowsSimplifiedActions(t *testing.T) {
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

func TestMainMenuReflectsAutostartState(t *testing.T) {
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
