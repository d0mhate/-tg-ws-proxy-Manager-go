package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerConfigureDCIPMappingViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n12\n203:91.105.192.100, 2:149.154.167.220\n\n\n")
	if err != nil {
		t.Fatalf("configure dc mapping failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Telegram DC mapping saved") {
		t.Fatalf("expected success message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "DC_IPS='203:91.105.192.100, 2:149.154.167.220'") {
		t.Fatalf("expected dc mapping to be persisted, got:\n%s", config)
	}
}

func TestManagerConfigureDCIPMappingCanResetToDefaults(t *testing.T) {
	env := append(managerEnv(t), "DC_IPS=203:91.105.192.100")
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	if out, err := runManager(t, env, "status"); err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}

	out, err := runManagerMenu(t, env, "4\n12\ndefault\n\n\n")
	if err != nil {
		t.Fatalf("reset dc mapping failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Telegram DC mapping reset to defaults") {
		t.Fatalf("expected reset confirmation, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "DC_IPS=''") {
		t.Fatalf("expected persisted dc mapping to be cleared, got:\n%s", config)
	}
}

func TestManagerCFDomainPersistedViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n9\nexample.com\n\n\n")
	if err != nil {
		t.Fatalf("configure cf domain failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Cloudflare domain saved") {
		t.Fatalf("expected success message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "CF_DOMAIN='example.com'") {
		t.Fatalf("expected CF_DOMAIN to be persisted, got:\n%s", config)
	}
}

func TestManagerToggleCFProxyViaAdvancedMenu(t *testing.T) {
	env := setEnvValue(managerEnv(t), "CF_PROXY", "0")
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n6\n\n\n")
	if err != nil {
		t.Fatalf("toggle cf proxy failed: %v\n%s", err, out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "CF_PROXY='1'") {
		t.Fatalf("expected CF_PROXY toggled to 1 and persisted, got:\n%s\n%s", config, out)
	}
}

func TestManagerToggleCFProxyFirstViaAdvancedMenu(t *testing.T) {
	env := setEnvValue(managerEnv(t), "CF_PROXY_FIRST", "0")
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n7\n\n\n")
	if err != nil {
		t.Fatalf("toggle cf proxy first failed: %v\n%s", err, out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "CF_PROXY_FIRST='1'") {
		t.Fatalf("expected CF_PROXY_FIRST toggled to 1 and persisted, got:\n%s\n%s", config, out)
	}
}

func TestManagerCFSettingsShownInTelegramSettings(t *testing.T) {
	env := append(managerEnv(t), "CF_PROXY=1", "CF_DOMAIN=example.com")

	out, err := runManagerMenu(t, env, "4\n2\n\n\n")
	if err != nil {
		t.Fatalf("show settings failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "cf proxy : on") || !strings.Contains(out, "cf order : fallback") || !strings.Contains(out, "cf domain: example.com") {
		t.Fatalf("expected cf settings in telegram screen, got:\n%s", out)
	}
}

func TestManagerCFSettingsShownInSummary(t *testing.T) {
	env := append(managerEnv(t), "CF_PROXY=1")

	out, err := runManagerMenu(t, env, "\n")
	if err != nil && !strings.Contains(out, "CF") {
		t.Fatalf("expected CF info in summary, got:\n%s", out)
	}
	if !strings.Contains(out, "CF      on") {
		t.Fatalf("expected CF proxy on in main menu summary, got:\n%s", out)
	}
	if !strings.Contains(out, "fallback") {
		t.Fatalf("expected CF order in main menu summary, got:\n%s", out)
	}
}

func TestManagerCFDomainClearViaAdvancedMenu(t *testing.T) {
	env := append(managerEnv(t), "CF_PROXY=1", "CF_DOMAIN=example.com")
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n9\nclear\n\n\n")
	if err != nil {
		t.Fatalf("clear cf domain failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Cloudflare domain cleared") {
		t.Fatalf("expected cleared confirmation, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "CF_DOMAIN=''") {
		t.Fatalf("expected CF_DOMAIN to be cleared, got:\n%s", config)
	}
}

func TestManagerCFSettingsLoadedFromConfig(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	writeFile(t, configPath, "CF_PROXY='1'\nCF_DOMAIN='saved.example.com'\n", 0o644)

	out, err := runManager(t, env, "telegram")
	if err != nil {
		t.Fatalf("telegram command failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "cf proxy : on") || !strings.Contains(out, "cf order : fallback") || !strings.Contains(out, "saved.example.com") {
		t.Fatalf("expected cf settings loaded from config, got:\n%s", out)
	}
}

func TestManagerCheckCFDomainViaAdvancedMenu(t *testing.T) {
	env := append(managerEnv(t), "CF_DOMAIN=example.com")

	fakeBinDir := t.TempDir()
	writeFile(t, filepath.Join(fakeBinDir, "openssl"), "#!/bin/sh\nprintf 'HTTP/1.1 101 Switching Protocols\\r\\n\\r\\n'\n", 0o755)
	env = setEnvValue(env, "PATH", fakeBinDir+":"+envValue(env, "PATH"))

	out, err := runManagerMenu(t, env, "4\n10\n\n\n\n")
	if err != nil {
		t.Fatalf("check cf domain failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Checking Cloudflare websocket endpoints") ||
		!strings.Contains(out, "domain") ||
		!strings.Contains(out, "kws203") ||
		!strings.Contains(out, "example.com") ||
		!strings.Contains(out, "Cloudflare proxy: all tested hosts support websocket upgrade") ||
		!strings.Contains(out, "tls/ws ok") ||
		!strings.Contains(out, "6/6 ok") {
		t.Fatalf("unexpected cf domain check output:\n%s", out)
	}
}

func TestManagerCheckCFDomainViaCurlFallback(t *testing.T) {
	env := append(managerEnv(t), "CF_DOMAIN=example.com")

	fakeBinDir := t.TempDir()
	writeFile(t, filepath.Join(fakeBinDir, "curl"), "#!/bin/sh\nprintf 'HTTP/1.1 101 Switching Protocols\\r\\n\\r\\n'\n", 0o755)

	for _, tool := range []string{"sed", "grep", "awk", "timeout", "wc", "date"} {
		if toolPath, err := exec.LookPath(tool); err == nil {
			_ = os.Symlink(toolPath, filepath.Join(fakeBinDir, tool))
		}
	}

	origPath := envValue(env, "PATH")
	var filteredDirs []string
	for _, dir := range filepath.SplitList(origPath) {
		if _, err := os.Stat(filepath.Join(dir, "openssl")); err == nil {
			continue
		}
		filteredDirs = append(filteredDirs, dir)
	}
	env = setEnvValue(env, "PATH", fakeBinDir+":"+strings.Join(filteredDirs, ":"))

	out, err := runManagerMenu(t, env, "4\n10\n\n\n\n")
	if err != nil {
		t.Fatalf("check cf domain (curl fallback) failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Checking Cloudflare websocket endpoints") ||
		!strings.Contains(out, "domain") ||
		!strings.Contains(out, "kws203") ||
		!strings.Contains(out, "example.com") ||
		!strings.Contains(out, "Cloudflare proxy: all tested hosts support websocket upgrade") ||
		!strings.Contains(out, "tls/ws ok") ||
		!strings.Contains(out, "6/6 ok") {
		t.Fatalf("unexpected cf domain check output (curl fallback):\n%s", out)
	}
}

func TestManagerCheckCFDomainSplitsCommaSeparatedDomains(t *testing.T) {
	env := append(managerEnv(t), "CF_DOMAIN=one.example.com,two.example.com")

	fakeBinDir := t.TempDir()
	writeFile(t, filepath.Join(fakeBinDir, "openssl"), "#!/bin/sh\nprintf 'HTTP/1.1 101 Switching Protocols\\r\\n\\r\\n'\n", 0o755)
	env = setEnvValue(env, "PATH", fakeBinDir+":"+envValue(env, "PATH"))

	out, err := runManagerMenu(t, env, "4\n10\n\n\n\n")
	if err != nil {
		t.Fatalf("check cf domain with csv failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "one.example.com") ||
		!strings.Contains(out, "two.example.com") ||
		strings.Contains(out, "kws1.one.example.com,two.example.com") ||
		!strings.Contains(out, "Cloudflare proxy: all tested hosts support websocket upgrade") {
		t.Fatalf("unexpected csv cf domain check output:\n%s", out)
	}
}

func TestManagerConfigurePortViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	// 4 = Advanced, 13 = Port, enter new port, back, exit
	out, err := runManagerMenu(t, env, "4\n13\n2080\n\n\n")
	if err != nil {
		t.Fatalf("configure port failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Port saved: 2080") {
		t.Fatalf("expected success message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "PORT='2080'") {
		t.Fatalf("expected port to be persisted, got:\n%s", config)
	}
}

func TestManagerConfigurePortRejectsInvalidValues(t *testing.T) {
	env := managerEnv(t)

	for _, bad := range []string{"0", "65536", "abc", "-1"} {
		out, _ := runManagerMenu(t, env, "4\n13\n"+bad+"\n\n\n")
		if !strings.Contains(out, "Port must") {
			t.Errorf("expected validation error for port %q, got:\n%s", bad, out)
		}
	}
}

func TestManagerConfigurePoolSizeViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "4\n14\n8\n\n\n")
	if err != nil {
		t.Fatalf("configure pool size failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Pool size saved: 8") {
		t.Fatalf("expected success message, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, "POOL_SIZE='8'") {
		t.Fatalf("expected pool size to be persisted, got:\n%s", config)
	}
}

func TestManagerConfigurePoolSizeRejectsInvalidValues(t *testing.T) {
	env := managerEnv(t)

	for _, bad := range []string{"-1", "65", "abc"} {
		out, _ := runManagerMenu(t, env, "4\n14\n"+bad+"\n\n\n")
		if !strings.Contains(out, "Pool size must") {
			t.Errorf("expected validation error for pool size %q, got:\n%s", bad, out)
		}
	}
}

func TestManagerStartBackgroundUsesConfiguredPort(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile, "LISTEN_PORT=9999")

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--port") || !strings.Contains(args, "9999") {
		t.Errorf("expected --port 9999 in args, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundUsesConfiguredPoolSize(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile, "POOL_SIZE=7")

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--pool-size") || !strings.Contains(args, "7") {
		t.Errorf("expected --pool-size 7 in args, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}
