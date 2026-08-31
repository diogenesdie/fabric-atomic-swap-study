#!/usr/bin/env bash
#
# Falha de nó: pausa ou derruba um container do Fabric.
#
# Duas modalidades, que modelam coisas diferentes:
#
#   pause  — SIGSTOP nos processos. O container continua existindo e com as
#            conexões TCP abertas, mas não responde. Modela um nó travado ou
#            sob suspensão: o cliente fica esperando até estourar o timeout.
#   stop   — SIGTERM e depois SIGKILL. O container morre e as conexões caem na
#            hora. Modela crash limpo (fail-stop), em que o cliente descobre a
#            falha imediatamente.
#
# A distinção importa para o experimento: um nó pausado é indistinguível de um
# nó lento, que é exatamente a ambiguidade em que os protocolos de commit
# tropeçam.
#
# Uso:
#   crash-peer.sh pause <container>
#   crash-peer.sh resume <container>
#   crash-peer.sh stop <container>
#   crash-peer.sh start <container>
#   crash-peer.sh restore-all
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../../scripts/lib.sh"

exists() {
  docker ps -a --format '{{.Names}}' | grep -qx "$1"
}

state_of() {
  docker inspect -f '{{.State.Status}}' "$1" 2>/dev/null || echo "ausente"
}

case "${1:-}" in
  pause)
    [[ $# -ge 2 ]] || die "uso: crash-peer.sh pause <container>"
    exists "$2" || die "container não encontrado: $2"
    docker pause "$2" >/dev/null
    ok "$2: pausado (não responde, conexões preservadas)"
    ;;

  resume)
    [[ $# -ge 2 ]] || die "uso: crash-peer.sh resume <container>"
    if [[ "$(state_of "$2")" == "paused" ]]; then
      docker unpause "$2" >/dev/null
      ok "$2: retomado"
    else
      info "$2: não estava pausado (estado: $(state_of "$2"))"
    fi
    ;;

  stop)
    [[ $# -ge 2 ]] || die "uso: crash-peer.sh stop <container>"
    exists "$2" || die "container não encontrado: $2"
    # -t 0 vai direto ao SIGKILL: queremos crash, não encerramento gracioso.
    docker stop -t 0 "$2" >/dev/null
    ok "$2: derrubado"
    ;;

  start)
    [[ $# -ge 2 ]] || die "uso: crash-peer.sh start <container>"
    exists "$2" || die "container não encontrado: $2"
    docker start "$2" >/dev/null
    ok "$2: no ar de novo"
    ;;

  restore-all)
    # Devolve tudo ao normal entre execuções: um container esquecido pausado
    # contaminaria todos os resultados seguintes.
    n=0
    for c in $(docker ps -a --format '{{.Names}}' | grep -E '^(peer0|orderer|ca)\.' || true); do
      case "$(state_of "$c")" in
        paused)  docker unpause "$c" >/dev/null; ok "$c: retomado"; n=$((n+1)) ;;
        exited)  docker start "$c" >/dev/null;   ok "$c: reiniciado"; n=$((n+1)) ;;
      esac
    done
    [[ "$n" -gt 0 ]] || ok "nada a restaurar"
    ;;

  status)
    for c in $(docker ps -a --format '{{.Names}}' | grep -E '^(peer0|orderer|ca)\.' | sort); do
      printf '  %-32s %s\n' "$c" "$(state_of "$c")"
    done
    ;;

  *)
    die "uso: crash-peer.sh {pause|resume|stop|start|restore-all|status}"
    ;;
esac
