package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// mtproto flag passing

func TestManagerStartBackgroundPassesMTProtoFlags(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=mtproto",
		"MT_SECRET=aabbccddeeff00112233445566778899",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if !strings.Contains(args, "--mode") || !strings.Contains(args, "mtproto") {
		t.Errorf("expected --mode mtproto, got:\n%s", args)
	}
	if !strings.Contains(args, "--secret") || !strings.Contains(args, "aabbccddeeff00112233445566778899") {
		t.Errorf("expected --secret aabbccddeeff..., got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundPassesMTProtoDDSecret(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=mtproto",
		"MT_SECRET=ddaabbccddeeff00112233445566778899",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background with dd-secret failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if !strings.Contains(args, "--secret") || !strings.Contains(args, "ddaabbccddeeff00112233445566778899") {
		t.Errorf("expected dd-prefix secret in args, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundPassesMTProtoLinkIP(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=mtproto",
		"MT_SECRET=aabbccddeeff00112233445566778899",
		"MT_LINK_IP=1.2.3.4",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if !strings.Contains(args, "--link-ip") || !strings.Contains(args, "1.2.3.4") {
		t.Errorf("expected --link-ip 1.2.3.4 in args, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundOmitsLinkIPWhenUnset(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=mtproto",
		"MT_SECRET=aabbccddeeff00112233445566778899",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if strings.Contains(args, "--link-ip") {
		t.Errorf("expected --link-ip to be absent when MT_LINK_IP unset, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundMTProtoOmitsSOCKS5Auth(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=mtproto",
		"MT_SECRET=aabbccddeeff00112233445566778899",
		"SOCKS_USERNAME=alice",
		"SOCKS_PASSWORD=secret",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if strings.Contains(args, "--username") || strings.Contains(args, "--password") {
		t.Errorf("expected socks5 auth flags absent in mtproto mode, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundPassesMTProtoUpstreamProxy(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=mtproto",
		"MT_SECRET=aabbccddeeff00112233445566778899",
		"MT_UPSTREAM_PROXIES=proxy.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if !strings.Contains(args, "--mtproto-proxy") {
		t.Errorf("expected --mtproto-proxy flag, got:\n%s", args)
	}
	if !strings.Contains(args, "proxy.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f") {
		t.Errorf("expected upstream proxy entry in args, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundPassesMultipleMTProtoUpstreamProxies(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=mtproto",
		"MT_SECRET=aabbccddeeff00112233445566778899",
		"MT_UPSTREAM_PROXIES=proxy1.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f,proxy2.example.com:8443:dda0b1c2d3e4f5061728394a5b6c7d8e9f",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	count := strings.Count(args, "--mtproto-proxy")
	if count != 2 {
		t.Errorf("expected 2 --mtproto-proxy flags, got %d:\n%s", count, args)
	}
	if !strings.Contains(args, "proxy1.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f") {
		t.Errorf("expected proxy1 entry in args, got:\n%s", args)
	}
	if !strings.Contains(args, "proxy2.example.com:8443:dda0b1c2d3e4f5061728394a5b6c7d8e9f") {
		t.Errorf("expected proxy2 entry in args, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

func TestManagerStartBackgroundOmitsMTProtoUpstreamWhenSocks5Mode(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=socks5",
		"MT_UPSTREAM_PROXIES=proxy.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if strings.Contains(args, "--mtproto-proxy") {
		t.Errorf("expected --mtproto-proxy absent in socks5 mode, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}

// ---------------------------------------------------------------------------
// socks5 flag passing
// ---------------------------------------------------------------------------

func TestManagerStartBackgroundSocks5PassesAuthAndOmitsMTProto(t *testing.T) {
	env := append(managerEnv(t),
		"PROXY_MODE=socks5",
		"SOCKS_USERNAME=bob",
		"SOCKS_PASSWORD=hunter2",
	)
	binPath := envValue(env, "BIN_PATH")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeCapturingProxyScript(t, binPath)
	env = append(env, "ARGS_FILE="+argsFile)

	out, err := runManager(t, env, "start-background")
	if err != nil {
		t.Fatalf("start-background failed: %v\n%s", err, out)
	}

	waitForFile(t, argsFile)
	args := readTrimmed(t, argsFile)

	if !strings.Contains(args, "--username") || !strings.Contains(args, "bob") {
		t.Errorf("expected --username bob in socks5 mode, got:\n%s", args)
	}
	if !strings.Contains(args, "--password") || !strings.Contains(args, "hunter2") {
		t.Errorf("expected --password hunter2 in socks5 mode, got:\n%s", args)
	}
	if strings.Contains(args, "--mode") || strings.Contains(args, "--secret") {
		t.Errorf("expected --mode/--secret absent in socks5 mode, got:\n%s", args)
	}

	runManager(t, env, "stop") //nolint
}
