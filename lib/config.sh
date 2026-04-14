# config.sh

read_config_value() {
    key="$1"
    [ -f "$PERSIST_CONFIG_FILE" ] || return 1
    sed -n "s/^${key}='\(.*\)'$/\1/p" "$PERSIST_CONFIG_FILE" 2>/dev/null | head -n 1
}

normalize_dc_ip_list() {
    value="$(printf "%s" "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    [ -n "$value" ] || return 1

    awk -v input="$value" '
        function trim(s) {
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
            return s
        }
        BEGIN {
            count = split(input, parts, ",")
            if (count < 1) exit 1
            out = ""
            for (i = 1; i <= count; i++) {
                part = trim(parts[i])
                if (part == "") exit 1
                n = split(part, pair, ":")
                if (n != 2) exit 1
                dc = trim(pair[1])
                ip = trim(pair[2])
                if (dc !~ /^[0-9]+$/) exit 1
                octets = split(ip, ipParts, ".")
                if (octets != 4) exit 1
                for (j = 1; j <= 4; j++) {
                    if (ipParts[j] !~ /^[0-9]+$/) exit 1
                    if (ipParts[j] < 0 || ipParts[j] > 255) exit 1
                }
                if (out != "") out = out ", "
                out = out dc ":" ip
            }
            print out
            exit 0
        }
    '
}

write_settings_config() {
    bin_path="${1:-}"

    if [ -z "$bin_path" ]; then
        bin_path="$(read_config_value BIN 2>/dev/null || true)"
    fi

    auth_settings_valid || return 1

    mkdir -p "$PERSIST_STATE_DIR" || return 1
    {
        if [ -n "$bin_path" ]; then
            printf "BIN='%s'\n" "$bin_path"
        fi
        printf "HOST='%s'\n" "$LISTEN_HOST"
        printf "PORT='%s'\n" "$LISTEN_PORT"
        printf "VERBOSE='%s'\n" "$VERBOSE"
        printf "USERNAME='%s'\n" "$SOCKS_USERNAME"
        printf "PASSWORD='%s'\n" "$SOCKS_PASSWORD"
        printf "DC_IPS='%s'\n" "$DC_IPS"
        printf "CF_PROXY='%s'\n" "$CF_PROXY"
        printf "CF_PROXY_FIRST='%s'\n" "$CF_PROXY_FIRST"
        printf "CF_DOMAIN='%s'\n" "$CF_DOMAIN"
    } > "$PERSIST_CONFIG_FILE" || return 1
}

load_saved_settings() {
    [ -f "$PERSIST_CONFIG_FILE" ] || return 0

    if [ -z "$LISTEN_HOST_FROM_ENV" ]; then
        host="$(read_config_value HOST 2>/dev/null || true)"
        [ -n "$host" ] && LISTEN_HOST="$host"
    fi

    if [ -z "$LISTEN_PORT_FROM_ENV" ]; then
        port="$(read_config_value PORT 2>/dev/null || true)"
        [ -n "$port" ] && LISTEN_PORT="$port"
    fi

    if [ -z "$VERBOSE_FROM_ENV" ]; then
        verbose_value="$(read_config_value VERBOSE 2>/dev/null || true)"
        [ -n "$verbose_value" ] && VERBOSE="$verbose_value"
    fi

    if [ -z "$SOCKS_USERNAME_FROM_ENV" ]; then
        SOCKS_USERNAME="$(read_config_value USERNAME 2>/dev/null || true)"
    fi

    if [ -z "$SOCKS_PASSWORD_FROM_ENV" ]; then
        SOCKS_PASSWORD="$(read_config_value PASSWORD 2>/dev/null || true)"
    fi

    if [ -z "$DC_IPS_FROM_ENV" ]; then
        DC_IPS="$(read_config_value DC_IPS 2>/dev/null || true)"
    fi

    if [ -z "$CF_PROXY_FROM_ENV" ]; then
        cf_proxy_value="$(read_config_value CF_PROXY 2>/dev/null || true)"
        [ -n "$cf_proxy_value" ] && CF_PROXY="$cf_proxy_value"
    fi

    if [ -z "$CF_PROXY_FIRST_FROM_ENV" ]; then
        cf_proxy_first_value="$(read_config_value CF_PROXY_FIRST 2>/dev/null || true)"
        [ -n "$cf_proxy_first_value" ] && CF_PROXY_FIRST="$cf_proxy_first_value"
    fi

    if [ -z "$CF_DOMAIN_FROM_ENV" ]; then
        CF_DOMAIN="$(read_config_value CF_DOMAIN 2>/dev/null || true)"
    fi
}

selected_update_channel() {
    case "$UPDATE_CHANNEL" in
        preview)
            printf "preview"
            return 0
            ;;
        ""|release)
            ;;
        *)
            return 1
            ;;
    esac

    if [ "$UPDATE_CHANNEL_FROM_ENV" = x ]; then
        printf "release"
        return 0
    fi

    value="$(read_first_line "$PERSIST_UPDATE_CHANNEL_FILE" 2>/dev/null || true)"
    case "$value" in
        preview)
            printf "preview"
            return 0
            ;;
    esac

    printf "release"
}

selected_preview_branch() {
    if [ "$(selected_update_channel 2>/dev/null || true)" != "preview" ]; then
        return 1
    fi

    selected_preview_branch_value
}

selected_preview_branch_value() {
    if [ "$PREVIEW_BRANCH_FROM_ENV" = x ]; then
        [ -n "$PREVIEW_BRANCH" ] || return 1
        printf "%s" "$PREVIEW_BRANCH"
        return 0
    fi

    value="$(read_first_line "$PERSIST_PREVIEW_BRANCH_FILE" 2>/dev/null || true)"
    [ -n "$value" ] || return 1
    printf "%s" "$value"
}

selected_update_ref() {
    if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
        selected_preview_branch
        return $?
    fi

    tag="$(selected_release_tag 2>/dev/null || true)"
    if [ -n "$tag" ]; then
        printf "%s" "$tag"
        return 0
    fi

    printf "latest"
}

write_update_source_state() {
    channel="$1"
    ref="$2"

    case "$channel" in
        preview)
            [ -n "$ref" ] || return 1
            mkdir -p "$PERSIST_STATE_DIR" || return 1
            printf "preview\n" > "$PERSIST_UPDATE_CHANNEL_FILE" || return 1
            printf "%s\n" "$ref" > "$PERSIST_PREVIEW_BRANCH_FILE" || return 1
            rm -f "$PERSIST_RELEASE_TAG_FILE"
            return 0
            ;;
        release)
            mkdir -p "$PERSIST_STATE_DIR" || return 1
            printf "release\n" > "$PERSIST_UPDATE_CHANNEL_FILE" || return 1
            rm -f "$PERSIST_PREVIEW_BRANCH_FILE"
            write_release_tag_state "$ref" || return 1
            return 0
            ;;
    esac

    return 1
}

auth_settings_valid() {
    if [ -n "$SOCKS_USERNAME" ] && [ -n "$SOCKS_PASSWORD" ]; then
        return 0
    fi

    if [ -z "$SOCKS_USERNAME" ] && [ -z "$SOCKS_PASSWORD" ]; then
        return 0
    fi

    return 1
}

show_invalid_auth_settings() {
    printf "%sSOCKS5 auth settings are incomplete%s\n\n" "$C_RED" "$C_RESET"
    printf "SOCKS_USERNAME and SOCKS_PASSWORD must be both set or both empty.\n"
}

persistent_install_dir() {
    value="$(read_first_line "$PERSIST_PATH_FILE" 2>/dev/null || true)"
    [ -n "$value" ] || return 1
    printf "%s" "$value"
}

persistent_bin_path() {
    dir="$(persistent_install_dir 2>/dev/null || true)"
    [ -n "$dir" ] || return 1
    printf "%s/tg-ws-proxy" "$dir"
}

persistent_manager_path() {
    dir="$(persistent_install_dir 2>/dev/null || true)"
    [ -n "$dir" ] || return 1
    printf "%s/%s" "$dir" "$PERSIST_MANAGER_NAME"
}

has_persistent_install() {
    bin="$(persistent_bin_path 2>/dev/null || true)"
    [ -n "$bin" ] || return 1
    [ -x "$bin" ]
}

write_persistent_state() {
    install_dir="$1"
    version="$2"

    mkdir -p "$PERSIST_STATE_DIR" || return 1
    printf "%s\n" "$install_dir" > "$PERSIST_PATH_FILE" || return 1
    if [ -n "$version" ]; then
        printf "%s\n" "$version" > "$PERSIST_VERSION_FILE" || return 1
    else
        rm -f "$PERSIST_VERSION_FILE"
    fi
}

write_autostart_config() {
    bin_path="$1"
    write_settings_config "$bin_path"
}

sync_autostart_config_if_enabled() {
    if ! autostart_enabled; then
        return 0
    fi

    bin_path="$(persistent_bin_path 2>/dev/null || true)"
    if [ -z "$bin_path" ] || [ ! -x "$bin_path" ]; then
        return 0
    fi

    write_autostart_config "$bin_path"
}
