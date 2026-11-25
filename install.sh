#!/usr/bin/env bash
# Установка и обновление плагина опросов на сервере Mattermost.
#
# Запускать на сервере Mattermost, рядом с mmctl (или указать MMCTL=/path/to/mmctl).
# По умолчанию ничего не меняет — только показывает команды:
#   ./install.sh
# Применить:
#   DRY_RUN=0 ./install.sh
#
# Если mmctl бьётся с сокетом, нужен удалённый режим:
#   DRY_RUN=0 MMCTL_SERVER=https://chat.example.ru ./install.sh
# (предварительно: mmctl auth login https://chat.example.ru)

set -euo pipefail

PLUGIN_ID="com.chebcheg.plugin-polls"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARCHIVE="${ARCHIVE:-$HERE/dist/com.chebcheg.plugin-polls-2.1.3.tar.gz}"
MMCTL="${MMCTL:-mmctl}"
DRY_RUN="${DRY_RUN:-1}"

log()  { printf '%s\n' "$*"; }
fail() { printf 'ОШИБКА: %s\n' "$*" >&2; exit 1; }

run() {
    if [ "$DRY_RUN" = "1" ]; then
        printf '[dry-run] %s\n' "$*"
    else
        printf '+ %s\n' "$*"
        "$@"
    fi
}

# --- проверки до любых изменений ---------------------------------------------

[ -f "$ARCHIVE" ] || fail "нет архива плагина: $ARCHIVE"
if [ "$DRY_RUN" != "1" ]; then
    command -v "$MMCTL" >/dev/null 2>&1 || fail "не найден $MMCTL (задайте MMCTL=/path/to/mmctl)"
fi

log "Архив: $ARCHIVE ($(du -h "$ARCHIVE" | cut -f1))"
log "Плагин: $PLUGIN_ID"
log "Режим: $([ "$DRY_RUN" = "1" ] && echo 'проверка, без изменений' || echo 'ПРИМЕНЕНИЕ')"

MMCTL_ARGS=(--local)
if [ -n "${MMCTL_SERVER:-}" ]; then
    MMCTL_ARGS=()
fi

# --- установка ---------------------------------------------------------------

# --force обязателен: без него обновление поверх установленной версии молча
# не произойдёт (mmctl ответит «плагин с таким ID уже установлен»).
run "$MMCTL" "${MMCTL_ARGS[@]}" plugin add --force "$ARCHIVE"
run "$MMCTL" "${MMCTL_ARGS[@]}" plugin enable "$PLUGIN_ID"

# --- проверка ----------------------------------------------------------------

if [ "$DRY_RUN" != "1" ]; then
    log
    log "Проверка:"
    if "$MMCTL" "${MMCTL_ARGS[@]}" plugin list 2>/dev/null | grep -q "$PLUGIN_ID"; then
        "$MMCTL" "${MMCTL_ARGS[@]}" plugin list | grep "$PLUGIN_ID"
        log
        log "Готово. В канале введите /poll — откроется окно создания опроса."
        log "Если кнопки не реагируют, проверьте SiteURL в System Console: он должен совпадать с адресом, по которому открыт Mattermost."
    else
        fail "плагин не появился в списке — смотрите System Console → Logs"
    fi
fi
