#!/usr/bin/env bats

setup() {
  tmpdir="$(mktemp -d)"
  export SOURCE_BIN="$tmpdir/source/tg-ws-proxy"
  export SOURCE_VERSION_FILE="$tmpdir/source/tg-ws-proxy.version"
  export SOURCE_MANAGER_SCRIPT="$tmpdir/source/tg-ws-proxy-go.sh"
  export INSTALL_DIR="$tmpdir/install"
  export BIN_PATH="$INSTALL_DIR/tg-ws-proxy"
  export VERSION_FILE="$INSTALL_DIR/version"
  export PERSIST_STATE_DIR="$tmpdir/state"
  export PERSIST_PATH_FILE="$PERSIST_STATE_DIR/install_dir"
  export PERSIST_VERSION_FILE="$PERSIST_STATE_DIR/version"
  export PERSIST_RELEASE_TAG_FILE="$PERSIST_STATE_DIR/release_tag"
  export PERSIST_UPDATE_CHANNEL_FILE="$PERSIST_STATE_DIR/update_channel"
  export PERSIST_PREVIEW_BRANCH_FILE="$PERSIST_STATE_DIR/preview_branch"
  export PERSIST_MANAGER_NAME="tg-ws-proxy-go.sh"
  export LAUNCHER_NAME="tgm-test"
  export LAUNCHER_PATH="$tmpdir/bin/tgm-test"
  export RELEASE_DOWNLOAD_BASE_URL="https://example.com/latest/download"
  export SCRIPT_RELEASE_BASE_URL="https://example.com/releases"
  export PREVIEW_BASE_URL="https://example.com/preview"
  export DEFAULT_BINARY_NAME="tg-ws-proxy-openwrt"
  export MIN_PINNED_RELEASE_TAG="v1.1.29"
}

teardown() {
  rm -rf "$tmpdir"
}

@test "resolved_release_url prefers explicit release url preview and pinned release" {
  run bash -c '
    RELEASE_URL="https://manual.example/custom"
    PREVIEW_BASE_URL="https://example.com/preview"
    SCRIPT_RELEASE_BASE_URL="https://example.com/releases"
    RELEASE_DOWNLOAD_BASE_URL="https://example.com/latest/download"
    DEFAULT_BINARY_NAME="tg-ws-proxy-openwrt"
    BINARY_NAME=""

    source ./lib/platform.sh
    source ./lib/platform.sh
    source ./lib/platform.sh
    source ./lib/release.sh

    is_openwrt() { return 1; }
    generic_binary_name() { printf "tg-ws-proxy-linux-amd64"; }
    selected_preview_branch() { printf "preview-branch"; }
    selected_release_tag() { printf "v1.2.3"; }

    [ "$(resolved_release_url)" = "https://manual.example/custom" ] || exit 1

    RELEASE_URL=""
    [ "$(resolved_release_url)" = "https://example.com/preview/preview-branch/tg-ws-proxy-linux-amd64" ] || exit 1

    selected_preview_branch() { return 1; }
    selected_release_tag() { printf "v1.2.3"; }
    [ "$(resolved_release_url)" = "https://example.com/releases/v1.2.3/tg-ws-proxy-linux-amd64" ] || exit 1

    selected_release_tag() { return 1; }
    [ "$(resolved_release_url)" = "https://example.com/latest/download/tg-ws-proxy-linux-amd64" ] || exit 1
  '

  [ "$status" -eq 0 ]
}

@test "latest_release_tag and recent_release_tags read file fixtures and filter tags" {
  run bash -c '
    RELEASE_API_JSON="$1"
    RELEASES_API_JSON="$2"
    RELEASE_API_URL="file://$RELEASE_API_JSON"
    RELEASES_API_URL="file://$RELEASES_API_JSON"
    MIN_PINNED_RELEASE_TAG="v1.1.29"

    read_first_line() {
      [ -f "$1" ] || return 1
      sed -n '\''1p'\'' "$1"
    }

    cat > "$RELEASE_API_JSON" <<'\''EOF'\''
{"tag_name":"v1.2.3"}
EOF

    cat > "$RELEASES_API_JSON" <<'\''EOF'\''
[
  {"tag_name":"v1.2.3"},
  {"tag_name":"v1.2.3"},
  {"tag_name":"v1.2.0"},
  {"tag_name":"v1.1.28"},
  {"tag_name":"garbage"}
]
EOF

    source ./lib/release.sh

    [ "$(latest_release_tag)" = "v1.2.3" ] || exit 1

    cat > "$RELEASES_API_JSON.expected" <<'\''EOF'\''
v1.2.3
v1.2.0
EOF
    recent_release_tags 5 > "$RELEASES_API_JSON.actual" || exit 1
    diff -u "$RELEASES_API_JSON.expected" "$RELEASES_API_JSON.actual"
  ' _ "$tmpdir/release_api.json" "$tmpdir/releases_api.json"

  [ "$status" -eq 0 ]
}

@test "ensure_source_binary_current downloads missing source and writes cached version" {
  run bash -c '
    SOURCE_BIN="$1"
    SOURCE_VERSION_FILE="$2"

    source ./lib/release.sh
    source ./lib/install.sh
    resolved_release_ref() { printf "v1.2.3"; }
    selected_preview_branch() { return 1; }
    cached_source_version() { return 1; }
    resolved_release_url() { printf "https://example.com/releases/v1.2.3/tg-ws-proxy"; }
    release_url_reachable() { return 0; }
    download_binary() {
      mkdir -p "$(dirname "$SOURCE_BIN")"
      printf "#!/bin/sh\nexit 0\n" > "$SOURCE_BIN"
      chmod +x "$SOURCE_BIN"
    }

    ensure_source_binary_current || exit 1
    [ -x "$SOURCE_BIN" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$SOURCE_VERSION_FILE")" = "v1.2.3" ] || exit 1
  ' _ "$SOURCE_BIN" "$SOURCE_VERSION_FILE"

  [ "$status" -eq 0 ]
}

@test "ensure_source_binary_current keeps cached binary when release url is unreachable" {
  run bash -c '
    SOURCE_BIN="$1"
    SOURCE_VERSION_FILE="$2"

    mkdir -p "$(dirname "$SOURCE_BIN")"
    printf "#!/bin/sh\nexit 0\n" > "$SOURCE_BIN"
    chmod +x "$SOURCE_BIN"
    printf "v1.0.0\n" > "$SOURCE_VERSION_FILE"

    source ./lib/install.sh
    resolved_release_ref() { printf "v1.2.3"; }
    selected_preview_branch() { return 1; }
    cached_source_version() { printf "v1.0.0"; }
    resolved_release_url() { printf "https://example.com/releases/v1.2.3/tg-ws-proxy"; }
    release_url_reachable() { return 1; }
    download_binary() { exit 1; }

    ensure_source_binary_current || exit 1
    [ -x "$SOURCE_BIN" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$SOURCE_VERSION_FILE")" = "v1.0.0" ] || exit 1
  ' _ "$SOURCE_BIN" "$SOURCE_VERSION_FILE"

  [ "$status" -eq 0 ]
}

@test "verify_source_binary checks sha256 without executing downloaded binary" {
  run bash -c '
    SOURCE_BIN="$1"
    RELEASE_API_URL="file://$2"
    OPENWRT_RELEASE_FILE="$3"
    DEFAULT_BINARY_NAME="tg-ws-proxy-openwrt"
    MARKER_FILE="$4"

    mkdir -p "$(dirname "$SOURCE_BIN")"
    cat > "$SOURCE_BIN" <<'\''EOF'\''
#!/bin/sh
echo executed > "$MARKER_FILE"
exit 0
EOF
    chmod +x "$SOURCE_BIN"

    cat > "$OPENWRT_RELEASE_FILE" <<'\''EOF'\''
DISTRIB_ID='\''OpenWrt'\''
DISTRIB_ARCH='\''mipsel_24kc'\''
EOF

    digest="$(sha256sum "$SOURCE_BIN" | awk '\''{print $1}'\'')"
    cat > "$2" <<EOF
{"tag_name":"v1.2.3","assets":[{"name":"tg-ws-proxy-openwrt-mipsel_24kc","digest":"sha256:$digest"}]}
EOF

    source ./lib/platform.sh
    source ./lib/release.sh

    verify_source_binary || exit 1
    [ -x "$SOURCE_BIN" ] || exit 1
    [ ! -f "$4" ] || exit 1
  ' _ "$SOURCE_BIN" "$tmpdir/release_api.json" "$tmpdir/openwrt_release" "$tmpdir/executed.marker"

  [ "$status" -eq 0 ]
}

@test "resolved_release_url and release_asset_digest use the same asset name" {
  run bash -c '
    RELEASE_API_URL="file://$1"
    RELEASE_DOWNLOAD_BASE_URL="https://example.com/latest/download"
    BINARY_NAME=""

    cat > "$1" <<'\''EOF'\''
{"tag_name":"v1.2.3","assets":[
  {"name":"tg-ws-proxy-linux-amd64","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"},
  {"name":"tg-ws-proxy-linux-arm64","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"}
]}
EOF

    source ./lib/release.sh

    is_openwrt() { return 1; }
    generic_binary_name() { printf "tg-ws-proxy-linux-amd64"; }
    selected_preview_branch() { return 1; }
    selected_release_tag() { return 1; }

    [ "$(resolved_release_url)" = "https://example.com/latest/download/tg-ws-proxy-linux-amd64" ] || exit 1
    [ "$(release_asset_digest "$(resolved_binary_name)")" = "sha256:1111111111111111111111111111111111111111111111111111111111111111" ] || exit 1
    [ -z "$(release_asset_digest "tg-ws-proxy-linux-armv7" 2>/dev/null || true)" ] || exit 1
  ' _ "$tmpdir/release_api.json"

  [ "$status" -eq 0 ]
}

@test "resolved_release_url and release_asset_digest stay aligned across supported architectures" {
  run bash -c '
    RELEASE_API_URL="file://$1"
    RELEASE_DOWNLOAD_BASE_URL="https://example.com/latest/download"
    PREVIEW_BASE_URL="https://example.com/preview"
    SCRIPT_RELEASE_BASE_URL="https://example.com/releases"
    DEFAULT_BINARY_NAME="tg-ws-proxy-openwrt"
    BINARY_NAME=""

    cat > "$1" <<'\''EOF'\''
{"tag_name":"v1.2.3","assets":[
  {"name":"tg-ws-proxy-openwrt-mipsel_24kc","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"},
  {"name":"tg-ws-proxy-openwrt-mips_24kc","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"},
  {"name":"tg-ws-proxy-openwrt-aarch64","digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333"},
  {"name":"tg-ws-proxy-openwrt-x86_64","digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444"},
  {"name":"tg-ws-proxy-openwrt-armv7","digest":"sha256:5555555555555555555555555555555555555555555555555555555555555555"}
]}
EOF

    source ./lib/platform.sh
    source ./lib/release.sh

    is_openwrt() { return 0; }
    selected_preview_branch() { return 1; }
    selected_release_tag() { return 1; }

    check_case() {
      arch="$1"
      want_name="$2"
      want_digest="$3"
      openwrt_arch() { printf "%s" "$arch"; }

      [ "$(resolved_binary_name)" = "$want_name" ] || return 1
      [ "$(resolved_release_url)" = "https://example.com/latest/download/$want_name" ] || return 1
      [ "$(release_asset_digest "$(resolved_binary_name)")" = "$want_digest" ] || return 1
    }

    check_case "mipsel_24kc" "tg-ws-proxy-openwrt-mipsel_24kc" "sha256:1111111111111111111111111111111111111111111111111111111111111111" || exit 1
    check_case "mips_24kc" "tg-ws-proxy-openwrt-mips_24kc" "sha256:2222222222222222222222222222222222222222222222222222222222222222" || exit 1
    check_case "aarch64" "tg-ws-proxy-openwrt-aarch64" "sha256:3333333333333333333333333333333333333333333333333333333333333333" || exit 1
    check_case "x86_64" "tg-ws-proxy-openwrt-x86_64" "sha256:4444444444444444444444444444444444444444444444444444444444444444" || exit 1
    check_case "arm_cortex-a7_neon-vfpv4" "tg-ws-proxy-openwrt-armv7" "sha256:5555555555555555555555555555555555555555555555555555555555555555" || exit 1
  ' _ "$tmpdir/release_api_matrix.json"

  [ "$status" -eq 0 ]
}

@test "install_from_source installs binary manager bundle version and launcher" {
  run bash -c '
    SOURCE_BIN="$1"
    SOURCE_VERSION_FILE="$2"
    SOURCE_MANAGER_SCRIPT="$3"
    INSTALL_DIR="$4"
    BIN_PATH="$5"
    VERSION_FILE="$6"
    PERSIST_MANAGER_NAME="$7"
    LAUNCHER_PATH="$8"
    LAUNCHER_NAME="$9"

    mkdir -p "$(dirname "$SOURCE_BIN")" "$(dirname "$SOURCE_MANAGER_SCRIPT")" "$(dirname "$LAUNCHER_PATH")"
    printf "#!/bin/sh\nexit 0\n" > "$SOURCE_BIN"
    chmod +x "$SOURCE_BIN"
    printf "v1.2.3\n" > "$SOURCE_VERSION_FILE"
    printf "#!/bin/sh\n. \"$(dirname "$0")/lib/colors.sh\"\n" > "$SOURCE_MANAGER_SCRIPT"
    chmod +x "$SOURCE_MANAGER_SCRIPT"
    mkdir -p "$(dirname "$SOURCE_MANAGER_SCRIPT")/lib"
    printf "# colors fixture\n" > "$(dirname "$SOURCE_MANAGER_SCRIPT")/lib/colors.sh"

    current_script_path() { printf "%s" "$SOURCE_MANAGER_SCRIPT"; }

    source ./lib/utils.sh
    source ./lib/config.sh
    source ./lib/release.sh
    source ./lib/install.sh

    launcher_path="$(install_from_source)" || exit 1
    [ "$launcher_path" = "$LAUNCHER_PATH" ] || exit 1
    [ -x "$BIN_PATH" ] || exit 1
    [ -x "$INSTALL_DIR/$PERSIST_MANAGER_NAME" ] || exit 1
    [ -f "$INSTALL_DIR/lib/colors.sh" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$VERSION_FILE")" = "v1.2.3" ] || exit 1
    [ -x "$LAUNCHER_PATH" ] || exit 1
  ' _ "$SOURCE_BIN" "$SOURCE_VERSION_FILE" "$SOURCE_MANAGER_SCRIPT" "$INSTALL_DIR" "$BIN_PATH" "$VERSION_FILE" "$PERSIST_MANAGER_NAME" "$LAUNCHER_PATH" "$LAUNCHER_NAME"

  [ "$status" -eq 0 ]
}

@test "install_persistent_from_source writes persistent state and launcher" {
  run bash -c '
    SOURCE_BIN="$1"
    SOURCE_VERSION_FILE="$2"
    SOURCE_MANAGER_SCRIPT="$3"
    PERSIST_STATE_DIR="$4"
    PERSIST_PATH_FILE="$5"
    PERSIST_VERSION_FILE="$6"
    PERSIST_MANAGER_NAME="$7"
    LAUNCHER_PATH="$8"
    target_dir="$9"

    mkdir -p "$(dirname "$SOURCE_BIN")" "$(dirname "$SOURCE_MANAGER_SCRIPT")" "$(dirname "$LAUNCHER_PATH")"
    printf "#!/bin/sh\nexit 0\n" > "$SOURCE_BIN"
    chmod +x "$SOURCE_BIN"
    printf "v1.2.3\n" > "$SOURCE_VERSION_FILE"
    printf "#!/bin/sh\nexit 0\n" > "$SOURCE_MANAGER_SCRIPT"
    chmod +x "$SOURCE_MANAGER_SCRIPT"

    current_script_path() { printf "%s" "$SOURCE_MANAGER_SCRIPT"; }

    source ./lib/utils.sh
    source ./lib/config.sh
    source ./lib/release.sh
    source ./lib/install.sh

    launcher_path="$(install_persistent_from_source "$target_dir")" || exit 1
    [ "$launcher_path" = "$LAUNCHER_PATH" ] || exit 1
    [ -x "$target_dir/tg-ws-proxy" ] || exit 1
    [ -x "$target_dir/$PERSIST_MANAGER_NAME" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_PATH_FILE")" = "$target_dir" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_VERSION_FILE")" = "v1.2.3" ] || exit 1
  ' _ "$SOURCE_BIN" "$SOURCE_VERSION_FILE" "$SOURCE_MANAGER_SCRIPT" "$PERSIST_STATE_DIR" "$PERSIST_PATH_FILE" "$PERSIST_VERSION_FILE" "$PERSIST_MANAGER_NAME" "$LAUNCHER_PATH" "$tmpdir/persistent-install"

  [ "$status" -eq 0 ]
}
