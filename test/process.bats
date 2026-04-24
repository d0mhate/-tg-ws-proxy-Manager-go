#!/usr/bin/env bats

setup() {
  tmpdir="$(mktemp -d)"
  export BIN_PATH="$tmpdir/bin/tg-ws-proxy"
  export PID_FILE="$tmpdir/run/proxy.pid"
  export INIT_SCRIPT_PATH="$tmpdir/init.d/tg-ws-proxy-go"
  export PERSIST_STATE_DIR="$tmpdir/state"
  export PERSIST_PATH_FILE="$PERSIST_STATE_DIR/install_dir"
  export PERSIST_MANAGER_NAME="tg-ws-proxy-go.sh"
}

teardown() {
  rm -rf "$tmpdir"
}

@test "runtime_bin_path prefers local bin and falls back to persistent bin" {
  run bash -c '
    BIN_PATH="$1"
    PERSIST_PATH_FILE="$2"
    PERSIST_MANAGER_NAME="$3"

    read_first_line() {
      [ -f "$1" ] || return 1
      sed -n '\''1p'\'' "$1"
    }

    source ./lib/config.sh
    source ./lib/process.sh

    mkdir -p "$(dirname "$BIN_PATH")"
    printf "#!/bin/sh\nexit 0\n" > "$BIN_PATH"
    chmod +x "$BIN_PATH"
    [ "$(runtime_bin_path)" = "$BIN_PATH" ] || exit 1

    rm -f "$BIN_PATH"
    install_dir="$(dirname "$BIN_PATH")/persistent"
    mkdir -p "$install_dir"
    mkdir -p "$(dirname "$PERSIST_PATH_FILE")"
    printf "%s\n" "$install_dir" > "$PERSIST_PATH_FILE"
    printf "#!/bin/sh\nexit 0\n" > "$install_dir/tg-ws-proxy"
    chmod +x "$install_dir/tg-ws-proxy"
    [ "$(runtime_bin_path)" = "$install_dir/tg-ws-proxy" ] || exit 1
  ' _ "$BIN_PATH" "$PERSIST_PATH_FILE" "$PERSIST_MANAGER_NAME"

  [ "$status" -eq 0 ]
}

@test "run_binary builds socks5 command with auth dc and cloudflare args" {
  run bash -c '
    BIN_PATH="$1"
    capture_file="$2"
    LISTEN_HOST="127.0.0.1"
    LISTEN_PORT="1081"
    PROXY_MODE="socks5"
    SOCKS_USERNAME="user"
    SOCKS_PASSWORD="pass"
    VERBOSE="1"
    DC_IPS="1:1.1.1.1, 2:2.2.2.2"
    CF_PROXY="1"
    CF_PROXY_FIRST="1"
    CF_DOMAIN="cf.example"
    MT_LINK_IP=""
    MT_SECRET=""
    MT_UPSTREAM_PROXIES=""

    read_first_line() {
      [ -f "$1" ] || return 1
      sed -n '\''1p'\'' "$1"
    }

    source ./lib/config.sh
    source ./lib/process.sh

    mkdir -p "$(dirname "$BIN_PATH")"
    cat > "$BIN_PATH" <<'\''EOF'\''
#!/bin/sh
for arg in "$@"; do
  printf "%s\n" "$arg"
done > "$CAPTURE_FILE"
EOF
    chmod +x "$BIN_PATH"
    CAPTURE_FILE="$capture_file" run_binary || exit 1

    diff -u - "$capture_file" <<'\''EOF'\''
--host
127.0.0.1
--port
1081
--username
user
--password
pass
--verbose
--dc-ip
1:1.1.1.1
--dc-ip
2:2.2.2.2
--cf-proxy
--cf-domain
cf.example
--cf-proxy-first
EOF
  ' _ "$BIN_PATH" "$tmpdir/socks.args"

  [ "$status" -eq 0 ]
}

@test "run_binary builds mtproto command with link ip and upstream proxies" {
  run bash -c '
    BIN_PATH="$1"
    capture_file="$2"
    LISTEN_HOST="0.0.0.0"
    LISTEN_PORT="1443"
    PROXY_MODE="mtproto"
    SOCKS_USERNAME=""
    SOCKS_PASSWORD=""
    VERBOSE="0"
    DC_IPS="1:149.154.175.50"
    CF_PROXY="0"
    CF_PROXY_FIRST="0"
    CF_DOMAIN=""
    MT_SECRET="00112233445566778899aabbccddeeff"
    MT_LINK_IP="9.9.9.9"
    MT_UPSTREAM_PROXIES="one.example:443:00112233445566778899aabbccddeeff, two.example:443:dd00112233445566778899aabbccddeeff"

    read_first_line() {
      [ -f "$1" ] || return 1
      sed -n '\''1p'\'' "$1"
    }

    source ./lib/config.sh
    source ./lib/process.sh

    mkdir -p "$(dirname "$BIN_PATH")"
    cat > "$BIN_PATH" <<'\''EOF'\''
#!/bin/sh
for arg in "$@"; do
  printf "%s\n" "$arg"
done > "$CAPTURE_FILE"
EOF
    chmod +x "$BIN_PATH"
    CAPTURE_FILE="$capture_file" run_binary || exit 1

    diff -u - "$capture_file" <<'\''EOF'\''
--host
0.0.0.0
--port
1443
--mode
mtproto
--secret
00112233445566778899aabbccddeeff
--link-ip
9.9.9.9
--dc-ip
1:149.154.175.50
--mtproto-proxy
one.example:443:00112233445566778899aabbccddeeff
--mtproto-proxy
two.example:443:dd00112233445566778899aabbccddeeff
EOF
  ' _ "$BIN_PATH" "$tmpdir/mtproto.args"

  [ "$status" -eq 0 ]
}

@test "restart_running_proxy_for_updated_settings uses init script when autostart is enabled" {
  run bash -c '
    INIT_SCRIPT_PATH="$1"
    PID_FILE="$2"

    autostart_enabled() { return 0; }
    run_binary_background() { echo "should-not-run"; return 1; }

    source ./lib/process.sh

    mkdir -p "$(dirname "$INIT_SCRIPT_PATH")"
    cat > "$INIT_SCRIPT_PATH" <<'\''EOF'\''
#!/bin/sh
if [ "$1" = "restart" ]; then
  exit 0
fi
exit 1
EOF
    chmod +x "$INIT_SCRIPT_PATH"

    restart_running_proxy_for_updated_settings
  ' _ "$INIT_SCRIPT_PATH" "$PID_FILE"

  [ "$status" -eq 0 ]
  [ ! -f "$PID_FILE" ]
}

@test "restart_running_proxy_for_updated_settings restarts background process and writes pid file" {
  run bash -c '
    PID_FILE="$1"

    autostart_enabled() { return 1; }
    stop_running() { return 0; }

    source ./lib/process.sh
    kill() {
      if [ "$1" = "-0" ] && [ "$2" = "4242" ]; then
        return 0
      fi
      command kill "$@"
    }
    run_binary_background() {
      printf "4242"
    }

    restart_running_proxy_for_updated_settings || exit 1
    pid="$(sed -n '\''1p'\'' "$PID_FILE")"
    [ -n "$pid" ] || exit 1
    [ "$pid" = "4242" ] || exit 1
  ' _ "$PID_FILE"

  [ "$status" -eq 0 ]
}
