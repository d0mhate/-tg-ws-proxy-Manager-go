

#!/usr/bin/env bats

setup() {
    TEST_DIR="$BATS_TEST_TMPDIR"

    export PERSISTENT_DIR_CANDIDATES="$TEST_DIR/a $TEST_DIR/b"
    export LAUNCHER_PATH="$TEST_DIR/launcher"
    export LAUNCHER_NAME="tgm"
    export INSTALL_DIR="$TEST_DIR/install"
    export BIN_PATH="$TEST_DIR/bin"
    export SOURCE_BIN="$TEST_DIR/src_bin"
    export SOURCE_MANAGER_SCRIPT="$TEST_DIR/src_script.sh"
    export VERSION_FILE="$TEST_DIR/version"
    export PERSIST_MANAGER_NAME="mgr.sh"
    export COMMAND_MODE="1"

    mkdir -p "$TEST_DIR/a" "$TEST_DIR/b"

    source "$BATS_TEST_DIRNAME/../lib/install.sh"
}

# ---- stubs ----

path_available_kb() {
    case "$1" in
        *a) echo 100 ;;
        *b) echo 1000 ;;
    esac
}

current_script_path() { echo "$TEST_DIR/current.sh"; }

cached_source_version() { echo "1.0.0"; }

has_persistent_install() { return 1; }

# ---- select_persistent_dir ----

@test "select_persistent_dir picks first suitable" {
    run select_persistent_dir 50

    [ "$status" -eq 0 ]
    [[ "$output" == *"a" ]]
}

@test "select_persistent_dir skips insufficient" {
    run select_persistent_dir 500

    [ "$status" -eq 0 ]
    [[ "$output" == *"b" ]]
}

@test "select_persistent_dir fails if none" {
    run select_persistent_dir 5000

    [ "$status" -ne 0 ]
}

# ---- current_launcher_path ----

@test "current_launcher_path returns main path" {
    touch "$LAUNCHER_PATH"

    run current_launcher_path

    [ "$output" = "$LAUNCHER_PATH" ]
}

@test "current_launcher_path falls back to tmp" {
    touch "/tmp/$LAUNCHER_NAME"

    run current_launcher_path

    [[ "$output" == *"/tmp"* ]]
}

# ---- install_launcher ----

@test "install_launcher creates launcher" {
    run install_launcher "$TEST_DIR/script.sh"

    [ "$status" -eq 0 ]
    [ -f "$output" ]
}

@test "install_launcher is executable" {
    path="$(install_launcher "$TEST_DIR/script.sh")"

    [ -x "$path" ]
}

# ---- copy_manager_bundle ----

@test "copy_manager_bundle copies script" {
    src="$TEST_DIR/src.sh"
    dest="$TEST_DIR/dest.sh"

    echo test > "$src"

    run copy_manager_bundle "$src" "$dest"

    [ -f "$dest" ]
}

@test "copy_manager_bundle copies lib dir" {
    mkdir -p "$TEST_DIR/lib"
    echo a > "$TEST_DIR/lib/x"

    src="$TEST_DIR/script.sh"
    dest="$TEST_DIR/out/script.sh"
    echo ok > "$src"

    run copy_manager_bundle "$src" "$dest"

    [ -d "$TEST_DIR/out/lib" ]
}

# ---- install_from_source ----

@test "install_from_source copies binary" {
    echo bin > "$SOURCE_BIN"
    echo script > "$SOURCE_MANAGER_SCRIPT"

    run install_from_source

    [ "$status" -eq 0 ]
    [ -f "$BIN_PATH" ]
}

@test "install_from_source writes version" {
    echo bin > "$SOURCE_BIN"
    echo script > "$SOURCE_MANAGER_SCRIPT"

    install_from_source

    [ -f "$VERSION_FILE" ]
}

@test "ensure_source_binary_current downloads when missing binary" {
    SOURCE_BIN="$TEST_DIR/missing"
    resolved_release_ref() { echo "1.0.0"; }
    cached_source_version() { echo ""; }
    release_url_reachable() { return 0; }
    download_binary() { touch "$SOURCE_BIN"; return 0; }
    write_source_version_file() { return 0; }

    run ensure_source_binary_current

    [ "$status" -eq 0 ]
    [ -f "$SOURCE_BIN" ]
}

@test "ensure_source_binary_current uses cached binary if url unreachable" {
    echo bin > "$SOURCE_BIN"

    resolved_release_ref() { echo "1.0.0"; }
    cached_source_version() { echo "1.0.0"; }
    release_url_reachable() { return 1; }

    run ensure_source_binary_current

    [ "$status" -eq 0 ]
}

@test "ensure_source_binary_current fails if no binary and no network" {
    rm -f "$SOURCE_BIN"

    resolved_release_ref() { echo "1.0.0"; }
    cached_source_version() { echo ""; }
    release_url_reachable() { return 1; }

    run ensure_source_binary_current

    [ "$status" -ne 0 ]
}