#!/usr/bin/env bats

setup() {
  tmpdir="$(mktemp -d)"
  export PERSIST_STATE_DIR="$tmpdir/state"
  export PERSIST_CONFIG_FILE="$PERSIST_STATE_DIR/autostart.conf"
  export PERSIST_PATH_FILE="$PERSIST_STATE_DIR/install_dir"
  export PERSIST_VERSION_FILE="$PERSIST_STATE_DIR/version"
  export PERSIST_RELEASE_TAG_FILE="$PERSIST_STATE_DIR/release_tag"
  export PERSIST_UPDATE_CHANNEL_FILE="$PERSIST_STATE_DIR/update_channel"
  export PERSIST_PREVIEW_BRANCH_FILE="$PERSIST_STATE_DIR/preview_branch"
}

teardown() {
  rm -rf "$tmpdir"
}

@test "write_settings_config writes full config with bin path" {
  run bash -c '
    PERSIST_STATE_DIR="$1"
    PERSIST_CONFIG_FILE="$2"
    LISTEN_HOST="127.0.0.1"
    LISTEN_PORT="9090"
    VERBOSE="1"
    SOCKS_USERNAME="user"
    SOCKS_PASSWORD="pass"
    DC_IPS="1:1.1.1.1, 2:2.2.2.2"
    CF_PROXY="1"
    CF_PROXY_FIRST="1"
    CF_DOMAIN="one.example,two.example"
    PROXY_MODE="mtproto"
    MT_SECRET="00112233445566778899aabbccddeeff"
    MT_LINK_IP="8.8.8.8"
    MT_UPSTREAM_PROXIES="mt1.example:443:00112233445566778899aabbccddeeff"

    source ./lib/config.sh

    write_settings_config "/opt/tg-ws-proxy"
    grep -Fqx "BIN='\''/opt/tg-ws-proxy'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "HOST='\''127.0.0.1'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "PORT='\''9090'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "VERBOSE='\''1'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "USERNAME='\''user'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "PASSWORD='\''pass'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "DC_IPS='\''1:1.1.1.1, 2:2.2.2.2'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "CF_PROXY='\''1'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "CF_PROXY_FIRST='\''1'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "CF_DOMAIN='\''one.example,two.example'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "PROXY_MODE='\''mtproto'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "MT_SECRET='\''00112233445566778899aabbccddeeff'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "MT_LINK_IP='\''8.8.8.8'\''" "$PERSIST_CONFIG_FILE" || exit 1
    grep -Fqx "MT_UPSTREAM_PROXIES='\''mt1.example:443:00112233445566778899aabbccddeeff'\''" "$PERSIST_CONFIG_FILE" || exit 1
  ' _ "$PERSIST_STATE_DIR" "$PERSIST_CONFIG_FILE"

  [ "$status" -eq 0 ]
}

@test "write_settings_config rejects incomplete socks auth" {
  run bash -c '
    PERSIST_STATE_DIR="$1"
    PERSIST_CONFIG_FILE="$2"
    LISTEN_HOST="0.0.0.0"
    LISTEN_PORT="1080"
    VERBOSE="0"
    SOCKS_USERNAME="user"
    SOCKS_PASSWORD=""
    DC_IPS=""
    CF_PROXY="0"
    CF_PROXY_FIRST="0"
    CF_DOMAIN=""
    PROXY_MODE="socks5"
    MT_SECRET=""
    MT_LINK_IP=""
    MT_UPSTREAM_PROXIES=""

    source ./lib/config.sh
    write_settings_config "/opt/tg-ws-proxy"
  ' _ "$PERSIST_STATE_DIR" "$PERSIST_CONFIG_FILE"

  [ "$status" -ne 0 ]
  [ ! -f "$PERSIST_CONFIG_FILE" ]
}

@test "load_saved_settings loads persisted values and keeps env overrides" {
  run bash -c '
    PERSIST_CONFIG_FILE="$1"
    LISTEN_HOST="0.0.0.0"
    LISTEN_PORT="1080"
    VERBOSE="0"
    SOCKS_USERNAME=""
    SOCKS_PASSWORD=""
    DC_IPS=""
    CF_PROXY="0"
    CF_PROXY_FIRST="0"
    CF_DOMAIN=""
    PROXY_MODE="socks5"
    MT_SECRET=""
    MT_LINK_IP=""
    MT_UPSTREAM_PROXIES=""
    LISTEN_HOST_FROM_ENV=""
    LISTEN_PORT_FROM_ENV="x"
    VERBOSE_FROM_ENV=""
    SOCKS_USERNAME_FROM_ENV=""
    SOCKS_PASSWORD_FROM_ENV="x"
    DC_IPS_FROM_ENV=""
    CF_PROXY_FROM_ENV=""
    CF_PROXY_FIRST_FROM_ENV=""
    CF_DOMAIN_FROM_ENV=""
    PROXY_MODE_FROM_ENV=""
    MT_SECRET_FROM_ENV=""
    MT_LINK_IP_FROM_ENV=""
    MT_UPSTREAM_PROXIES_FROM_ENV=""

    mkdir -p "$(dirname "$PERSIST_CONFIG_FILE")"
    cat > "$PERSIST_CONFIG_FILE" <<'\''EOF'\''
HOST='\''10.0.0.1'\''
PORT='\''2080'\''
VERBOSE='\''1'\''
USERNAME='\''saved-user'\''
PASSWORD='\''saved-pass'\''
DC_IPS='\''1:1.1.1.1'\''
CF_PROXY='\''1'\''
CF_PROXY_FIRST='\''1'\''
CF_DOMAIN='\''cf.example'\''
PROXY_MODE='\''mtproto'\''
MT_SECRET='\''00112233445566778899aabbccddeeff'\''
MT_LINK_IP='\''5.6.7.8'\''
MT_UPSTREAM_PROXIES='\''mt.example:443:00112233445566778899aabbccddeeff'\''
EOF

    source ./lib/config.sh
    load_saved_settings

    [ "$LISTEN_HOST" = "10.0.0.1" ] || exit 1
    [ "$LISTEN_PORT" = "1080" ] || exit 1
    [ "$VERBOSE" = "1" ] || exit 1
    [ "$SOCKS_USERNAME" = "saved-user" ] || exit 1
    [ "$SOCKS_PASSWORD" = "" ] || exit 1
    [ "$DC_IPS" = "1:1.1.1.1" ] || exit 1
    [ "$CF_PROXY" = "1" ] || exit 1
    [ "$CF_PROXY_FIRST" = "1" ] || exit 1
    [ "$CF_DOMAIN" = "cf.example" ] || exit 1
    [ "$PROXY_MODE" = "mtproto" ] || exit 1
    [ "$MT_SECRET" = "00112233445566778899aabbccddeeff" ] || exit 1
    [ "$MT_LINK_IP" = "5.6.7.8" ] || exit 1
    [ "$MT_UPSTREAM_PROXIES" = "mt.example:443:00112233445566778899aabbccddeeff" ] || exit 1
  ' _ "$PERSIST_CONFIG_FILE"

  [ "$status" -eq 0 ]
}

@test "selected update helpers prefer env then persisted state" {
  run bash -c '
    PERSIST_STATE_DIR="$1"
    PERSIST_UPDATE_CHANNEL_FILE="$2"
    PERSIST_PREVIEW_BRANCH_FILE="$3"
    PERSIST_RELEASE_TAG_FILE="$4"
    UPDATE_CHANNEL=""
    UPDATE_CHANNEL_FROM_ENV=""
    PREVIEW_BRANCH=""
    PREVIEW_BRANCH_FROM_ENV=""
    RELEASE_TAG=""

    read_first_line() {
      [ -f "$1" ] || return 1
      sed -n '\''1p'\'' "$1"
    }

    selected_release_tag() {
      if [ -n "$RELEASE_TAG" ]; then
        printf "%s" "$RELEASE_TAG"
        return 0
      fi
      if [ -f "$PERSIST_RELEASE_TAG_FILE" ]; then
        sed -n '\''1p'\'' "$PERSIST_RELEASE_TAG_FILE"
        return 0
      fi
      return 1
    }

    source ./lib/config.sh

    [ "$(selected_update_channel)" = "release" ] || exit 1
    [ "$(selected_update_ref)" = "latest" ] || exit 1

    mkdir -p "$PERSIST_STATE_DIR"
    printf "preview\n" > "$PERSIST_UPDATE_CHANNEL_FILE"
    printf "feature-branch\n" > "$PERSIST_PREVIEW_BRANCH_FILE"
    [ "$(selected_update_channel)" = "preview" ] || exit 1
    [ "$(selected_update_ref)" = "feature-branch" ] || exit 1

    UPDATE_CHANNEL="release"
    UPDATE_CHANNEL_FROM_ENV="x"
    RELEASE_TAG="v1.2.3"
    [ "$(selected_update_channel)" = "release" ] || exit 1
    [ "$(selected_update_ref)" = "v1.2.3" ] || exit 1
  ' _ "$PERSIST_STATE_DIR" "$PERSIST_UPDATE_CHANNEL_FILE" "$PERSIST_PREVIEW_BRANCH_FILE" "$PERSIST_RELEASE_TAG_FILE"

  [ "$status" -eq 0 ]
}

@test "write_update_source_state writes preview and release state files" {
  run bash -c '
    PERSIST_STATE_DIR="$1"
    PERSIST_UPDATE_CHANNEL_FILE="$2"
    PERSIST_PREVIEW_BRANCH_FILE="$3"
    PERSIST_RELEASE_TAG_FILE="$4"

    write_release_tag_state() {
      if [ -n "$1" ]; then
        printf "%s\n" "$1" > "$PERSIST_RELEASE_TAG_FILE"
      else
        rm -f "$PERSIST_RELEASE_TAG_FILE"
      fi
    }

    source ./lib/config.sh

    write_update_source_state preview preview-main || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_UPDATE_CHANNEL_FILE")" = "preview" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_PREVIEW_BRANCH_FILE")" = "preview-main" ] || exit 1
    [ ! -f "$PERSIST_RELEASE_TAG_FILE" ] || exit 1

    write_update_source_state release v1.2.3 || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_UPDATE_CHANNEL_FILE")" = "release" ] || exit 1
    [ ! -f "$PERSIST_PREVIEW_BRANCH_FILE" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_RELEASE_TAG_FILE")" = "v1.2.3" ] || exit 1
  ' _ "$PERSIST_STATE_DIR" "$PERSIST_UPDATE_CHANNEL_FILE" "$PERSIST_PREVIEW_BRANCH_FILE" "$PERSIST_RELEASE_TAG_FILE"

  [ "$status" -eq 0 ]
}

@test "write_persistent_state writes install dir and optional version" {
  run bash -c '
    PERSIST_STATE_DIR="$1"
    PERSIST_PATH_FILE="$2"
    PERSIST_VERSION_FILE="$3"

    source ./lib/config.sh

    write_persistent_state "/opt/tg-ws-proxy-go" "v1.2.3" || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_PATH_FILE")" = "/opt/tg-ws-proxy-go" ] || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_VERSION_FILE")" = "v1.2.3" ] || exit 1

    write_persistent_state "/opt/tg-ws-proxy-go" "" || exit 1
    [ "$(sed -n '\''1p'\'' "$PERSIST_PATH_FILE")" = "/opt/tg-ws-proxy-go" ] || exit 1
    [ ! -f "$PERSIST_VERSION_FILE" ] || exit 1
  ' _ "$PERSIST_STATE_DIR" "$PERSIST_PATH_FILE" "$PERSIST_VERSION_FILE"

  [ "$status" -eq 0 ]
}
