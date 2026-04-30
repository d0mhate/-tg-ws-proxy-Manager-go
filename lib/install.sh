#!/bin/sh
# install.sh

select_persistent_dir() {
    required_kb="$1"

    for dir in $PERSISTENT_DIR_CANDIDATES; do
        free_kb="$(path_available_kb "$dir")"
        [ -n "$free_kb" ] || continue
        if [ "$free_kb" -ge "$required_kb" ]; then
            printf "%s" "$dir"
            return 0
        fi
    done

    return 1
}

current_launcher_path() {
    if [ -f "$LAUNCHER_PATH" ]; then
        printf "%s" "$LAUNCHER_PATH"
        return 0
    fi

    if [ -f "/tmp/$LAUNCHER_NAME" ]; then
        printf "%s" "/tmp/$LAUNCHER_NAME"
        return 0
    fi

    return 1
}

install_launcher() {
    script_target="$1"
    target="$LAUNCHER_PATH"

    if ! mkdir -p "$(dirname "$target")" 2>/dev/null; then
        target="/tmp/$LAUNCHER_NAME"
    fi

    if ! {
        printf '#!/bin/sh\n'
        printf 'sh %s "$@"\n' "$script_target"
    } > "$target" 2>/dev/null; then
        target="/tmp/$LAUNCHER_NAME"
        {
            printf '#!/bin/sh\n'
            printf 'sh %s "$@"\n' "$script_target"
        } > "$target" || return 1
    fi

    chmod +x "$target" || return 1
    printf "%s" "$target"
}

copy_manager_bundle() {
    src_script="$1"
    dest_script="$2"
    fallback_script="${3:-}"

    mkdir -p "$(dirname "$dest_script")" || return 1
    cp "$src_script" "$dest_script" || return 1
    chmod +x "$dest_script" || return 1

    src_lib_dir="$(dirname "$src_script")/lib"
    if [ ! -d "$src_lib_dir" ] && [ -n "$fallback_script" ]; then
        fallback_lib_dir="$(dirname "$fallback_script")/lib"
        if [ -d "$fallback_lib_dir" ]; then
            src_lib_dir="$fallback_lib_dir"
        fi
    fi

    if [ -d "$src_lib_dir" ]; then
        dest_lib_dir="$(dirname "$dest_script")/lib"
        rm -rf "$dest_lib_dir"
        cp -R "$src_lib_dir" "$dest_lib_dir" || return 1
    fi
}

copy_current_manager_script() {
    current_script="$(current_script_path 2>/dev/null || true)"
    [ -n "$current_script" ] || current_script="$0"
    copy_manager_bundle "$current_script" "$SOURCE_MANAGER_SCRIPT"
}

refresh_current_manager_script_from_source() {
    current_script="$(current_script_path 2>/dev/null || true)"
    [ -n "$current_script" ] || return 0
    [ -f "$SOURCE_MANAGER_SCRIPT" ] || return 0

    source_script="$(canonical_path "$SOURCE_MANAGER_SCRIPT")"
    [ "$current_script" = "$source_script" ] && return 0

    tmp_manager="$(canonical_path "$INSTALL_DIR/$PERSIST_MANAGER_NAME")"
    [ "$current_script" = "$tmp_manager" ] && return 0

    [ -w "$current_script" ] || return 0

    copy_manager_bundle "$SOURCE_MANAGER_SCRIPT" "$current_script" "$current_script"
}

ensure_source_manager_current() {
    ref="$1"
    strict="${2:-0}"

    if [ -n "$ref" ] && download_manager_script "$ref"; then
        return 0
    fi

    if [ "$strict" = "1" ] && [ -n "$ref" ]; then
        return 1
    fi

    if [ -x "$SOURCE_MANAGER_SCRIPT" ]; then
        return 0
    fi

    copy_current_manager_script
}

install_from_source() {
    mkdir -p "$INSTALL_DIR" || return 1
    cp "$SOURCE_BIN" "$BIN_PATH" || return 1
    chmod +x "$BIN_PATH" || return 1
    current_script="$(current_script_path 2>/dev/null || true)"
    copy_manager_bundle "$SOURCE_MANAGER_SCRIPT" "$INSTALL_DIR/$PERSIST_MANAGER_NAME" "$current_script" || return 1

    version="$(cached_source_version 2>/dev/null || true)"
    if [ -n "$version" ]; then
        printf "%s\n" "$version" > "$VERSION_FILE" || return 1
    else
        rm -f "$VERSION_FILE"
    fi

    if has_persistent_install; then
        launcher_path="$(current_launcher_path 2>/dev/null || true)"
    else
        launcher_path="$(install_launcher "$INSTALL_DIR/$PERSIST_MANAGER_NAME")" || return 1
    fi
    printf "%s" "$launcher_path"
}

install_persistent_from_source() {
    install_dir="$1"

    mkdir -p "$install_dir" || return 1
    cp "$SOURCE_BIN" "$install_dir/tg-ws-proxy" || return 1
    chmod +x "$install_dir/tg-ws-proxy" || return 1
    current_script="$(current_script_path 2>/dev/null || true)"
    copy_manager_bundle "$SOURCE_MANAGER_SCRIPT" "$install_dir/$PERSIST_MANAGER_NAME" "$current_script" || return 1

    version="$(cached_source_version 2>/dev/null || true)"
    write_persistent_state "$install_dir" "$version" || return 1
    launcher_path="$(install_launcher "$install_dir/$PERSIST_MANAGER_NAME")" || return 1
    printf "%s" "$launcher_path"
}

ensure_source_binary_current() {
    desired_ref="$(resolved_release_ref 2>/dev/null || true)"
    preview_branch="$(selected_preview_branch 2>/dev/null || true)"

    if [ -n "$desired_ref" ]; then
        cached_tag="$(cached_source_version 2>/dev/null || true)"
        need_download="0"

        if [ ! -f "$SOURCE_BIN" ]; then
            printf "%sLocal binary not found%s\n\n" "$C_YELLOW" "$C_RESET"
            need_download="1"
        elif [ -n "$preview_branch" ]; then
            need_download="1"
        elif [ -z "$cached_tag" ]; then
            printf "%sLocal binary version is unknown%s\n\n" "$C_YELLOW" "$C_RESET"
            need_download="1"
        elif [ "$cached_tag" != "$desired_ref" ]; then
            printf "%sLocal binary is outdated%s\n\n" "$C_YELLOW" "$C_RESET"
            printf "Cached version: %s\n" "$cached_tag"
            if [ -n "$(selected_release_tag 2>/dev/null || true)" ]; then
                printf "Selected version: %s\n\n" "$desired_ref"
            else
                printf "Latest version: %s\n\n" "$desired_ref"
            fi
            need_download="1"
        fi

        if [ "$need_download" = "1" ]; then
            release_url="$(resolved_release_url)"
            printf "Trying to download from GitHub Release\n"
            printf "%s\n\n" "$release_url"
            if ! release_url_reachable; then
                if [ -f "$SOURCE_BIN" ]; then
                    printf "%sRelease URL is not reachable%s\n\n" "$C_YELLOW" "$C_RESET"
                    printf "Using local cached binary\n"
                else
                    printf "%sRelease URL is not reachable%s\n\n" "$C_RED" "$C_RESET"
                    printf "Check GitHub Release visibility or network access\n"
                    return 1
                fi
            else
                rm -f "$SOURCE_BIN" "$SOURCE_VERSION_FILE"
                if ! download_binary; then
                    printf "%sDownload failed%s\n\n" "$C_RED" "$C_RESET"
                    printf "You can also place the binary here manually\n"
                    printf "  %s\n" "$SOURCE_BIN"
                    return 1
                fi
                write_source_version_file "$desired_ref" || return 1
            fi
        fi
    elif [ ! -f "$SOURCE_BIN" ]; then
        printf "%sCould not detect latest release version%s\n\n" "$C_RED" "$C_RESET"
        printf "GitHub API rate limit may be exceeded (60 req/hour per IP)\n"
        printf "Use Advanced > Configure update source to set version manually\n"
        printf "Or check GitHub API access or network access\n"
        return 1
    fi
}

check_tmp_space() {
    required_kb="${1:-$REQUIRED_TMP_KB}"
    free_kb="$(tmp_available_kb)"
    [ -n "$free_kb" ] || return 0
    [ "$free_kb" -ge "$required_kb" ]
}

install_binary() {
    show_header
    show_environment_checks
    printf "\n"

    need_kb="$(required_tmp_runtime_install_kb)"
    if ! check_tmp_space "$need_kb"; then
        free_kb="$(tmp_available_kb)"
        printf "%sNot enough free space in /tmp%s\n\n" "$C_RED" "$C_RESET"
        printf "Required: %s KB\n" "$need_kb"
        printf "Available: %s KB\n" "${free_kb:-unknown}"
        pause
        return 1
    fi

    if ! ensure_source_binary_current; then
        pause
        return 1
    fi
    release_ref="$(resolved_release_ref 2>/dev/null || true)"
    if ! ensure_source_manager_current "$release_ref"; then
        pause
        return 1
    fi

    launcher_path="$(install_from_source)" || return 1
    write_update_source_state "$(selected_update_channel 2>/dev/null || printf release)" "$(selected_update_ref 2>/dev/null || printf latest)" || return 1

    show_header
    printf "%sBinary installed%s\n\n" "$C_GREEN" "$C_RESET"
    printf "Source:\n  %s\n\n" "$SOURCE_BIN"
    printf "Installed to:\n  %s\n" "$BIN_PATH"
    version="$(installed_version 2>/dev/null || true)"
    if [ -n "$version" ]; then
        printf "\nVersion:\n  %s\n" "$version"
    fi
    if [ -n "$launcher_path" ]; then
        printf "\nLauncher:\n  %s\n" "$launcher_path"
    fi
    pause
}

install_persistent_binary() {
    if ! ensure_source_manager_current "$(resolved_release_ref 2>/dev/null || true)"; then
        return 1
    fi

    need_kb="$(required_persistent_kb)"
    target_dir="$(select_persistent_dir "$need_kb" 2>/dev/null || true)"
    if [ -z "$target_dir" ]; then
        return 1
    fi

    install_persistent_from_source "$target_dir"
}

show_persistent_install_failure() {
    need_kb="$(required_persistent_kb)"
    printf "%sNo suitable persistent path found%s\n\n" "$C_RED" "$C_RESET"
    printf "Need about: %s KB in persistent storage\n" "$need_kb"
    for candidate in $PERSISTENT_DIR_CANDIDATES; do
        printf "  %s : %s KB free\n" "$candidate" "$(path_available_kb "$candidate" 2>/dev/null || printf unknown)"
    done
}

update_binary() {
    show_header
    show_environment_checks
    printf "\n"

    need_kb="$(required_tmp_runtime_install_kb)"
    if ! check_tmp_space "$need_kb"; then
        free_kb="$(tmp_available_kb)"
        printf "%sNot enough free space in /tmp%s\n\n" "$C_RED" "$C_RESET"
        printf "Required: %s KB\n" "$need_kb"
        printf "Available: %s KB\n" "${free_kb:-unknown}"
        pause
        return 1
    fi

    release_ref="$(resolved_release_ref 2>/dev/null || true)"
    if [ -z "$release_ref" ]; then
        if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
            printf "%sPreview branch is not configured%s\n\n" "$C_RED" "$C_RESET"
            printf "Set a preview branch first in Advanced -> Configure update source\n"
        else
            printf "%sCould not detect latest release version%s\n\n" "$C_RED" "$C_RESET"
            printf "GitHub API rate limit may be exceeded (60 req/hour per IP)\n"
            printf "Use Advanced > Configure update source to set version manually\n"
            printf "Or check GitHub API access or network access\n"
        fi
        pause
        return 1
    fi

    current_tag="$(installed_version 2>/dev/null || true)"
    if [ -z "$current_tag" ]; then
        current_tag="$(persistent_installed_version 2>/dev/null || true)"
    fi
    if [ "$(selected_update_channel 2>/dev/null || true)" != "preview" ] && [ -n "$current_tag" ] && [ "$current_tag" = "$release_ref" ] && [ -x "$BIN_PATH" ] && ! has_persistent_install; then
        if [ -n "$(selected_release_tag 2>/dev/null || true)" ]; then
            printf "%sAlready on the selected version%s\n\n" "$C_GREEN" "$C_RESET"
        else
            printf "%sAlready on the latest version%s\n\n" "$C_GREEN" "$C_RESET"
        fi
        printf "Current version: %s\n" "$current_tag"
        pause
        return 0
    fi

    printf "Current version: %s\n" "${current_tag:-unknown}"
    if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
        printf "Preview branch: %s\n\n" "$release_ref"
    elif [ -n "$(selected_release_tag 2>/dev/null || true)" ]; then
        printf "Selected version: %s\n\n" "$release_ref"
    else
        printf "Latest version: %s\n\n" "$release_ref"
    fi

    if ! release_url_reachable; then
        printf "%sRelease URL is not reachable%s\n\n" "$C_RED" "$C_RESET"
        if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
            printf "Check preview branch artifacts or network access\n"
        else
            printf "Check GitHub Release visibility or network access\n"
        fi
        pause
        return 1
    fi

    rm -f "$SOURCE_BIN" "$SOURCE_VERSION_FILE"
    if ! download_binary; then
        printf "%sDownload failed%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi
    write_source_version_file "$release_ref" || return 1
    if ! ensure_source_manager_current "$release_ref" "1"; then
        printf "%sManager script update failed%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi
    refresh_current_manager_script_from_source || true
    launcher_path="$(install_from_source)" || return 1
    write_update_source_state "$(selected_update_channel 2>/dev/null || printf release)" "$(selected_update_ref 2>/dev/null || printf latest)" || return 1
    if has_persistent_install; then
        persist_dir="$(persistent_install_dir 2>/dev/null || true)"
        if [ -n "$persist_dir" ]; then
            launcher_path="$(install_persistent_from_source "$persist_dir")" || return 1
        fi
    fi

    show_header
    printf "%sUpdated to %s%s\n\n" "$C_GREEN" "$release_ref" "$C_RESET"
    printf "Installed to:\n  %s\n" "$BIN_PATH"
    if [ -n "$launcher_path" ]; then
        printf "\nLauncher:\n  %s\n" "$launcher_path"
    fi
    if [ "$COMMAND_MODE" = "0" ]; then
        printf "\n%sRestarting menu...%s\n" "$C_GREEN" "$C_RESET"
        sleep 1
        restart_target="$(current_launcher_path 2>/dev/null || current_script_path 2>/dev/null || printf "%s" "$0")"
        exec "$restart_target" || {
            printf "%sPlease restart the menu manually.%s\n" "$C_YELLOW" "$C_RESET"
            pause
        }
    else
        pause
    fi
}

remove_all() {
    stop_running >/dev/null 2>&1 || true
    if [ -f "$INIT_SCRIPT_PATH" ]; then
        "$INIT_SCRIPT_PATH" disable >/dev/null 2>&1 || true
        "$INIT_SCRIPT_PATH" stop >/dev/null 2>&1 || true
    fi

    persist_dir="$(persistent_install_dir 2>/dev/null || true)"
    rm -rf "$INSTALL_DIR"
    if [ -n "$persist_dir" ]; then
        rm -rf "$persist_dir"
    fi
    rm -f "$SOURCE_BIN" "$SOURCE_VERSION_FILE"
    rm -f "$SOURCE_MANAGER_SCRIPT"
    rm -f "$PID_FILE"
    rm -rf "$PERSIST_STATE_DIR"
    rm -f "$INIT_SCRIPT_PATH"
    rm -f "$LAUNCHER_PATH" "/tmp/$LAUNCHER_NAME"

    show_header
    printf "%sBinary launcher autostart and downloaded files removed%s\n" "$C_GREEN" "$C_RESET"
    if [ "$COMMAND_MODE" = "0" ]; then
        printf "Menu session closed because manager files were removed.\n"
        return 20
    fi
    pause
}
