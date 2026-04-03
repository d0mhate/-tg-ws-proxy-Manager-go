package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerMTProtoSettingsDefaultDisabled(t *testing.T) {
	env := managerEnv(t)

	out, err := runManager(t, env, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "MTProto Proxy") {
		t.Fatalf("expected MTProto section to be hidden when disabled, got:\n%s", out)
	}
}

func TestManagerMTProtoSettingsShownWhenEnabled(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	out, err := runManager(t, env, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MTProto Proxy") {
		t.Fatalf("expected MTProto section when enabled, got:\n%s", out)
	}
	if !strings.Contains(out, "port     : 8443") {
		t.Fatalf("expected MTProto port in output, got:\n%s", out)
	}
	if !strings.Contains(out, "secret   : ee0123456789abcdef0123456789abcdef") {
		t.Fatalf("expected MTProto secret in output, got:\n%s", out)
	}
	if !strings.Contains(out, "tg://proxy?server=") {
		t.Fatalf("expected tg://proxy link in output, got:\n%s", out)
	}
}

func TestManagerMTProtoSettingsShowQRWhenRendererAvailable(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	writeQRProxyScript(t, envValue(env, "BIN_PATH"))

	out, err := runManager(t, env, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "qr       :") {
		t.Fatalf("expected QR label in output, got:\n%s", out)
	}
	if !strings.Contains(out, "██") {
		t.Fatalf("expected QR body in output, got:\n%s", out)
	}
}

func TestManagerMTProtoConfigPersisted(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=9443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	config := readTrimmed(t, configPath)

	if !strings.Contains(config, "MTPROTO_ENABLED='1'") {
		t.Fatalf("expected MTPROTO_ENABLED in config, got:\n%s", config)
	}
	if !strings.Contains(config, "MTPROTO_PORT='9443'") {
		t.Fatalf("expected MTPROTO_PORT in config, got:\n%s", config)
	}
	if !strings.Contains(config, "MTPROTO_SECRET='ee0123456789abcdef0123456789abcdef'") {
		t.Fatalf("expected MTPROTO_SECRET in config, got:\n%s", config)
	}
}

func TestManagerMTProtoConfigLoadedFromSaved(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=9443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	// Remove env overrides so settings must come from saved config.
	checkEnv := unsetEnvValue(unsetEnvValue(unsetEnvValue(env,
		"MTPROTO_ENABLED"), "MTPROTO_PORT"), "MTPROTO_SECRET")

	out, err := runManager(t, checkEnv, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MTProto Proxy") {
		t.Fatalf("expected MTProto section loaded from saved config, got:\n%s", out)
	}
	if !strings.Contains(out, "port     : 9443") {
		t.Fatalf("expected saved MTProto port, got:\n%s", out)
	}
}

func TestManagerMTProtoInitScriptIncludesFlags(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	initScriptPath := envValue(env, "INIT_SCRIPT_PATH")
	initScript := readTrimmed(t, initScriptPath)

	if !strings.Contains(initScript, "--mtproto") {
		t.Fatalf("expected --mtproto in init script, got:\n%s", initScript)
	}
	if !strings.Contains(initScript, "--mtproto-port") {
		t.Fatalf("expected --mtproto-port in init script, got:\n%s", initScript)
	}
	if !strings.Contains(initScript, "--mtproto-secret") {
		t.Fatalf("expected --mtproto-secret in init script, got:\n%s", initScript)
	}
}

func TestManagerMTProtoInitScriptGuardedByCondition(t *testing.T) {
	env := managerEnv(t) // MTProto disabled by default

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	initScriptPath := envValue(env, "INIT_SCRIPT_PATH")
	initScript := readTrimmed(t, initScriptPath)

	// The init script should guard MTProto flags with a condition.
	if !strings.Contains(initScript, `MTPROTO_ENABLED" = "1"`) {
		t.Fatalf("expected conditional guard for MTProto in init script, got:\n%s", initScript)
	}
}

func TestManagerMTProtoRunBinaryPassesFlags(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	if out, err := runManager(t, env, "start-background"); err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if !strings.Contains(args, "--mtproto") {
		t.Fatalf("expected --mtproto in binary args, got:\n%s", args)
	}
	if !strings.Contains(args, "--mtproto-port\n8443") {
		t.Fatalf("expected --mtproto-port 8443 in binary args, got:\n%s", args)
	}
	if !strings.Contains(args, "--mtproto-secret\nee0123456789abcdef0123456789abcdef") {
		t.Fatalf("expected --mtproto-secret in binary args, got:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestManagerMTProtoRunBinaryOmitsFlagsWhenDisabled(t *testing.T) {
	env := managerEnv(t) // MTProto disabled by default

	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	if out, err := runManager(t, env, "start-background"); err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if strings.Contains(args, "--mtproto") {
		t.Fatalf("expected no --mtproto in binary args when disabled, got:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestManagerMTProtoOnlyRunBinaryPassesModeFlags(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	if out, err := runManager(t, env, "start-mtproto-background"); err != nil {
		t.Fatalf("start-mtproto-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if !strings.Contains(args, "--socks5=false") {
		t.Fatalf("expected --socks5=false in mtproto-only args, got:\n%s", args)
	}
	if !strings.Contains(args, "--mtproto") {
		t.Fatalf("expected --mtproto in mtproto-only args, got:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestManagerConfigureMTProtoEnableViaMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	writeQRProxyScript(t, envValue(env, "BIN_PATH"))

	// Menu: 7 (MTProto proxy) → 2 (Configure MTProto) → y (enable) → Enter (default port) → Enter (back) → Enter (exit)
	out, err := runManagerMenu(t, env, "7\n2\ny\n\n\n\n")
	if err != nil {
		t.Fatalf("configure mtproto via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MTProto proxy enabled") {
		t.Fatalf("expected enabled message, got:\n%s", out)
	}
	if !strings.Contains(out, "tg://proxy?server=") {
		t.Fatalf("expected tg://proxy link in output, got:\n%s", out)
	}
	if !strings.Contains(out, "qr     :") {
		t.Fatalf("expected QR label in output, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "MTPROTO_ENABLED='1'") {
		t.Fatalf("expected MTPROTO_ENABLED=1 in config, got:\n%s", config)
	}
	if !strings.Contains(config, "MTPROTO_SECRET='ee") {
		t.Fatalf("expected auto-generated secret in config, got:\n%s", config)
	}
}

func TestManagerConfigureMTProtoDisableViaMenu(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")

	// Need to persist initial config first.
	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	// Menu: 7 (MTProto proxy) → 2 (Configure MTProto) → 1 (Disable) → Enter (back) → Enter (exit)
	out, err := runManagerMenu(t, env, "7\n2\n1\n\n\n")
	if err != nil {
		t.Fatalf("disable mtproto via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MTProto proxy disabled") {
		t.Fatalf("expected disabled message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "MTPROTO_ENABLED='0'") {
		t.Fatalf("expected MTPROTO_ENABLED=0 in config, got:\n%s", config)
	}
}

func TestManagerConfigureMTProtoChangePortViaMenu(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	// Menu: 7 (MTProto proxy) → 2 (Configure MTProto) → 2 (Change port) → 9443 → Enter (back) → Enter (exit)
	out, err := runManagerMenu(t, env, "7\n2\n2\n9443\n\n\n")
	if err != nil {
		t.Fatalf("change mtproto port via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MTProto port changed to 9443") {
		t.Fatalf("expected port changed message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "MTPROTO_PORT='9443'") {
		t.Fatalf("expected new port in config, got:\n%s", config)
	}
}

func TestManagerConfigureMTProtoRegenerateSecretViaMenu(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	// Menu: 7 (MTProto proxy) → 2 (Configure MTProto) → 3 (Regenerate secret) → Enter (back) → Enter (exit)
	out, err := runManagerMenu(t, env, "7\n2\n3\n\n\n")
	if err != nil {
		t.Fatalf("regenerate mtproto secret via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "New secret: ee") {
		t.Fatalf("expected new secret message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "MTPROTO_SECRET='ee") {
		t.Fatalf("expected new secret in config, got:\n%s", config)
	}
	// Should be different from the original secret.
	if strings.Contains(config, "MTPROTO_SECRET='ee0123456789abcdef0123456789abcdef'") {
		t.Fatalf("expected regenerated secret to differ from original, got:\n%s", config)
	}
}

func TestManagerConfigureMTProtoEnableWithCustomPort(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")

	// Menu: 7 (MTProto proxy) → 2 (Configure MTProto) → y (enable) → 7443 (custom port) → Enter (back) → Enter (exit)
	out, err := runManagerMenu(t, env, "7\n2\ny\n7443\n\n\n")
	if err != nil {
		t.Fatalf("enable mtproto with custom port via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "port   : 7443") {
		t.Fatalf("expected custom port in output, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "MTPROTO_PORT='7443'") {
		t.Fatalf("expected custom port in config, got:\n%s", config)
	}
}

func TestManagerConfigureMTProtoEnableOffersRestartWhenRunning(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	if out, err := runManager(t, env, "start-background"); err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}
	waitForFile(t, argsFile)

	// Menu: 7 → 2 → y (enable) → Enter (default port) → y (restart) → Enter (back) → Enter (exit)
	out, err := runManagerMenu(t, env, "7\n2\ny\n\ny\n\n\n")
	if err != nil {
		t.Fatalf("enable mtproto with restart via menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Restart now to apply the new settings? [y/N]:") {
		t.Fatalf("expected restart prompt, got:\n%s", out)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		args := readTrimmed(t, argsFile)
		if strings.Contains(args, "--mtproto") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--mtproto") {
		t.Fatalf("expected restarted proxy to include --mtproto flag, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestManagerTelegramCommandShowsMTProtoWhenEnabled(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	out, err := runManager(t, env, "telegram")
	if err != nil {
		t.Fatalf("telegram failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MTProto Proxy") {
		t.Fatalf("expected MTProto section in telegram output, got:\n%s", out)
	}
	if !strings.Contains(out, "tg://proxy?server=") {
		t.Fatalf("expected tg://proxy link, got:\n%s", out)
	}
}

func TestManagerMTProtoTopLevelMenuShowsOption(t *testing.T) {
	env := managerEnv(t)

	out, err := runManagerMenu(t, env, "\n")
	if err != nil {
		t.Fatalf("main menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "7) MTProto proxy") {
		t.Fatalf("expected MTProto top-level option, got:\n%s", out)
	}
}

func TestManagerRemoveClearsMTProtoConfig(t *testing.T) {
	env := append(managerEnv(t),
		"MTPROTO_ENABLED=1",
		"MTPROTO_PORT=8443",
		"MTPROTO_SECRET=ee0123456789abcdef0123456789abcdef",
	)

	if out, err := runManager(t, env, "enable-autostart"); err != nil {
		t.Fatalf("enable-autostart failed: %v\n%s", err, out)
	}

	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	persistStateDir := envValue(env, "PERSIST_STATE_DIR")

	// Verify config exists with MTProto settings.
	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "MTPROTO_ENABLED='1'") {
		t.Fatalf("expected MTProto in config before remove:\n%s", config)
	}

	if out, err := runManager(t, env, "remove"); err != nil {
		t.Fatalf("remove failed: %v\n%s", err, out)
	}

	// Entire persist state dir should be gone.
	if _, err := os.Stat(persistStateDir); !os.IsNotExist(err) {
		t.Fatalf("expected persist state dir to be removed, stat err=%v", err)
	}

	// Running telegram after remove should not show MTProto (no saved config).
	checkEnv := unsetEnvValue(unsetEnvValue(unsetEnvValue(env,
		"MTPROTO_ENABLED"), "MTPROTO_PORT"), "MTPROTO_SECRET")

	out, err := runManager(t, checkEnv, "telegram")
	if err != nil {
		t.Fatalf("telegram after remove failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "MTProto Proxy") {
		t.Fatalf("expected no MTProto section after remove, got:\n%s", out)
	}
}

func TestManagerMTProtoSecretGeneratedOnEnable(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")

	// Enable MTProto, accept default port.
	_, _ = runManagerMenu(t, env, "7\n2\ny\n\n\n\n")

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}

	config := readTrimmed(t, configPath)
	// Secret should start with 'ee' and be 34 chars total.
	idx := strings.Index(config, "MTPROTO_SECRET='ee")
	if idx < 0 {
		t.Fatalf("expected auto-generated secret starting with ee, got:\n%s", config)
	}
	// Extract the secret value.
	after := config[idx+len("MTPROTO_SECRET='"):]
	end := strings.Index(after, "'")
	if end < 0 {
		t.Fatalf("malformed config line:\n%s", config)
	}
	secret := after[:end]
	if len(secret) != 34 { // "ee" + 32 hex chars
		t.Fatalf("expected 34-char secret, got %d chars: %s", len(secret), secret)
	}
}
