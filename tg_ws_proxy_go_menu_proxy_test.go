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
)

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
	if !strings.Contains(menuOut, "proxy: running") {
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
	if !strings.Contains(out, "Binary path:") {
		t.Fatalf("expected terminal start output to include binary path, got:\n%s", out)
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
	if !strings.Contains(out, "Binary path:") {
		t.Fatalf("expected background start output to include binary path, got:\n%s", out)
	}
	if !strings.Contains(out, "Background process pid:") {
		t.Fatalf("expected background pid output, got:\n%s", out)
	}

	menuOut := waitForMenuText(t, env, "2) Stop proxy")
	if !strings.Contains(menuOut, "proxy: running") {
		t.Fatalf("expected menu to show running proxy after background start, got:\n%s", menuOut)
	}

	stopOut, err := runManager(t, env, "stop")
	if err != nil {
		t.Fatalf("stop after background start failed: %v\n%s", err, stopOut)
	}

	menuOut = waitForMenuText(t, env, "2) Start proxy")
	if !strings.Contains(menuOut, "proxy: stopped") {
		t.Fatalf("expected menu to show stopped proxy after background stop, got:\n%s", menuOut)
	}
}

func TestManagerStartBackgroundWithRuntimeOverrideShowsRunningAndStops(t *testing.T) {
	env := managerEnv(t)
	overrideBin := filepath.Join(t.TempDir(), "override", "tg-ws-proxy")

	buildFakeProxyBinary(t, overrideBin)
	env = append(env, "RUNTIME_BIN_OVERRIDE="+overrideBin)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background with runtime override failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Background process pid:") {
		t.Fatalf("expected background pid output, got:\n%s", out)
	}

	menuOut := waitForMenuText(t, env, "2) Stop proxy")
	if !strings.Contains(menuOut, "proxy: running") {
		t.Fatalf("expected menu to show running proxy for runtime override, got:\n%s", menuOut)
	}

	statusOut, statusErr := runManager(t, env, "status")
	if statusErr != nil {
		t.Fatalf("status failed for runtime override: %v\n%s", statusErr, statusOut)
	}
	if !strings.Contains(statusOut, "process   : running") {
		t.Fatalf("expected status to report running proxy for runtime override, got:\n%s", statusOut)
	}

	stopOut, stopErr := runManager(t, env, "stop")
	if stopErr != nil {
		t.Fatalf("stop failed for runtime override: %v\n%s", stopErr, stopOut)
	}
	if !strings.Contains(stopOut, "Proxy stopped") {
		t.Fatalf("expected stop confirmation for runtime override, got:\n%s", stopOut)
	}

	menuOut = waitForMenuText(t, env, "2) Start proxy")
	if !strings.Contains(menuOut, "proxy: stopped") {
		t.Fatalf("expected menu to show stopped proxy after runtime override stop, got:\n%s", menuOut)
	}
}

func TestManagerMenuBackgroundStartThenStopProxySameSession(t *testing.T) {
	env := managerEnv(t)
	binPath := envValue(env, "BIN_PATH")
	if binPath == "" {
		t.Fatal("BIN_PATH not found in env")
	}

	buildFakeProxyBinary(t, binPath)

	out, err := runManagerMenu(t, env, "2\nb\n\n2\n\n\n")
	if err != nil {
		t.Fatalf("same-session background start then stop failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Background process pid:") {
		t.Fatalf("expected background start output, got:\n%s", out)
	}
	if !strings.Contains(out, "Proxy stopped") {
		t.Fatalf("expected stop confirmation in same menu session, got:\n%s", out)
	}
	if !strings.Contains(out, "2) Start proxy") {
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
	if !strings.Contains(out, "proxy: running") {
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

	if !strings.Contains(menuOut, "proxy: running") || !strings.Contains(statusOut, "process   : running") {
		t.Fatalf("menu/status disagree on running state\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
	if !strings.Contains(menuOut, "autostart: enabled") || !strings.Contains(statusOut, "autostart : enabled") {
		t.Fatalf("menu/status disagree on autostart state\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
	if !strings.Contains(menuOut, "verbose: on") || !strings.Contains(statusOut, "verbose   : on") {
		t.Fatalf("menu/status disagree on verbose state\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
	if !strings.Contains(menuOut, "track: release/latest") || !strings.Contains(statusOut, "src mode  : release") || !strings.Contains(statusOut, "ref       : latest") {
		t.Fatalf("menu/status disagree on update track\nmenu:\n%s\nstatus:\n%s", menuOut, statusOut)
	}
}
