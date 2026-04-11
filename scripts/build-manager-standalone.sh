#!/bin/sh

set -eu

out_path="${1:-build/tg-ws-proxy-go.sh}"

mkdir -p "$(dirname "$out_path")"

{
    printf '%s\n' '#!/bin/sh'
    printf '\n'
    printf '%s\n' 'set -u'
    printf '\n'

    for file in \
        lib/colors.sh \
        lib/globals.sh \
        lib/utils.sh \
        lib/platform.sh \
        lib/config.sh \
        lib/release.sh \
        lib/process.sh \
        lib/autostart.sh \
        lib/install.sh \
        lib/ui.sh \
        lib/menu.sh
    do
        awk 'NR == 1 && $0 ~ /^# / { next } { print }' "$file"
        printf '\n'
    done

    awk '
        found { print }
        /^load_saved_settings$/ {
            found = 1
            print
        }
    ' tg-ws-proxy-go.sh
} > "$out_path"

chmod +x "$out_path"
