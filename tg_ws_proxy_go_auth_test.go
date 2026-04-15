package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerEnableAutostartInstallsPersistentCopy(t *testing.T) {
	env := managerEnv(t)
	startMarker := filepath.Join(t.TempDir(), "service-started")
	env = append(env, "FAKE_INIT_START_MARKER="+startMarker)

	out, err := runManager(t, env, "enable-autostart")
	if err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Persistent copy installed automatically") || !strings.Contains(out, "Autostart enabled") {
		t.Fatalf("expected success output, got:\n%s", out)
	}
	if _, err := os.Stat(startMarker); err != nil {
		t.Fatalf("expected init.d service start marker to be created: %v", err)
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

func TestManagerAutostartConfigPersistsOptionalAuthCredentials(t *testing.T) {
	env := append(managerEnv(t), "SOCKS_USERNAME=alice", "SOCKS_PASSWORD=secret")

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}
	initScriptPath := envValue(env, "INIT_SCRIPT_PATH")
	if initScriptPath == "" {
		t.Fatal("INIT_SCRIPT_PATH not found in env")
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "USERNAME='alice'") || !strings.Contains(config, "PASSWORD='secret'") {
		t.Fatalf("expected auth credentials to be persisted in autostart config, got:\n%s", config)
	}

	initScript := readTrimmed(t, initScriptPath)
	if !strings.Contains(initScript, `--username "$USERNAME" --password "$PASSWORD"`) {
		t.Fatalf("expected init script to pass auth flags when configured, got:\n%s", initScript)
	}
}

func TestManagerTelegramSettingsLoadSavedAuthCredentials(t *testing.T) {
	env := append(managerEnv(t), "SOCKS_USERNAME=alice", "SOCKS_PASSWORD=secret")

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	checkEnv := unsetEnvValue(unsetEnvValue(env, "SOCKS_USERNAME"), "SOCKS_PASSWORD")

	out, err := runManager(t, checkEnv, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "username : alice") || !strings.Contains(out, "password : <set>") {
		t.Fatalf("expected telegram settings to load saved auth credentials, got:\n%s", out)
	}
}

func TestManagerConfigureSocksAuthViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n10\nalice\nsecret\n\n\n")
	if err != nil {
		t.Fatalf("configure socks auth via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "SOCKS5 auth saved") {
		t.Fatalf("expected auth saved message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "USERNAME='alice'") || !strings.Contains(config, "PASSWORD='secret'") {
		t.Fatalf("expected configured auth credentials in settings file, got:\n%s", config)
	}

	settingsOut, err := runManager(t, env, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, settingsOut)
	}
	if !strings.Contains(settingsOut, "username : alice") || !strings.Contains(settingsOut, "password : <set>") {
		t.Fatalf("expected telegram settings to show configured auth, got:\n%s", settingsOut)
	}
}

func TestManagerConfigureSocksAuthCanBeClearedViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	if out, err := runManagerMenu(t, env, "4\n10\nalice\nsecret\n\n\n"); err != nil {
		t.Fatalf("initial configure socks auth via menu failed: %v\n%s", err, out)
	}

	out, err := runManagerMenu(t, env, "4\n10\n\n\n")
	if err != nil {
		t.Fatalf("clear socks auth via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "SOCKS5 auth disabled") {
		t.Fatalf("expected auth disabled message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "USERNAME=''") || !strings.Contains(config, "PASSWORD=''") {
		t.Fatalf("expected cleared auth credentials in settings file, got:\n%s", config)
	}

	settingsOut, err := runManager(t, env, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, settingsOut)
	}
	if !strings.Contains(settingsOut, "username : <empty>") || !strings.Contains(settingsOut, "password : <empty>") {
		t.Fatalf("expected telegram settings to show cleared auth, got:\n%s", settingsOut)
	}
}

func TestManagerConfigureSocksAuthRejectsEmptyPasswordViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n10\nalice\n\n\n\n")
	if err != nil {
		t.Fatalf("configure socks auth with empty password failed unexpectedly: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Password cannot be empty when username is set") {
		t.Fatalf("expected empty password validation message, got:\n%s", out)
	}

	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no settings file to be created after failed auth config, stat err=%v", statErr)
	}
}

func TestManagerConfigureSocksAuthOffersRestartAndAppliesIt(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	if out, err := runManager(t, env, "start-background"); err != nil {
		t.Fatalf("initial start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	initialArgs := readTrimmed(t, argsFile)
	if strings.Contains(initialArgs, "--username") || strings.Contains(initialArgs, "--password") {
		t.Fatalf("expected initial background start without auth flags, got args:\n%s", initialArgs)
	}

	out, err := runManagerMenu(t, env, "4\n10\nalice\nsecret\ny\n\n\n")
	if err != nil {
		t.Fatalf("configure socks auth with restart failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Restart now to apply the new settings? [y/N]:") {
		t.Fatalf("expected restart prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Proxy restarted with the updated settings") {
		t.Fatalf("expected successful restart message, got:\n%s", out)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		args := readTrimmed(t, argsFile)
		if strings.Contains(args, "--username") && strings.Contains(args, "alice") &&
			strings.Contains(args, "--password") && strings.Contains(args, "secret") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--username") || !strings.Contains(args, "alice") ||
		!strings.Contains(args, "--password") || !strings.Contains(args, "secret") {
		t.Fatalf("expected restarted proxy to use updated auth flags, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after restarted auth background start failed: %v", err)
	}
}

func TestManagerConfigureSocksAuthCanSkipRestartPrompt(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	if out, err := runManager(t, env, "start-background"); err != nil {
		t.Fatalf("initial start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	out, err := runManagerMenu(t, env, "4\n10\nalice\nsecret\n\n\n\n")
	if err != nil {
		t.Fatalf("configure socks auth without restart failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Restart now to apply the new settings? [y/N]:") {
		t.Fatalf("expected restart prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Restart skipped. New settings will apply on the next start.") {
		t.Fatalf("expected restart skipped message, got:\n%s", out)
	}

	args := readTrimmed(t, argsFile)
	if strings.Contains(args, "--username") || strings.Contains(args, "--password") {
		t.Fatalf("expected running proxy to keep old no-auth args after restart skip, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after skipped auth restart failed: %v", err)
	}
}

func TestManagerMenuBackgroundStartThenConfigureSocksAuthOffersRestart(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManagerMenu(t, env, "2\nb\n\n4\n10\nalice\nsecret\ny\n\n\n")
	if err != nil {
		t.Fatalf("menu background start then configure socks auth failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Background process pid:") {
		t.Fatalf("expected background start output, got:\n%s", out)
	}
	if !strings.Contains(out, "Restart now to apply the new settings? [y/N]:") {
		t.Fatalf("expected restart prompt after configuring auth from same menu session, got:\n%s", out)
	}
	if !strings.Contains(out, "Proxy restarted with the updated settings") {
		t.Fatalf("expected restart success message, got:\n%s", out)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		args := readTrimmed(t, argsFile)
		if strings.Contains(args, "--username") && strings.Contains(args, "alice") &&
			strings.Contains(args, "--password") && strings.Contains(args, "secret") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--username") || !strings.Contains(args, "alice") ||
		!strings.Contains(args, "--password") || !strings.Contains(args, "secret") {
		t.Fatalf("expected restarted proxy from same menu session to use updated auth flags, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after same-session auth restart failed: %v", err)
	}
}

func TestManagerEnableAutostartRejectsPartialAuthSettings(t *testing.T) {
	env := append(managerEnv(t), "SOCKS_USERNAME=alice")

	out, err := runManager(t, env, "enable-autostart")
	if err == nil {
		t.Fatalf("expected enable-autostart to reject partial auth settings:\n%s", out)
	}
	if !strings.Contains(out, "SOCKS5 auth settings are incomplete") {
		t.Fatalf("expected partial auth validation error, got:\n%s", out)
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
