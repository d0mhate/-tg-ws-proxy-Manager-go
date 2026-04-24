

#!/usr/bin/env bats

setup() {
    TEST_DIR="$BATS_TEST_TMPDIR"

    export SOURCE_BIN="$TEST_DIR/bin"
    export SOURCE_VERSION_FILE="$TEST_DIR/version"
    export VERSION_FILE="$TEST_DIR/installed"
    export PERSIST_VERSION_FILE="$TEST_DIR/persist_version"
    export PERSIST_RELEASE_TAG_FILE="$TEST_DIR/persist_tag"
    export LATEST_VERSION_CACHE_FILE="$TEST_DIR/cache"

    export RELEASE_DOWNLOAD_BASE_URL="https://example.com/bin"
    export SCRIPT_RELEASE_BASE_URL="https://example.com/script"
    export PREVIEW_BASE_URL="https://preview.example.com"

    export REQUIRED_TMP_KB=100
    export PERSISTENT_SPACE_HEADROOM_KB=50
    export MIN_PINNED_RELEASE_TAG="v1.0.0"

    source "$BATS_TEST_DIRNAME/../lib/release.sh"

    # stub read_first_line (from utils.sh)
    read_first_line() {
        file="$1"
        [ -f "$file" ] || return 1
        IFS= read -r line < "$file" || return 1
        [ -n "$line" ] || return 1
        printf "%s" "$line"
    }
}

# ---- resolved_binary_name ----

@test "resolved_binary_name uses explicit name" {
    BINARY_NAME="custom-bin"
    run resolved_binary_name

    [ "$output" = "custom-bin" ]
}

@test "resolved_binary_name falls back to generic" {
    unset BINARY_NAME
    is_openwrt() { return 1; }
    generic_binary_name() { echo "generic-bin"; }

    run resolved_binary_name

    [ "$output" = "generic-bin" ]
}

# ---- resolved_release_url ----

@test "resolved_release_url uses explicit URL" {
    RELEASE_URL="https://direct"
    run resolved_release_url

    [ "$output" = "https://direct" ]
}

@test "resolved_release_url uses preview branch" {
    selected_preview_branch() { echo "dev"; }
    resolved_binary_name() { echo "bin"; }

    run resolved_release_url

    [[ "$output" == *"dev/bin" ]]
}

@test "resolved_release_url uses tag" {
    selected_preview_branch() { return 1; }
    selected_release_tag() { echo "v1.2.3"; }
    resolved_binary_name() { echo "bin"; }

    run resolved_release_url

    [[ "$output" == *"v1.2.3/bin" ]]
}

# ---- version_ge ----

@test "version_ge true when greater" {
    run version_ge v2.0.0 v1.0.0
    [ "$status" -eq 0 ]
}

@test "version_ge false when lower" {
    run version_ge v1.0.0 v2.0.0
    [ "$status" -ne 0 ]
}

# ---- normalize_version ----

@test "normalize_version keeps valid" {
    run normalize_version v1.2.3
    [ "$output" = "v1.2.3" ]
}

@test "normalize_version rejects invalid" {
    run normalize_version 1.2.3
    [ "$status" -ne 0 ]
}

# ---- source_binary_size_kb ----

@test "source_binary_size_kb calculates size" {
    printf "123456" > "$SOURCE_BIN"

    run source_binary_size_kb

    [ "$status" -eq 0 ]
}

# ---- required_persistent_kb ----

@test "required_persistent_kb uses fallback when unknown" {
    run required_persistent_kb

    [ "$output" = "100" ]
}

# ---- installed_version ----

@test "installed_version reads file" {
    echo "v1.0.0" > "$VERSION_FILE"

    run installed_version

    [ "$output" = "v1.0.0" ]
}

# ---- cached_source_version ----

@test "cached_source_version reads file" {
    echo "v2.0.0" > "$SOURCE_VERSION_FILE"

    run cached_source_version

    [ "$output" = "v2.0.0" ]
}

# ---- write_release_tag_state ----

@test "write_release_tag_state writes file" {
    run write_release_tag_state v1.1.0

    [ -f "$PERSIST_RELEASE_TAG_FILE" ]
}

# ---- latest_version_cache ----

@test "latest_version_cache_is_fresh works" {
    echo "v1.0.0" > "$LATEST_VERSION_CACHE_FILE"
    date +%s >> "$LATEST_VERSION_CACHE_FILE"

    run latest_version_cache_is_fresh

    [ "$status" -eq 0 ]
}