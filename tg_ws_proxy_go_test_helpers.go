package main

import (
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

func copyManagerBundle(t *testing.T, destScript string) {
	t.Helper()

	managerScript, err := os.ReadFile("tg-ws-proxy-go.sh")
	if err != nil {
		t.Fatalf("read manager script: %v", err)
	}
	writeFile(t, destScript, string(managerScript), 0o755)

	entries, err := os.ReadDir("lib")
	if err != nil {
		t.Fatalf("read lib dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		srcPath := filepath.Join("lib", entry.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read %s: %v", srcPath, err)
		}
		writeFile(t, filepath.Join(filepath.Dir(destScript), "lib", entry.Name()), string(data), 0o644)
	}
}

func managerEnv(t *testing.T) []string {
	t.Helper()

	root := t.TempDir()
	sourceBin := filepath.Join(root, "source", "tg-ws-proxy-openwrt")
	sourceVersion := sourceBin + ".version"
	sourceManager := filepath.Join(root, "source", "tg-ws-proxy-go.sh")
	managerScriptPath := filepath.Join(root, "manager", "tg-ws-proxy-go.sh")
	releaseAPI := filepath.Join(root, "release.json")
	releasesAPI := filepath.Join(root, "releases.json")
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
	writeFile(t, releasesAPI, "[{\"tag_name\":\"v1.1.30\"},{\"tag_name\":\"v1.1.29\"},{\"tag_name\":\"v1.1.28\"},{\"tag_name\":\"v1.1.27\"},{\"tag_name\":\"v1.1.25\"}]\n", 0o644)
	writeFile(t, rcCommonPath, "#!/bin/sh\nscript=\"$1\"\ncmd=\"$2\"\nname=\"$(basename \"$script\")\"\nrc_dir=\"${RC_D_DIR:-/etc/rc.d}\"\nmkdir -p \"$rc_dir\"\ncase \"$cmd\" in\nenable)\n  ln -sf \"$script\" \"$rc_dir/S95$name\"\n  ;;\ndisable)\n  rm -f \"$rc_dir\"/*\"$name\"\n  ;;\nstart|restart)\n  marker=\"${FAKE_INIT_START_MARKER:-}\"\n  if [ -n \"$marker\" ]; then\n    mkdir -p \"$(dirname \"$marker\")\"\n    : > \"$marker\"\n  fi\n  ;;\nstop)\n  exit 0\n  ;;\n*)\n  exit 0\n  ;;\nesac\n", 0o755)
	writeFile(t, openwrtRelease, "DISTRIB_ID='OpenWrt'\nDISTRIB_ARCH='mipsel_24kc'\n", 0o644)
	copyManagerBundle(t, managerScriptPath)
	copyManagerBundle(t, scriptReleasePath)

	env := append([]string{}, os.Environ()...)
	env = append(env,
		"RELEASE_API_URL=file://"+releaseAPI,
		"RELEASES_API_URL=file://"+releasesAPI,
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
		"PERSIST_RELEASE_TAG_FILE="+filepath.Join(persistStateDir, "release_tag"),
		"PERSIST_UPDATE_CHANNEL_FILE="+filepath.Join(persistStateDir, "update_channel"),
		"PERSIST_PREVIEW_BRANCH_FILE="+filepath.Join(persistStateDir, "preview_branch"),
		"LATEST_VERSION_CACHE_FILE="+filepath.Join(persistStateDir, "latest_version_cache"),
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

func runManagerWithInput(t *testing.T, env []string, input string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("sh", append([]string{managerScriptPath(env)}, args...)...)
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

func buildPortHoldingProxyBinary(t *testing.T, path string) {
	t.Helper()

	source := filepath.Join(t.TempDir(), "main.go")
	writeFile(t, source, `package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := os.Getenv("HOLD_ADDR")
	if addr == "" {
		log.Fatal("HOLD_ADDR is required")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

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
		t.Fatalf("build port-holding fake proxy binary: %v\n%s", err, string(out))
	}
}

func writeModeAwareProxyScript(t *testing.T, path string) {
	t.Helper()

	source := filepath.Join(t.TempDir(), "main.go")
	writeFile(t, source, `package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if os.Getenv("PROXY_TEST_MODE") == "exit" {
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
}
`, 0o644)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir mode-aware proxy dir: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", path, source)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build mode-aware proxy binary: %v\n%s", err, string(out))
	}
}

func writeCapturingProxyScript(t *testing.T, path string) {
	t.Helper()

	source := filepath.Join(t.TempDir(), "main.go")
	writeFile(t, source, `package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	if argsFile := os.Getenv("ARGS_FILE"); argsFile != "" {
		_ = os.MkdirAll(filepath.Dir(argsFile), 0o755)
		_ = os.WriteFile(argsFile, []byte(joinArgs(os.Args[1:])), 0o644)
	}

	if os.Getenv("PROXY_TEST_MODE") == "exit" {
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := args[0]
	for _, arg := range args[1:] {
		out += "\n" + arg
	}
	return out
}
`, 0o644)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir capturing proxy dir: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", path, source)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build capturing proxy binary: %v\n%s", err, string(out))
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

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for file %s", path)
}
