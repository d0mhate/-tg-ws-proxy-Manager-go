#!/bin/sh
# release.sh

network_fetch() {
    url="$1"

    if command -v wget >/dev/null 2>&1; then
        wget -q -T "$NETWORK_TIMEOUT_SEC" -O - "$url" 2>/dev/null
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --connect-timeout "$NETWORK_CONNECT_TIMEOUT_SEC" --max-time "$NETWORK_TIMEOUT_SEC" "$url" 2>/dev/null
        return $?
    fi

    return 1
}

network_head() {
    url="$1"

    if command -v wget >/dev/null 2>&1; then
        wget -q --server-response --spider -T "$NETWORK_TIMEOUT_SEC" "$url" 2>&1
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -fsSLI --connect-timeout "$NETWORK_CONNECT_TIMEOUT_SEC" --max-time "$NETWORK_TIMEOUT_SEC" "$url" 2>/dev/null
        return $?
    fi

    return 1
}

network_probe() {
    url="$1"

    if command -v wget >/dev/null 2>&1; then
        wget -q --spider -T "$NETWORK_TIMEOUT_SEC" "$url" >/dev/null 2>&1
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -I -L --fail --connect-timeout "$NETWORK_CONNECT_TIMEOUT_SEC" --max-time "$NETWORK_TIMEOUT_SEC" "$url" >/dev/null 2>&1
        return $?
    fi

    return 1
}

network_download() {
    url="$1"
    dest="$2"

    if command -v wget >/dev/null 2>&1; then
        wget -T "$DOWNLOAD_TIMEOUT_SEC" -O "$dest" "$url"
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -L --fail --connect-timeout "$NETWORK_CONNECT_TIMEOUT_SEC" --max-time "$DOWNLOAD_TIMEOUT_SEC" -o "$dest" "$url"
        return $?
    fi

    return 1
}

resolved_binary_name() {
    if [ -n "$BINARY_NAME" ]; then
        printf "%s" "$BINARY_NAME"
        return 0
    fi

    if is_openwrt; then
        arch="$(openwrt_arch)"
        if [ -n "$arch" ]; then
            binary_name_for_arch "$arch"
            return 0
        fi
    fi

    generic_binary_name
}

resolved_release_url() {
    if [ -n "$RELEASE_URL" ]; then
        printf "%s" "$RELEASE_URL"
        return 0
    fi

    preview_branch="$(selected_preview_branch 2>/dev/null || true)"
    if [ -n "$preview_branch" ]; then
        printf "%s/%s/%s" "$PREVIEW_BASE_URL" "$preview_branch" "$(resolved_binary_name)"
        return 0
    fi

    tag="$(selected_release_tag 2>/dev/null || true)"
    if [ -n "$tag" ]; then
        printf "%s/%s/%s" "$SCRIPT_RELEASE_BASE_URL" "$tag" "$(resolved_binary_name)"
        return 0
    fi

    printf "%s/%s" "$RELEASE_DOWNLOAD_BASE_URL" "$(resolved_binary_name)"
}

source_binary_size_kb() {
    if [ ! -f "$SOURCE_BIN" ]; then
        return 1
    fi
    bytes="$(wc -c < "$SOURCE_BIN" 2>/dev/null | tr -d ' ')"
    [ -n "$bytes" ] || return 1
    printf "%s" $(( (bytes + 1023) / 1024 ))
}

remote_url_size_bytes() {
    url="$1"
    [ -n "$url" ] || return 1

    case "$url" in
        file://*)
            local_path="${url#file://}"
            [ -f "$local_path" ] || return 1
            bytes="$(wc -c < "$local_path" 2>/dev/null | tr -d ' ')"
            [ -n "$bytes" ] || return 1
            printf "%s" "$bytes"
            return 0
            ;;
    esac

    bytes="$(network_head "$url" | tr -d '\r' | awk 'tolower($1) == "content-length:" {print $2}' | tail -n 1)"
    case "$bytes" in
        ''|*[!0-9]*)
            ;;
        *)
            printf "%s" "$bytes"
            return 0
            ;;
    esac

    if command -v wget >/dev/null 2>&1; then
        bytes="$(wget --server-response --spider -T "$NETWORK_TIMEOUT_SEC" "$url" 2>&1 | tr -d '\r' | awk 'tolower($1) == "content-length:" {print $2}' | tail -n 1)"
        case "$bytes" in
            ''|*[!0-9]*)
                ;;
            *)
                printf "%s" "$bytes"
                return 0
                ;;
        esac
    fi

    return 1
}

remote_url_size_kb() {
    bytes="$(remote_url_size_bytes "$1" 2>/dev/null || true)"
    [ -n "$bytes" ] || return 1
    printf "%s" $(( (bytes + 1023) / 1024 ))
}

resolved_source_binary_size_kb() {
    url="$(resolved_release_url 2>/dev/null || true)"
    if [ -n "$url" ]; then
        size_kb="$(remote_url_size_kb "$url" 2>/dev/null || true)"
        if [ -n "$size_kb" ]; then
            printf "%s" "$size_kb"
            return 0
        fi
    fi

    source_binary_size_kb
}

manager_bundle_size_kb() {
    if [ ! -f "$SOURCE_MANAGER_SCRIPT" ]; then
        return 1
    fi

    script_bytes="$(wc -c < "$SOURCE_MANAGER_SCRIPT" 2>/dev/null | tr -d ' ')"
    [ -n "$script_bytes" ] || return 1
    total_kb=$(( (script_bytes + 1023) / 1024 ))

    lib_dir="$(dirname "$SOURCE_MANAGER_SCRIPT")/lib"
    if [ -d "$lib_dir" ]; then
        lib_bytes="$(find "$lib_dir" -type f -exec wc -c {} + 2>/dev/null | awk '{sum += $1} END {print sum+0}')"
        [ -n "$lib_bytes" ] || return 1
        total_kb=$((total_kb + (lib_bytes + 1023) / 1024))
    fi

    printf "%s" "$total_kb"
}

resolved_manager_bundle_size_kb() {
    ref="$(resolved_release_ref 2>/dev/null || true)"
    if [ -n "$ref" ]; then
        script_url="$(script_release_url "$ref" 2>/dev/null || true)"
        if [ -n "$script_url" ]; then
            size_kb="$(remote_url_size_kb "$script_url" 2>/dev/null || true)"
            if [ -n "$size_kb" ]; then
                printf "%s" "$size_kb"
                return 0
            fi
        fi
    fi

    manager_bundle_size_kb
}

source_payload_size_kb() {
    total_kb=0
    have_size="0"

    if size_kb="$(resolved_source_binary_size_kb 2>/dev/null)"; then
        total_kb=$((total_kb + size_kb))
        have_size="1"
    fi

    if bundle_kb="$(resolved_manager_bundle_size_kb 2>/dev/null)"; then
        total_kb=$((total_kb + bundle_kb))
        have_size="1"
    fi

    [ "$have_size" = "1" ] || return 1
    printf "%s" "$total_kb"
}

required_tmp_download_kb() {
    payload_kb="$(source_payload_size_kb 2>/dev/null || true)"
    if [ -z "$payload_kb" ]; then
        printf "%s" "$REQUIRED_TMP_KB"
        return 0
    fi

    need_kb=$((payload_kb + TMP_SPACE_HEADROOM_KB))
    if [ "$need_kb" -lt "$REQUIRED_TMP_KB" ]; then
        need_kb="$REQUIRED_TMP_KB"
    fi
    printf "%s" "$need_kb"
}

required_tmp_runtime_install_kb() {
    payload_kb="$(source_payload_size_kb 2>/dev/null || true)"
    if [ -z "$payload_kb" ]; then
        printf "%s" $((REQUIRED_TMP_KB * 2))
        return 0
    fi

    need_kb=$((payload_kb * 2 + TMP_SPACE_HEADROOM_KB))
    minimum_kb=$((REQUIRED_TMP_KB * 2))
    if [ "$need_kb" -lt "$minimum_kb" ]; then
        need_kb="$minimum_kb"
    fi
    printf "%s" "$need_kb"
}

required_persistent_kb() {
    size_kb="$(source_payload_size_kb 2>/dev/null || true)"
    if [ -z "$size_kb" ]; then
        printf "%s" "$REQUIRED_TMP_KB"
        return 0
    fi

    need_kb=$((size_kb + PERSISTENT_SPACE_HEADROOM_KB))
    if [ "$need_kb" -lt "$REQUIRED_TMP_KB" ]; then
        need_kb="$REQUIRED_TMP_KB"
    fi
    printf "%s" "$need_kb"
}

normalize_version() {
    value="$1"
    case "$value" in
        v[0-9]*)
            printf "%s" "$value"
            return 0
            ;;
    esac
    return 1
}

version_ge() {
    left="$(normalize_version "$1" 2>/dev/null || true)"
    right="$(normalize_version "$2" 2>/dev/null || true)"
    [ -n "$left" ] || return 1
    [ -n "$right" ] || return 1

    awk -v left="${left#v}" -v right="${right#v}" '
        BEGIN {
            split(left, l, ".")
            split(right, r, ".")
            max = length(l) > length(r) ? length(l) : length(r)
            for (i = 1; i <= max; i++) {
                lv = (i in l) ? l[i] + 0 : 0
                rv = (i in r) ? r[i] + 0 : 0
                if (lv > rv) exit 0
                if (lv < rv) exit 1
            }
            exit 0
        }
    '
}

release_tag_meets_minimum() {
    version_ge "$1" "$MIN_PINNED_RELEASE_TAG"
}

installed_version() {
    value="$(read_first_line "$VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

cached_source_version() {
    value="$(read_first_line "$SOURCE_VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

persistent_release_tag() {
    value="$(read_first_line "$PERSIST_RELEASE_TAG_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

release_tag_requests_latest() {
    case "$RELEASE_TAG" in
        latest|default|none)
            return 0
            ;;
    esac
    return 1
}

selected_release_tag() {
    if release_tag_requests_latest; then
        return 1
    fi

    value="$(normalize_version "$RELEASE_TAG" 2>/dev/null || true)"
    if [ -n "$value" ]; then
        printf "%s" "$value"
        return 0
    fi

    persistent_release_tag
}

resolved_release_ref() {
    preview_branch="$(selected_preview_branch 2>/dev/null || true)"
    if [ -n "$preview_branch" ]; then
        printf "%s" "$preview_branch"
        return 0
    fi

    tag="$(selected_release_tag 2>/dev/null || true)"
    if [ -n "$tag" ]; then
        printf "%s" "$tag"
        return 0
    fi

    latest_release_tag
}

write_release_tag_state() {
    value="$(normalize_version "$1" 2>/dev/null || true)"
    if [ -z "$value" ]; then
        rm -f "$PERSIST_RELEASE_TAG_FILE"
        return 0
    fi

    mkdir -p "$(dirname "$PERSIST_RELEASE_TAG_FILE")" || return 1
    printf "%s\n" "$value" > "$PERSIST_RELEASE_TAG_FILE" || return 1
}

persistent_installed_version() {
    value="$(read_first_line "$PERSIST_VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

latest_release_tag() {
    _lrt_tag=""
    case "$RELEASE_API_URL" in
        file://*)
            local_path="${RELEASE_API_URL#file://}"
            _lrt_tag="$(tr -d '\n' < "$local_path" 2>/dev/null | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | sed -n '1p')"
            ;;
        *)
            if ! _lrt_tag="$(network_fetch "$RELEASE_API_URL" | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | sed -n '1p')"; then
                return 1
            fi
            ;;
    esac
    if [ -z "$_lrt_tag" ]; then
        return 1
    fi
    printf "%s" "$_lrt_tag"
}

recent_release_tags() {
    max_items="${1:-10}"

    case "$RELEASES_API_URL" in
        file://*)
            local_path="${RELEASES_API_URL#file://}"
            tr -d '\n' < "$local_path" 2>/dev/null | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | while IFS= read -r raw_tag; do
                normalized_tag="$(normalize_version "$raw_tag" 2>/dev/null || true)"
                [ -n "$normalized_tag" ] || continue
                release_tag_meets_minimum "$normalized_tag" || continue
                printf "%s\n" "$normalized_tag"
            done | awk '!seen[$0]++' | sed -n "1,${max_items}p"
            return 0
            ;;
    esac

    if network_fetch "$RELEASES_API_URL" | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | while IFS= read -r raw_tag; do
        normalized_tag="$(normalize_version "$raw_tag" 2>/dev/null || true)"
        [ -n "$normalized_tag" ] || continue
        release_tag_meets_minimum "$normalized_tag" || continue
        printf "%s\n" "$normalized_tag"
    done | awk '!seen[$0]++' | sed -n "1,${max_items}p"; then
        return 0
    fi

    return 1
}

_parse_preview_branch_names() {
    tr -d '\n' | sed 's/},[[:space:]]*{/}\
{/g' | while IFS= read -r _ppbn_item || [ -n "$_ppbn_item" ]; do
        case "$_ppbn_item" in
            *'"type":"dir"'*|*'"type": "dir"'*)
                _ppbn_name="$(printf "%s" "$_ppbn_item" | sed -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
                [ -n "$_ppbn_name" ] && printf "%s\n" "$_ppbn_name"
                ;;
        esac
    done
}

list_preview_branches() {
    _lpb_max="${1:-20}"

    case "$PREVIEW_BRANCHES_API_URL" in
        file://*)
            _lpb_path="${PREVIEW_BRANCHES_API_URL#file://}"
            _parse_preview_branch_names < "$_lpb_path" 2>/dev/null | sed -n "1,${_lpb_max}p"
            return 0
            ;;
    esac

    if network_fetch "$PREVIEW_BRANCHES_API_URL" | _parse_preview_branch_names | sed -n "1,${_lpb_max}p"; then
        return 0
    fi

    return 1
}

release_url_reachable() {
    url="$(resolved_release_url)"
    network_probe "$url"
}

script_release_url() {
    preview_branch="$(selected_preview_branch 2>/dev/null || true)"
    if [ -n "$preview_branch" ]; then
        printf "%s/%s/%s" "$PREVIEW_BASE_URL" "$preview_branch" "$PERSIST_MANAGER_NAME"
        return 0
    fi

    ref="$1"
    printf "%s/%s/%s" "$SCRIPT_RELEASE_BASE_URL" "$ref" "$PERSIST_MANAGER_NAME"
}

release_api_payload() {
    case "$RELEASE_API_URL" in
        file://*)
            cat "${RELEASE_API_URL#file://}" 2>/dev/null || return 1
            return 0
            ;;
    esac

    network_fetch "$RELEASE_API_URL"
}

release_asset_digest() {
    asset_name="$1"
    payload="$(release_api_payload 2>/dev/null || true)"
    [ -n "$payload" ] || return 1

    printf "%s" "$payload" | tr -d '\n' | awk -v target="$asset_name" -F'"' '
        {
            current = ""
            for (i = 1; i <= NF; i++) {
                if ($i == "name" && (i + 2) <= NF) {
                    current = $(i + 2)
                    continue
                }
                if ($i == "digest" && current == target && (i + 2) <= NF) {
                    print $(i + 2)
                    exit
                }
            }
        }
    '
}

parse_sha256_digest() {
    digest="$1"
    case "$digest" in
        sha256:????????????????????????????????????????????????????????????????)
            sum="${digest#sha256:}"
            case "$sum" in
                *[!0-9a-fA-F]*)
                    return 1
                    ;;
            esac
            printf "%s" "$sum" | tr '[:upper:]' '[:lower:]'
            return 0
            ;;
    esac
    return 1
}

sha256_file() {
    file_path="$1"

    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file_path" 2>/dev/null | awk '{print $1}'
        return $?
    fi

    if command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$file_path" 2>/dev/null | sed 's/^.*= //'
        return $?
    fi

    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$file_path" 2>/dev/null | awk '{print $1}'
        return $?
    fi

    return 1
}

verify_source_binary() {
    [ -x "$SOURCE_BIN" ] || return 0

    if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
        return 0
    fi

    asset_name="$(resolved_binary_name)"
    raw_digest="$(release_asset_digest "$asset_name" 2>/dev/null || true)"
    [ -n "$raw_digest" ] || return 0

    want_sum="$(parse_sha256_digest "$raw_digest" 2>/dev/null || true)"
    [ -n "$want_sum" ] || return 0

    got_sum="$(sha256_file "$SOURCE_BIN" 2>/dev/null || true)"
    [ -n "$got_sum" ] || return 0

    got_sum="$(printf "%s" "$got_sum" | tr '[:upper:]' '[:lower:]')"
    if [ "$got_sum" = "$want_sum" ]; then
        return 0
    fi

    printf "%sSHA256 mismatch: downloaded binary does not match release digest%s\n" "$C_RED" "$C_RESET"
    printf "The file may be corrupted or tampered with\n"
    return 1
}

download_binary() {
    mkdir -p "$(dirname "$SOURCE_BIN")" || return 1
    url="$(resolved_release_url)"

    if ! network_download "$url" "$SOURCE_BIN"; then
        return 1
    fi

    chmod +x "$SOURCE_BIN" 2>/dev/null || true

    if ! verify_source_binary; then
        rm -f "$SOURCE_BIN"
        return 1
    fi

    return 0
}

download_manager_script() {
    ref="$1"
    url="$(script_release_url "$ref")"

    mkdir -p "$(dirname "$SOURCE_MANAGER_SCRIPT")" || return 1

    if ! network_download "$url" "$SOURCE_MANAGER_SCRIPT" >/dev/null 2>&1; then
        return 1
    fi
    chmod +x "$SOURCE_MANAGER_SCRIPT" || return 1
    return 0
}

write_source_version_file() {
    version="$1"
    [ -n "$version" ] || return 0
    printf "%s\n" "$version" > "$SOURCE_VERSION_FILE" || return 1
}

read_latest_version_cache() {
    value="$(read_first_line "$LATEST_VERSION_CACHE_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

latest_version_cache_is_fresh() {
    [ -f "$LATEST_VERSION_CACHE_FILE" ] || return 1
    ts="$(sed -n '2p' "$LATEST_VERSION_CACHE_FILE" 2>/dev/null || true)"
    [ -n "$ts" ] || return 1
    now="$(date +%s 2>/dev/null || printf "0")"
    age=$((now - ts))
    [ "$age" -lt 3600 ]
}

refresh_latest_version_cache() {
    tag="$(latest_release_tag 2>/dev/null || true)"
    [ -n "$tag" ] || return 0
    mkdir -p "$(dirname "$LATEST_VERSION_CACHE_FILE")" 2>/dev/null || return 0
    ts="$(date +%s 2>/dev/null || printf "0")"
    printf "%s\n%s\n" "$tag" "$ts" > "$LATEST_VERSION_CACHE_FILE" 2>/dev/null || true
}
