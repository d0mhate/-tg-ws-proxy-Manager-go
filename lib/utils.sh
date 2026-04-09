# utils.sh

tmp_available_kb() {
    df -k /tmp 2>/dev/null | awk 'NR==2 {print $4+0}'
}

closest_existing_path() {
    path="$1"

    while [ -n "$path" ] && [ "$path" != "/" ] && [ ! -e "$path" ]; do
        path="$(dirname "$path")"
    done

    if [ -z "$path" ]; then
        printf "/"
        return 0
    fi

    printf "%s" "$path"
}

path_available_kb() {
    path="$(closest_existing_path "$1")"
    df -k "$path" 2>/dev/null | awk 'NR==2 {print $4+0}'
}

read_first_line() {
    file="$1"
    [ -f "$file" ] || return 1
    IFS= read -r line < "$file" || return 1
    [ -n "$line" ] || return 1
    printf "%s" "$line"
}

pause() {
    if [ "$COMMAND_MODE" = "1" ]; then
        return 0
    fi
    printf "\nPress Enter to continue..."
    read dummy
}

canonical_path() {
    path="$1"
    readlink -f "$path" 2>/dev/null || printf "%s" "$path"
}

current_script_path() {
    if [ -n "${0:-}" ]; then
        canonical_path "$0"
        return 0
    fi
    return 1
}

password_display() {
    if [ -n "$SOCKS_PASSWORD" ]; then
        printf "<set>"
    else
        printf "<empty>"
    fi
}