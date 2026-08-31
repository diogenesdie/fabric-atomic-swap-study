#!/usr/bin/env bash
#
# Funções compartilhadas pelos scripts: saída padronizada, resolução de
# caminhos e utilidades de versão.
#
# Uso: source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Raiz do repositório, independente de onde o script foi chamado
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export REPO_ROOT

# Clone do monorepo Cacti/Weaver
CACTI_DIR="${CACTI_DIR:-$REPO_ROOT/cacti}"
export CACTI_DIR

# Testbed de duas redes Fabric dentro do monorepo
WEAVER_NET_DIR="$CACTI_DIR/weaver/tests/network-setups/fabric/dev"
export WEAVER_NET_DIR

# ------------------------------------------------------------------ saída

if [[ -t 1 ]]; then
  _C_RESET=$'\033[0m'; _C_BOLD=$'\033[1m'; _C_DIM=$'\033[2m'
  _C_RED=$'\033[31m'; _C_GREEN=$'\033[32m'; _C_YELLOW=$'\033[33m'
  _C_BLUE=$'\033[34m'
else
  _C_RESET=''; _C_BOLD=''; _C_DIM=''
  _C_RED=''; _C_GREEN=''; _C_YELLOW=''; _C_BLUE=''
fi

section() { printf '\n%s== %s ==%s\n' "$_C_BOLD$_C_BLUE" "$1" "$_C_RESET"; }
ok()      { printf '  %s✓%s %s\n' "$_C_GREEN" "$_C_RESET" "$1"; }
err()     { printf '  %s✗%s %s\n' "$_C_RED" "$_C_RESET" "$1" >&2; }
warn_msg(){ printf '  %s!%s %s\n' "$_C_YELLOW" "$_C_RESET" "$1"; }
info()    { printf '  %s·%s %s\n' "$_C_DIM" "$_C_RESET" "$1"; }
step()    { printf '\n%s-->%s %s\n' "$_C_BOLD" "$_C_RESET" "$1"; }

die() { err "$1"; exit "${2:-1}"; }

# ------------------------------------------------------------------ versões

# version_ge <a> <b> — verdadeiro se a >= b (comparação semântica)
version_ge() {
  printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

# ------------------------------------------------------------------ checagens

need_cacti() {
  [[ -d "$WEAVER_NET_DIR" ]] || die \
    "testbed do Weaver não encontrado em $WEAVER_NET_DIR
     Rode primeiro: ./scripts/01-clone-weaver.sh"
}

need_docker() {
  docker info >/dev/null 2>&1 || die \
    "o daemon do Docker não está respondendo — inicie o Docker Desktop"
}

# Conta containers de uma rede do testbed (net1_/net2_ ou nomes com sufixo)
count_network_containers() {
  local pattern=$1
  docker ps --format '{{.Names}}' | grep -cE "$pattern" || true
}

# Memória total usada pelos containers em execução, em MiB
docker_mem_used_mib() {
  docker stats --no-stream --format '{{.MemUsage}}' 2>/dev/null \
    | awk -F'/' '{print $1}' \
    | awk '
        /GiB/ { gsub(/GiB/,""); total += $1 * 1024; next }
        /MiB/ { gsub(/MiB/,""); total += $1;        next }
        /KiB/ { gsub(/KiB/,""); total += $1 / 1024; next }
        END   { printf "%.0f", total }'
}
