#!/usr/bin/env bats

setup() {
    TEST_DIR="$BATS_TEST_TMPDIR"

    export INIT_SCRIPT_PATH="$TEST_DIR/init.sh"
    export RC_D_DIR="$TEST_DIR/rc.d"
    export PERSIST_CONFIG_FILE="$TEST_DIR/config"
    export PERSIST_STATE_DIR="$TEST_DIR/state"
    export BIN_PATH="$TEST_DIR/bin"
    export LAUNCHER_PATH="$TEST_DIR/launcher"
    export LAUNCHER_NAME="tgm"
    export COMMAND_MODE="1"

    mkdir -p "$RC_D_DIR"

    source "$BATS_TEST_DIRNAME/../lib/autostart.sh"
}

# --------------------------
# Stubs
# --------------------------

persistent_bin_path() {
    printf "%s" "$TEST_PERSIST_BIN"
}

is_running() {
    [ "${TEST_IS_RUNNING:-0}" = "1" ]
}

is_openwrt() {
    [ "${TEST_IS_OPENWRT:-1}" = "1" ]
}

auth_settings_valid() {
    [ "${TEST_AUTH_VALID:-1}" = "1" ]
}

check_tmp_space() {
    return 0
}

ensure_source_binary_current() {
    return 0
}

install_persistent_binary() {
    printf "%s" "$TEST_DIR/launcher"
}

write_autostart_config() {
    return 0
}

selected_update_channel() {
    printf "release"
}

selected_update_ref() {
    printf "latest"
}

persistent_install_dir() {
    printf "%s" "$TEST_DIR/persist"
}

write_update_source_state() {
    return 0
}

install_launcher() {
    return 0
}

pause() {
    return 0
}

show_header() { :; }
show_invalid_auth_settings() { :; }
show_persistent_install_failure() { :; }

# --------------------------
# autostart_enabled
# --------------------------

@test "autostart_enabled false when no init script" {
    run autostart_enabled

    [ "$status" -ne 0 ]
}

@test "autostart_enabled false when no rc.d link" {
    touch "$INIT_SCRIPT_PATH"

    run autostart_enabled

    [ "$status" -ne 0 ]
}

@test "autostart_enabled false when binary missing" {
    touch "$INIT_SCRIPT_PATH"
    ln -s "$INIT_SCRIPT_PATH" "$RC_D_DIR/S95$(basename "$INIT_SCRIPT_PATH")"
    TEST_PERSIST_BIN=""

    run autostart_enabled

    [ "$status" -ne 0 ]
}

@test "autostart_enabled false when binary not executable" {
    touch "$INIT_SCRIPT_PATH"
    ln -s "$INIT_SCRIPT_PATH" "$RC_D_DIR/S95$(basename "$INIT_SCRIPT_PATH")"
    TEST_PERSIST_BIN="$TEST_DIR/bin"
    touch "$TEST_DIR/bin"

    run autostart_enabled

    [ "$status" -ne 0 ]
}

@test "autostart_enabled true when everything valid" {
    touch "$INIT_SCRIPT_PATH"
    ln -s "$INIT_SCRIPT_PATH" "$RC_D_DIR/S95$(basename "$INIT_SCRIPT_PATH")"
    TEST_PERSIST_BIN="$TEST_DIR/bin"
    touch "$TEST_DIR/bin"
    chmod +x "$TEST_DIR/bin"
    touch "$PERSIST_CONFIG_FILE"

    run autostart_enabled

    [ "$status" -eq 0 ]
}

# --------------------------
# write_init_script
# --------------------------

@test "write_init_script creates file" {
    run write_init_script

    [ "$status" -eq 0 ]
    [ -f "$INIT_SCRIPT_PATH" ]
}

@test "write_init_script is executable" {
    write_init_script

    [ -x "$INIT_SCRIPT_PATH" ]
}

@test "write_init_script contains mtproto validation" {
    write_init_script

    run grep -q 'MT_SECRET' "$INIT_SCRIPT_PATH"
    [ "$status" -eq 0 ]
}

@test "write_init_script contains socks auth validation" {
    write_init_script

    run grep -q 'USERNAME' "$INIT_SCRIPT_PATH"
    [ "$status" -eq 0 ]
}

@test "write_init_script contains dc handling" {
    write_init_script

    run grep -q -- '--dc-ip' "$INIT_SCRIPT_PATH"
    [ "$status" -eq 0 ]
}

@test "write_init_script contains cf proxy logic" {
    write_init_script

    run grep -q -- '--cf-proxy' "$INIT_SCRIPT_PATH"
    [ "$status" -eq 0 ]
}

# --------------------------
# enable_autostart (base flow)
# --------------------------

@test "enable_autostart fails when auth invalid" {
    TEST_AUTH_VALID="0"

    run enable_autostart

    [ "$status" -ne 0 ]
}

@test "enable_autostart fails when not openwrt" {
    TEST_AUTH_VALID="1"
    TEST_IS_OPENWRT="0"

    run enable_autostart

    [ "$status" -ne 0 ]
}

# --------------------------
# disable_autostart
# --------------------------

@test "disable_autostart handles no config" {
    run disable_autostart

    [ "$status" -eq 0 ]
}

@test "disable_autostart removes init script" {
    touch "$INIT_SCRIPT_PATH"

    run disable_autostart

    [ "$status" -eq 0 ]
    [ ! -f "$INIT_SCRIPT_PATH" ]
}

@test "disable_autostart removes persist dir" {
    mkdir -p "$TEST_DIR/persist"

    run disable_autostart

    [ "$status" -eq 0 ]
    [ ! -d "$TEST_DIR/persist" ]
}