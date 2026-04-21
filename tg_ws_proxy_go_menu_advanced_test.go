package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerAdvancedShowFullStatusViaMenu(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}
	buildFakeProxyBinary(t, binPath)

	out, err := runManagerMenu(t, env, "4\n1\n\n\n")
	if err != nil {
		t.Fatalf("advanced status screen failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Status") ||
		!strings.Contains(out, "tmp bin   : installed") ||
		!strings.Contains(out, "process   : stopped") {
		t.Fatalf("expected full status screen from advanced menu, got:\n%s", out)
	}
}

func TestManagerAdvancedShowQuickCommandsViaMenu(t *testing.T) {
	env := managerEnv(t)

	out, err := runManagerMenu(t, env, "4\n3\n\n\n")
	if err != nil {
		t.Fatalf("advanced quick commands screen failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Quick commands") ||
		!strings.Contains(out, "sh "+managerScriptPath(env)+" quick") ||
		!strings.Contains(out, "sh "+managerScriptPath(env)+" telegram") {
		t.Fatalf("expected quick commands screen from advanced menu, got:\n%s", out)
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

	removeOut, err := runManagerMenu(t, env, "4\n17\n\n\n\n")
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

	menuOut, err := runManagerMenu(t, env, "4\n4\n\n\n")
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

	out := waitForMenuText(t, checkEnv, "verbose: on")
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
	if !strings.Contains(menuOut, "autostart: disabled") {
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

func TestManagerDisableAutostartPreservesPinnedReleaseTag(t *testing.T) {
	env := managerEnv(t)
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if releaseTagPath == "" {
		t.Fatal("PERSIST_RELEASE_TAG_FILE not found in env")
	}

	writeFile(t, releaseTagPath, "v1.1.25\n", 0o644)

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	out, err := runManager(t, env, "disable-autostart")
	if err != nil {
		t.Fatalf("disable-autostart failed: %v\n%s", err, out)
	}
	if got := readTrimmed(t, releaseTagPath); got != "v1.1.25" {
		t.Fatalf("expected pinned release tag to survive disable-autostart, got %q", got)
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
	if !strings.Contains(menuOut, "autostart: disabled") {
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
	if !strings.Contains(menuOut, "autostart: enabled") {
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
	if !strings.Contains(menuOut, "autostart: disabled") {
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
