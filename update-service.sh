#!/bin/sh
set -eu
systemctl stop shift-compacto.service
install -m 0755 -o root -g root /home/rcaliman/Projetos/shift-compacto/shift-compacto /usr/bin/shift-compacto
restorecon /usr/bin/shift-compacto
systemctl start shift-compacto.service
