package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cfconfig "tg-ws-proxy/internal/config"
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
		!strings.Contains(out, "2) Run proxy in terminal") ||
		!strings.Contains(out, "3) Enable autostart") ||
		!strings.Contains(out, "5) Advanced") ||
		!strings.Contains(out, "6) Start in background") {
		t.Fatalf("expected simplified top-level menu, got:\n%s", out)
	}

	if strings.Contains(out, "Show quick commands") || strings.Contains(out, "Remove binary and runtime files") {
		t.Fatalf("expected advanced-only actions to be absent from top-level menu:\n%s", out)
	}
	if !strings.Contains(out, "track     : release/latest") {
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

	if !strings.Contains(out, "track     : preview/feature/auth-flow") {
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

	out := waitForMenuText(t, env, "2) Run proxy in terminal")
	if !strings.Contains(out, "2) Run proxy in terminal") {
		t.Fatalf("expected stopped terminal action label, got:\n%s", out)
	}
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

func TestManagerShowTelegramSettingsViaTopLevelMenu(t *testing.T) {
	env := append(managerEnv(t), "SOCKS_USERNAME=alice", "SOCKS_PASSWORD=secret")

	out, err := runManagerMenu(t, env, "4\n\n\n")
	if err != nil {
		t.Fatalf("top-level telegram settings failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Telegram SOCKS5") ||
		!strings.Contains(out, "username : alice") ||
		!strings.Contains(out, "password : <set>") {
		t.Fatalf("expected telegram settings screen with auth values, got:\n%s", out)
	}
}

func TestManagerConfigureDCIPMappingViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "5\n7\n203:91.105.192.100, 2:149.154.167.220\n\n\n")
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

	out, err := runManagerMenu(t, env, "5\n7\ndefault\n\n\n")
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

	out, err := runManagerMenu(t, env, "5\n11\nexample.com\n\n\n")
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

	out, err := runManagerMenu(t, env, "5\n9\n\n\n")
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

	out, err := runManagerMenu(t, env, "5\n10\n\n\n")
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

	out, err := runManagerMenu(t, env, "4\n\n\n")
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
	if err != nil && !strings.Contains(out, "cf proxy") {
		t.Fatalf("expected cf proxy in summary, got:\n%s", out)
	}
	if !strings.Contains(out, "cf proxy") {
		t.Fatalf("expected cf proxy in main menu summary, got:\n%s", out)
	}
	if !strings.Contains(out, "cf order") {
		t.Fatalf("expected cf order in main menu summary, got:\n%s", out)
	}
}

func TestManagerCFDomainClearViaAdvancedMenu(t *testing.T) {
	env := append(managerEnv(t), "CF_PROXY=1", "CF_DOMAIN=example.com")
	configPath := envValue(env, "PERSIST_CONFIG_FILE")
	if configPath == "" {
		t.Fatal("PERSIST_CONFIG_FILE not found in env")
	}

	out, err := runManagerMenu(t, env, "5\n11\nclear\n\n\n")
	if err != nil {
		t.Fatalf("clear cf domain failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Cloudflare domain reset to default") {
		t.Fatalf("expected reset confirmation, got:\n%s", out)
	}

	config := readTrimmed(t, configPath)
	if !strings.Contains(config, fmt.Sprintf("CF_DOMAIN='%s'", cfconfig.DefaultCFDomain)) {
		t.Fatalf("expected CF_DOMAIN to reset to default, got:\n%s", config)
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

	out, err := runManagerMenu(t, env, "5\n12\n\n\n\n")
	if err != nil {
		t.Fatalf("check cf domain failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Checking example.com") ||
		!strings.Contains(out, "kws1.example.com") ||
		!strings.Contains(out, "kws203.example.com") ||
		!strings.Contains(out, "Cloudflare proxy: all tested hosts support websocket upgrade") ||
		!strings.Contains(out, "tcp ok | tls ok | ws upgrade ok") {
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

	out, err := runManagerMenu(t, env, "5\n12\n\n\n\n")
	if err != nil {
		t.Fatalf("check cf domain (curl fallback) failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Checking example.com") ||
		!strings.Contains(out, "kws1.example.com") ||
		!strings.Contains(out, "kws203.example.com") ||
		!strings.Contains(out, "Cloudflare proxy: all tested hosts support websocket upgrade") ||
		!strings.Contains(out, "tcp ok | tls ok | ws upgrade ok") {
		t.Fatalf("unexpected cf domain check output (curl fallback):\n%s", out)
	}
}

func TestManagerAdvancedShowFullStatusViaMenu(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}
	buildFakeProxyBinary(t, binPath)

	out, err := runManagerMenu(t, env, "5\n1\n\n\n")
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

	out, err := runManagerMenu(t, env, "5\n4\n\n\n")
	if err != nil {
		t.Fatalf("advanced quick commands screen failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Quick commands") ||
		!strings.Contains(out, "sh "+managerScriptPath(env)+" quick") ||
		!strings.Contains(out, "sh "+managerScriptPath(env)+" telegram") {
		t.Fatalf("expected quick commands screen from advanced menu, got:\n%s", out)
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

func TestManagerStartBackgroundCanRestartAlreadyRunningProxy(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}
	buildFakeProxyBinary(t, binPath)

	firstOut, firstErr := runManager(t, env, "start-background")
	if firstErr != nil {
		t.Fatalf("initial start-background failed: %v\n%s", firstErr, firstOut)
	}
	pidFile := filepath.Join(envValue(env, "INSTALL_DIR"), "pid")
	firstPID := readTrimmed(t, pidFile)

	out, err := runManagerWithInput(t, env, "y\n\n", "start-background")
	if err != nil {
		t.Fatalf("restarting running proxy via start-background failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "tg-ws-proxy is already running") {
		t.Fatalf("expected already-running message, got:\n%s", out)
	}
	if !strings.Contains(out, "Stop it and start again? [y/N]:") {
		t.Fatalf("expected restart prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Background process pid:") {
		t.Fatalf("expected restarted background pid output, got:\n%s", out)
	}

	secondPID := readTrimmed(t, pidFile)
	if secondPID == "" || secondPID == firstPID {
		t.Fatalf("expected a new background pid after restart, first=%q second=%q\n%s", firstPID, secondPID, out)
	}

	stopOut, stopErr := runManager(t, env, "stop")
	if stopErr != nil {
		t.Fatalf("stop failed after restart flow: %v\n%s", stopErr, stopOut)
	}
}

func TestManagerStartCanRestartAlreadyRunningProxy(t *testing.T) {
	env := setEnvValue(managerEnv(t), "PROXY_TEST_MODE", "hold")
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}
	writeModeAwareProxyScript(t, binPath)

	firstOut, firstErr := runManager(t, env, "start-background")
	if firstErr != nil {
		t.Fatalf("initial start-background failed: %v\n%s", firstErr, firstOut)
	}
	menuOut := waitForMenuText(t, env, "2) Stop proxy")
	if !strings.Contains(menuOut, "proxy     : running") {
		t.Fatalf("expected proxy to be running before restart prompt test, got:\n%s", menuOut)
	}

	env = setEnvValue(env, "PROXY_TEST_MODE", "exit")
	out, err := runManagerWithInput(t, env, "y\n\n", "start")
	if err != nil {
		t.Fatalf("restarting running proxy via start failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "tg-ws-proxy is already running") {
		t.Fatalf("expected already-running message, got:\n%s", out)
	}
	if !strings.Contains(out, "Stop it and start again? [y/N]:") {
		t.Fatalf("expected restart prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Starting tg-ws-proxy in terminal") {
		t.Fatalf("expected terminal start output after restart, got:\n%s", out)
	}
	if !strings.Contains(out, "tg-ws-proxy exited with code 0") {
		t.Fatalf("expected restarted terminal proxy to exit cleanly, got:\n%s", out)
	}

	statusOut, statusErr := runManager(t, env, "status")
	if statusErr != nil {
		t.Fatalf("status failed after terminal restart flow: %v\n%s", statusErr, statusOut)
	}
	if !strings.Contains(statusOut, "process   : stopped") {
		t.Fatalf("expected no running proxy after terminal restart flow, got:\n%s", statusOut)
	}
}

func TestManagerStartBackgroundStartsProxyAndMenuShowsStop(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	buildFakeProxyBinary(t, binPath)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Starting tg-ws-proxy in background") {
		t.Fatalf("expected background start output, got:\n%s", out)
	}
	if !strings.Contains(out, "Background process pid:") {
		t.Fatalf("expected background pid output, got:\n%s", out)
	}

	menuOut := waitForMenuText(t, env, "2) Stop proxy")
	if !strings.Contains(menuOut, "proxy     : running") {
		t.Fatalf("expected menu to show running proxy after background start, got:\n%s", menuOut)
	}

	stopOut, err := runManager(t, env, "stop")
	if err != nil {
		t.Fatalf("stop after background start failed: %v\n%s", err, stopOut)
	}

	menuOut = waitForMenuText(t, env, "2) Run proxy in terminal")
	if !strings.Contains(menuOut, "proxy     : stopped") {
		t.Fatalf("expected menu to show stopped proxy after background stop, got:\n%s", menuOut)
	}
}

func TestManagerMenuBackgroundStartThenStopProxySameSession(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	buildFakeProxyBinary(t, binPath)

	out, err := runManagerMenu(t, env, "6\n\n2\n\n\n")
	if err != nil {
		t.Fatalf("same-session background start then stop failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Background process pid:") {
		t.Fatalf("expected background start output, got:\n%s", out)
	}
	if !strings.Contains(out, "Proxy stopped") {
		t.Fatalf("expected stop confirmation in same menu session, got:\n%s", out)
	}
	if !strings.Contains(out, "2) Run proxy in terminal") {
		t.Fatalf("expected menu to return to stopped action label, got:\n%s", out)
	}
}

func TestManagerStartBackgroundPassesOptionalAuthFlags(t *testing.T) {
	env := append(managerEnv(t), "SOCKS_USERNAME=alice", "SOCKS_PASSWORD=secret")
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(argsFile); statErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--username") || !strings.Contains(args, "alice") || !strings.Contains(args, "--password") || !strings.Contains(args, "secret") {
		t.Fatalf("expected background start to pass auth flags, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after auth background start failed: %v", err)
	}
}

func TestManagerStartBackgroundPassesCustomDCIPFlags(t *testing.T) {
	env := append(managerEnv(t), "DC_IPS=203:91.105.192.100, 2:149.154.167.220")
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--dc-ip") ||
		!strings.Contains(args, "203:91.105.192.100") ||
		!strings.Contains(args, "2:149.154.167.220") {
		t.Fatalf("expected background start to pass dc-ip flags, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after dc-ip background start failed: %v", err)
	}
}

func TestManagerStartBackgroundPassesCloudflareFlags(t *testing.T) {
	env := append(managerEnv(t), "CF_PROXY=1", "CF_DOMAIN=example.com")
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--cf-proxy") || !strings.Contains(args, "--cf-domain") || !strings.Contains(args, "example.com") {
		t.Fatalf("expected background start to pass cloudflare flags, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after cloudflare background start failed: %v", err)
	}
}

func TestManagerStartBackgroundPassesCloudflareFirstFlag(t *testing.T) {
	env := append(managerEnv(t), "CF_PROXY=1", "CF_PROXY_FIRST=1", "CF_DOMAIN=example.com")
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background with cf first failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)
	if !strings.Contains(args, "--cf-proxy-first") {
		t.Fatalf("expected background start to pass --cf-proxy-first, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after cloudflare-first background start failed: %v", err)
	}
}

func TestManagerStartBackgroundOmitsAuthFlagsWhenUnset(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(argsFile); statErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	args := readTrimmed(t, argsFile)
	if strings.Contains(args, "--username") || strings.Contains(args, "--password") {
		t.Fatalf("expected background start without auth to omit auth flags, got args:\n%s", args)
	}

	if _, err := runManager(t, env, "stop"); err != nil {
		t.Fatalf("stop after no-auth background start failed: %v", err)
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

	out := waitForMenuText(t, env, "2) Run proxy in terminal")
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
	if !strings.Contains(menuOut, "track     : release/latest") || !strings.Contains(statusOut, "src mode  : release") || !strings.Contains(statusOut, "ref       : latest") {
		t.Fatalf("menu/status disagree on update track\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
}

func TestManagerRecoveryWithLauncherButNoBinaryKeepsMenuSane(t *testing.T) {
	env := managerEnv(t)
	launcherPath := envValue(env, "LAUNCHER_PATH")
	if launcherPath == "" {
		t.Fatal("LAUNCHER_PATH not found in env")
	}
	writeFile(t, launcherPath, "#!/bin/sh\nexit 0\n", 0o755)

	menuOut := waitForMenuText(t, env, "2) Run proxy in terminal")
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
