#!/usr/bin/env bash
#
# Verifica os pré-requisitos antes de subir as redes Fabric.
# Falha (exit 1) no que é impeditivo; apenas avisa no que é ajustável.
#
set -euo pipefail

# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

fail=0
warn=0

section "Sistema"

info "SO/arquitetura: $(uname -sm)"
case "$(uname -m)" in
  arm64|aarch64)
    ok "arquitetura arm64 — o Fabric 2.5.16 publica imagens nativas"
    ;;
  x86_64)
    ok "arquitetura x86_64"
    ;;
  *)
    warn_msg "arquitetura $(uname -m) não testada"
    warn=$((warn + 1))
    ;;
esac

section "Ferramentas"

require_cmd() {
  local cmd=$1 label=${2:-$1}
  if command -v "$cmd" >/dev/null 2>&1; then
    ok "$label: $(command -v "$cmd")"
  else
    err "$label não encontrado — é obrigatório"
    fail=$((fail + 1))
  fi
}

require_cmd docker
require_cmd git
require_cmd make
require_cmd jq
require_cmd curl

# Go: o chaincode e os clientes são em Go
if command -v go >/dev/null 2>&1; then
  go_ver=$(go version | awk '{print $3}' | sed 's/^go//')
  if version_ge "$go_ver" "1.21"; then
    ok "go $go_ver (>= 1.21)"
  else
    err "go $go_ver é antigo — precisamos de 1.21 ou superior"
    fail=$((fail + 1))
  fi
else
  err "go não encontrado — é obrigatório"
  fail=$((fail + 1))
fi

section "Docker"

if ! docker info >/dev/null 2>&1; then
  err "o daemon do Docker não está respondendo — inicie o Docker Desktop"
  fail=$((fail + 1))
else
  ok "daemon respondendo (server $(docker version --format '{{.Server.Version}}'))"

  if docker compose version >/dev/null 2>&1; then
    ok "compose v2: $(docker compose version --short)"
  else
    err "docker compose v2 não disponível (o plugin 'compose' é obrigatório)"
    fail=$((fail + 1))
  fi

  # Memória disponível ao daemon — o ponto crítico para duas redes
  mem_bytes=$(docker info --format '{{.MemTotal}}' 2>/dev/null || echo 0)
  mem_gb=$(awk -v b="$mem_bytes" 'BEGIN{printf "%.1f", b/1024/1024/1024}')
  info "memória alocada ao Docker: ${mem_gb} GB"

  if awk -v m="$mem_gb" 'BEGIN{exit !(m >= 12)}'; then
    ok "folga confortável para as duas redes"
  elif awk -v m="$mem_gb" 'BEGIN{exit !(m >= 7)}'; then
    warn_msg "abaixo dos 12 GB recomendados para duas redes simultâneas."
    echo "             Estratégia: subir a rede 1, medir o consumo real com"
    echo "             'docker stats', e só então subir a rede 2. Se não couber,"
    echo "             aumente a memória em Docker Desktop > Settings > Resources,"
    echo "             ou caia para o plano B (dois canais numa rede)."
    warn=$((warn + 1))
  else
    err "menos de 7 GB no Docker — insuficiente até para uma rede Fabric"
    fail=$((fail + 1))
  fi

  info "CPUs disponíveis ao Docker: $(docker info --format '{{.NCPU}}')"

  running=$(docker ps -q | wc -l | tr -d ' ')
  if [[ "$running" -gt 0 ]]; then
    warn_msg "$running container(es) já em execução — podem competir por memória"
    warn=$((warn + 1))
  else
    ok "nenhum container em execução"
  fi

  # Espaço em disco: as imagens do Fabric mais os volumes pesam alguns GB
  avail_gb=$(df -g . 2>/dev/null | awk 'NR==2{print $4}')
  if [[ -n "${avail_gb:-}" ]]; then
    if [[ "$avail_gb" -ge 20 ]]; then
      ok "espaço livre em disco: ${avail_gb} GB"
    else
      warn_msg "apenas ${avail_gb} GB livres — as imagens do Fabric pesam ~5 GB"
      warn=$((warn + 1))
    fi
  fi
fi

section "Resultado"

if [[ "$fail" -gt 0 ]]; then
  err "$fail requisito(s) impeditivo(s). Resolva antes de continuar."
  exit 1
fi

if [[ "$warn" -gt 0 ]]; then
  warn_msg "$warn aviso(s) — é possível seguir, com atenção ao indicado acima."
else
  ok "ambiente pronto."
fi

echo
info "Próximo passo: ./scripts/01-clone-weaver.sh (ou 'make setup')"
