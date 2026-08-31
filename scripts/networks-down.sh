#!/usr/bin/env bash
#
# Derruba as duas redes Fabric.
#
# Uso:
#   ./scripts/networks-down.sh            # para os containers (estado preservado)
#   ./scripts/networks-down.sh --clean    # remove volumes, material de MSP e chaincode
#
# Entre execuções de experimento queremos o --clean, para partir de estado
# limpo; durante o desenvolvimento, o 'stop' simples é mais rápido.
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

PROFILE="${PROFILE:-1-node}"
clean=0
[[ "${1:-}" == "--clean" ]] && clean=1

need_cacti

if [[ "$clean" -eq 1 ]]; then
  section "Removendo redes e estado"
  warn_msg "isso apaga volumes, material de MSP e chaincode empacotado"
  ( cd "$WEAVER_NET_DIR" && make remove PROFILE="$PROFILE" ) || \
    warn_msg "make remove retornou erro — seguindo"
  ok "redes removidas"
else
  section "Parando redes"
  ( cd "$WEAVER_NET_DIR" && make stop PROFILE="$PROFILE" ) || \
    warn_msg "make stop retornou erro — seguindo"
  ok "redes paradas"
fi

remaining=$(docker ps -q | wc -l | tr -d ' ')
info "containers ainda em execução: $remaining"
if [[ "$remaining" -gt 0 ]]; then
  docker ps --format '  · {{.Names}}'
fi
