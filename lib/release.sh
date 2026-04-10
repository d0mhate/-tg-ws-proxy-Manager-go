# release.sh

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

    printf "%s" "$DEFAULT_BINARY_NAME"
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

required_persistent_kb() {
    size_kb="$(source_binary_size_kb 2>/dev/null || true)"
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
            if command -v wget >/dev/null 2>&1; then
                _lrt_tag="$(wget -qO - "$RELEASE_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | sed -n '1p')"
            elif command -v curl >/dev/null 2>&1; then
                _lrt_tag="$(curl -fsSL "$RELEASE_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | sed -n '1p')"
            else
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

    if command -v wget >/dev/null 2>&1; then
        wget -qO - "$RELEASES_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | while IFS= read -r raw_tag; do
            normalized_tag="$(normalize_version "$raw_tag" 2>/dev/null || true)"
            [ -n "$normalized_tag" ] || continue
            release_tag_meets_minimum "$normalized_tag" || continue
            printf "%s\n" "$normalized_tag"
        done | awk '!seen[$0]++' | sed -n "1,${max_items}p"
        return 0
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$RELEASES_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | while IFS= read -r raw_tag; do
            normalized_tag="$(normalize_version "$raw_tag" 2>/dev/null || true)"
            [ -n "$normalized_tag" ] || continue
            release_tag_meets_minimum "$normalized_tag" || continue
            printf "%s\n" "$normalized_tag"
        done | awk '!seen[$0]++' | sed -n "1,${max_items}p"
        return 0
    fi

    return 1
}

release_url_reachable() {
    url="$(resolved_release_url)"
    if command -v wget >/dev/null 2>&1; then
        wget --spider "$url" >/dev/null 2>&1
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -I -L --fail "$url" >/dev/null 2>&1
        return $?
    fi

    return 1
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

download_binary() {
    mkdir -p "$(dirname "$SOURCE_BIN")" || return 1
    url="$(resolved_release_url)"

    if command -v wget >/dev/null 2>&1; then
        wget -O "$SOURCE_BIN" "$url"
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -L --fail -o "$SOURCE_BIN" "$url"
        return $?
    fi

    return 1
}

download_manager_script() {
    ref="$1"
    url="$(script_release_url "$ref")"

    mkdir -p "$(dirname "$SOURCE_MANAGER_SCRIPT")" || return 1

    if command -v wget >/dev/null 2>&1; then
        wget -O "$SOURCE_MANAGER_SCRIPT" "$url" >/dev/null 2>&1 || return 1
        chmod +x "$SOURCE_MANAGER_SCRIPT" || return 1
        return 0
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -L --fail -o "$SOURCE_MANAGER_SCRIPT" "$url" >/dev/null 2>&1 || return 1
        chmod +x "$SOURCE_MANAGER_SCRIPT" || return 1
        return 0
    fi

    return 1
}

write_source_version_file() {
    version="$1"
    [ -n "$version" ] || return 0
    printf "%s\n" "$version" > "$SOURCE_VERSION_FILE" || return 1
}