#!/usr/bin/env bash
#
# Sobe as duas redes Fabric do testbed do Weaver, com o interop chaincode e o
# chaincode de aplicação usado no swap HTLC.
#
# Sobe a rede 1, mede o consumo de memória, e só então sobe a rede 2 — a
# máquina de referência tem 7,7 GB alocados ao Docker, abaixo do folgado.
#
# Uso:
#   ./scripts/02-networks-up.sh              # ambas as redes
#   ./scripts/02-networks-up.sh --network1   # só a rede 1
#   ./scripts/02-networks-up.sh --network2   # só a rede 2
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

CHAINCODE_NAME="${CHAINCODE_NAME:-simpleasset}"
PROFILE="${PROFILE:-1-node}"

only=""
case "${1:-}" in
  --network1) only=network1 ;;
  --network2) only=network2 ;;
  "")         only="" ;;
  *)          die "argumento desconhecido: $1 (use --network1 ou --network2)" ;;
esac

need_docker
need_cacti

section "Configuração"
info "chaincode de aplicação: $CHAINCODE_NAME"
info "perfil do testbed:      $PROFILE (1 peer + 1 CouchDB por rede)"
info "testbed:                $WEAVER_NET_DIR"

# O primeiro 'make' baixa install-fabric.sh, as imagens do Fabric 2.5.16 e os
# binários (configtxgen, peer). Pode levar vários minutos na primeira vez.
if [[ ! -f "$WEAVER_NET_DIR/.fabric-setup" ]]; then
  warn_msg "primeira execução: vai baixar imagens e binários do Fabric (~5 GB)"
fi

bring_up() {
  local nw=$1
  step "subindo $nw (canal + interop chaincode + $CHAINCODE_NAME)"

  # start-interop-networkN encadeia: setup-interop-cc, start-networkN
  # (network.sh up createChannel + deployCC do app) e deployCC do interop.
  ( cd "$WEAVER_NET_DIR" && \
    make "start-interop-$nw" CHAINCODE_NAME="$CHAINCODE_NAME" PROFILE="$PROFILE" )

  ok "$nw no ar"
}

report_state() {
  local label=$1
  section "Estado após $label"

  local n mem
  n=$(docker ps -q | wc -l | tr -d ' ')
  mem=$(docker_mem_used_mib)
  info "containers em execução: $n"
  info "memória usada pelos containers: ${mem} MiB"

  local total_mib
  total_mib=$(docker info --format '{{.MemTotal}}' | awk '{printf "%.0f", $1/1024/1024}')
  local pct
  pct=$(awk -v u="$mem" -v t="$total_mib" 'BEGIN{printf "%.0f", (u/t)*100}')
  info "uso do limite do Docker: ${pct}% de ${total_mib} MiB"

  if [[ "$pct" -ge 75 ]]; then
    warn_msg "consumo alto — a segunda rede pode não caber"
  else
    ok "folga suficiente"
  fi

  docker ps --format '  · {{.Names}}' | sort
}

case "$only" in
  network1)
    bring_up network1
    report_state "network1"
    ;;
  network2)
    bring_up network2
    report_state "network2"
    ;;
  *)
    bring_up network1
    report_state "network1"
    bring_up network2
    report_state "as duas redes"
    ;;
esac

section "Verificação de sanidade"

# Cada rede deve ter orderer, peer, couchdb e CAs próprios
for nw in network1 network2; do
  [[ -n "$only" && "$only" != "$nw" ]] && continue
  n=$(docker ps --format '{{.Names}}' | grep -c "$nw" || true)
  if [[ "$n" -ge 4 ]]; then
    ok "$nw: $n containers"
  else
    err "$nw: apenas $n containers — algo falhou"
  fi
done

echo
info "Próximo passo: ./scripts/04-spike-htlc.sh (ou 'make spike')"
info "Para derrubar: ./scripts/networks-down.sh"
