package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	if strings.Contains(out, "pid       : 222") {
		t.Fatalf("expected false pgrep match to be filtered out, got:\n%s", out)
	}
}

func TestManagerStatusDetectsPersistentServiceViaPidofFallback(t *testing.T) {
	env := managerEnv(t)

	persistDir := strings.Split(envValue(env, "PERSISTENT_DIR_CANDIDATES"), " ")[0]
	persistPathFile := envValue(env, "PERSIST_PATH_FILE")
	persistVersionFile := envValue(env, "PERSIST_VERSION_FILE")
	if persistDir == "" || persistPathFile == "" || persistVersionFile == "" {
		t.Fatal("missing persistent env paths")
	}

	persistBin := filepath.Join(persistDir, "tg-ws-proxy")
	buildFakeProxyBinary(t, persistBin)
	writeFile(t, filepath.Join(persistDir, "tg-ws-proxy-go.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, persistPathFile, persistDir+"\n", 0o644)
	writeFile(t, persistVersionFile, "v9.9.9\n", 0o644)

	cmd := exec.Command(persistBin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start persistent fake proxy: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	fakeBinDir := t.TempDir()
	writeFile(t, filepath.Join(fakeBinDir, "pgrep"), "#!/bin/sh\nexit 1\n", 0o755)
	writeFile(t, filepath.Join(fakeBinDir, "pidof"), fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"tg-ws-proxy\" ]; then\n  printf '%d\\n'\nfi\n", cmd.Process.Pid), 0o755)
	env = setEnvValue(env, "PATH", fakeBinDir+":"+envValue(env, "PATH"))

	out, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "process   : running") {
		t.Fatalf("expected pidof fallback to detect running persistent service, got:\n%s", out)
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
		!strings.Contains(out, "4) Advanced") {
		t.Fatalf("expected simplified top-level menu, got:\n%s", out)
	}

	if strings.Contains(out, "Show quick commands") || strings.Contains(out, "Remove binary and runtime files") {
		t.Fatalf("expected advanced-only actions to be absent from top-level menu:\n%s", out)
	}
	if !strings.Contains(out, "track: release/latest") {
		t.Fatalf("expected top-level menu to show default track, got:\n%s", out)
	}
}

func TestManagerMainMenuShowsPreviewSourceSummary(t *testing.T) {
	env := managerEnv(t)

	updateChannelFile := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchFile := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelFile == "" || previewBranchFile == "" {
		t.Fatal("preview source state files missing from env")
	}

	writeFile(t, updateChannelFile, "preview\n", 0o644)
	writeFile(t, previewBranchFile, "feature/auth-flow\n", 0o644)

	out, err := runManagerMenu(t, env, "\n")
	if err != nil {
		t.Fatalf("menu failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "track: preview/feature/auth-flow") {
		t.Fatalf("expected top-level menu to show preview track, got:\n%s", out)
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
	if !strings.Contains(out, "2) Start proxy") {
		t.Fatalf("expected stopped terminal action label, got:\n%s", out)
	}
	if !strings.Contains(out, "proxy: stopped") {
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
	if !strings.Contains(out, "autostart: enabled") {
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
	if !strings.Contains(out, "autostart: disabled") {
		t.Fatalf("expected disabled autostart summary, got:\n%s", out)
	}
}

func TestManagerShowTelegramSettingsViaTopLevelMenu(t *testing.T) {
	env := append(managerEnv(t), "SOCKS_USERNAME=alice", "SOCKS_PASSWORD=secret")

	out, err := runManagerMenu(t, env, "4\n2\n\n\n")
	if err != nil {
		t.Fatalf("top-level telegram settings failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Telegram SOCKS5") ||
		!strings.Contains(out, "username : alice") ||
		!strings.Contains(out, "password : <set>") {
		t.Fatalf("expected telegram settings screen with auth values, got:\n%s", out)
	}
}
