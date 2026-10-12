#!/bin/sh
set -eu

project=/home/rcaliman/Projetos/shift-compacto
if [ "$(id -u)" -ne 0 ]; then
    echo 'Execute este instalador com sudo ou pkexec.' >&2
    exit 1
fi

if [ -e /usr/bin/shift-compacto ] || [ -e /etc/systemd/system/shift-compacto.service ]; then
    echo 'Já existe uma instalação. Revise os arquivos antes de substituir.' >&2
    exit 1
fi

/usr/bin/install -m 0755 -o root -g root "$project/shift-compacto" /usr/bin/shift-compacto
/usr/bin/install -m 0644 -o root -g root "$project/shift-compacto.service" /etc/systemd/system/shift-compacto.service
if command -v restorecon >/dev/null 2>&1; then
    restorecon /usr/bin/shift-compacto /etc/systemd/system/shift-compacto.service
fi
systemctl daemon-reload
systemctl enable --now shift-compacto.service
systemctl is-enabled shift-compacto.service
systemctl is-active shift-compacto.service
