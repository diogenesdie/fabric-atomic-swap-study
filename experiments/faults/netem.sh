#!/usr/bin/env bash
#
# Injeta atraso de rede nos containers do Fabric, via tc/netem.
#
# As imagens do Fabric não trazem o iproute2, então não dá para rodar tc dentro
# delas. Usamos um container auxiliar no MESMO namespace de rede do alvo
# (--net=container:<alvo> --cap-add=NET_ADMIN): o tc executado ali altera a
# interface do peer, sem precisar modificar a imagem oficial.
#
# No macOS o tc não existe no host — mas roda normalmente dentro dos containers
# Linux, que é onde ele precisa estar.
#
# Uso:
#   netem.sh apply <container> <atraso> [jitter]   # netem.sh apply peer0... 300ms 50ms
#   netem.sh clear <container>
#   netem.sh clear-all
#   netem.sh status [container]
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../scripts/lib.sh"

NETEM_IMAGE="${NETEM_IMAGE:-htlc2pc/netem:1}"
IFACE="${IFACE:-eth0}"

# Constrói a imagem auxiliar na primeira vez; depois vem do cache.
ensure_image() {
  if docker image inspect "$NETEM_IMAGE" >/dev/null 2>&1; then
    return
  fi
  step "construindo a imagem auxiliar $NETEM_IMAGE (só na primeira vez)"
  docker build -q -t "$NETEM_IMAGE" \
    -f "$(dirname "${BASH_SOURCE[0]}")/Dockerfile.netem" \
    "$(dirname "${BASH_SOURCE[0]}")" >/dev/null
  ok "imagem pronta"
}

# tc_on <container> <argumentos do tc...>
tc_on() {
  local target=$1; shift
  docker run --rm --net="container:$target" --cap-add=NET_ADMIN \
    "$NETEM_IMAGE" "$@"
}

running() {
  docker ps --format '{{.Names}}' | grep -qx "$1"
}

apply() {
  local target=$1 delay=$2 jitter=${3:-}
  running "$target" || die "container não está em execução: $target"
  ensure_image

  # Remove qualquer disciplina anterior para que aplicar duas vezes não empilhe
  # atrasos — erro fácil de cometer num loop de experimento.
  tc_on "$target" qdisc del dev "$IFACE" root >/dev/null 2>&1 || true

  if [[ -n "$jitter" ]]; then
    tc_on "$target" qdisc add dev "$IFACE" root netem delay "$delay" "$jitter" distribution normal
    ok "$target: atraso $delay ± $jitter"
  else
    tc_on "$target" qdisc add dev "$IFACE" root netem delay "$delay"
    ok "$target: atraso $delay"
  fi
}

clear_one() {
  local target=$1
  running "$target" || return 0
  ensure_image
  if tc_on "$target" qdisc del dev "$IFACE" root >/dev/null 2>&1; then
    ok "$target: atraso removido"
  else
    info "$target: nenhum atraso aplicado"
  fi
}

clear_all() {
  local n=0
  for c in $(docker ps --format '{{.Names}}' | grep -E '^(peer0|orderer)\.' || true); do
    clear_one "$c"
    n=$((n + 1))
  done
  [[ "$n" -gt 0 ]] || info "nenhum container de rede em execução"
}

status() {
  ensure_image
  local targets
  if [[ $# -gt 0 ]]; then
    targets="$1"
  else
    targets=$(docker ps --format '{{.Names}}' | grep -E '^(peer0|orderer)\.' || true)
  fi
  for c in $targets; do
    local q
    q=$(tc_on "$c" qdisc show dev "$IFACE" 2>/dev/null | head -1 || echo "?")
    if grep -q netem <<<"$q"; then
      printf '  %s%s%s  %s\n' "$_C_YELLOW" "$c" "$_C_RESET" "$q"
    else
      printf '  %s  (sem atraso)\n' "$c"
    fi
  done
}

case "${1:-}" in
  apply)
    [[ $# -ge 3 ]] || die "uso: netem.sh apply <container> <atraso> [jitter]"
    apply "$2" "$3" "${4:-}"
    ;;
  clear)
    [[ $# -ge 2 ]] || die "uso: netem.sh clear <container>"
    clear_one "$2"
    ;;
  clear-all) clear_all ;;
  status)    shift; status "$@" ;;
  *)
    die "uso: netem.sh {apply|clear|clear-all|status}"
    ;;
esac
