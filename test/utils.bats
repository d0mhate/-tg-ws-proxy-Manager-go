

#!/usr/bin/env bats

setup() {
    TEST_DIR="$BATS_TEST_TMPDIR"

    export COMMAND_MODE="1"
    export SOCKS_PASSWORD=""

    source "$BATS_TEST_DIRNAME/../lib/utils.sh"
}

# --------------------------
# tmp_available_kb
# --------------------------

@test "tmp_available_kb returns number" {
    run tmp_available_kb

    [ "$status" -eq 0 ]
    [[ "$output" =~ ^[0-9]+$ ]]
}

# --------------------------
# closest_existing_path
# --------------------------

@test "closest_existing_path returns same path if exists" {
    file="$TEST_DIR/file"
    touch "$file"

    run closest_existing_path "$file"

    [ "$status" -eq 0 ]
    [ "$output" = "$file" ]
}

@test "closest_existing_path climbs up to existing dir" {
    dir="$TEST_DIR/dir"
    mkdir -p "$dir"

    run closest_existing_path "$dir/not/exist/file"

    [ "$status" -eq 0 ]
    [ "$output" = "$dir" ]
}

@test "closest_existing_path falls back to root" {
    run closest_existing_path ""

    [ "$status" -eq 0 ]
    [ "$output" = "/" ]
}

# --------------------------
# path_available_kb
# --------------------------

@test "path_available_kb works on existing path" {
    run path_available_kb "$TEST_DIR"

    [ "$status" -eq 0 ]
    [[ "$output" =~ ^[0-9]+$ ]]
}

@test "path_available_kb works on non-existing path" {
    run path_available_kb "$TEST_DIR/not/exist/file"

    [ "$status" -eq 0 ]
    [[ "$output" =~ ^[0-9]+$ ]]
}

# --------------------------
# read_first_line
# --------------------------

@test "read_first_line reads first line" {
    file="$TEST_DIR/file"
    printf "hello\nworld" > "$file"

    run read_first_line "$file"

    [ "$status" -eq 0 ]
    [ "$output" = "hello" ]
}

@test "read_first_line fails on missing file" {
    run read_first_line "$TEST_DIR/missing"

    [ "$status" -ne 0 ]
}

@test "read_first_line fails on empty file" {
    file="$TEST_DIR/empty"
    : > "$file"

    run read_first_line "$file"

    [ "$status" -ne 0 ]
}

# --------------------------
# pause
# --------------------------

@test "pause does nothing in command mode" {
    COMMAND_MODE="1"

    run pause

    [ "$status" -eq 0 ]
    [ "$output" = "" ]
}

@test "pause prints message when not command mode" {
    COMMAND_MODE="0"

    run pause <<<'\n'

    [ "$status" -eq 0 ]
    [[ "$output" == *"Press Enter to continue"* ]]
}

# --------------------------
# canonical_path
# --------------------------

@test "canonical_path resolves path or returns same" {
    file="$TEST_DIR/file"
    touch "$file"

    run canonical_path "$file"

    [ "$status" -eq 0 ]
    [[ "$output" == *"file"* ]]
}

# --------------------------
# current_script_path
# --------------------------

@test "current_script_path returns something" {
    run current_script_path

    [ "$status" -eq 0 ]
    [[ -n "$output" ]]
}

# --------------------------
# password_display
# --------------------------

@test "password_display empty" {
    SOCKS_PASSWORD=""

    run password_display

    [ "$status" -eq 0 ]
    [ "$output" = "<empty>" ]
}

@test "password_display set" {
    SOCKS_PASSWORD="secret"

    run password_display

    [ "$status" -eq 0 ]
    [ "$output" = "<set>" ]
}