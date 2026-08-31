#!/usr/bin/env bash
#
# Clona o monorepo Hyperledger Cacti (que incorpora o Weaver) numa versão
# pinada. O clone fica em ./cacti/, fora do versionamento deste repositório.
#
# Por que pinar num commit e não numa tag: o Cacti versiona por componente
# (as tags são do tipo weaver/sdks/fabric/go-sdk/v3.0.1), então não existe tag
# única que represente o estado do monorepo. Pinamos o commit do main.
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

CACTI_REPO="${CACTI_REPO:-https://github.com/hyperledger-cacti/cacti.git}"

# Commit de referência: main em 21.08.2026, mesmo dia do release v3.0.1 dos
# componentes do Weaver (interop chaincode e go-sdk).
CACTI_COMMIT="${CACTI_COMMIT:-2ebe62180e4425dbd028cc2f62c4b4aa44595355}"

section "Clone do Hyperledger Cacti/Weaver"

info "repositório: $CACTI_REPO"
info "commit:      $CACTI_COMMIT"
info "destino:     $CACTI_DIR"

if [[ -d "$CACTI_DIR/.git" ]]; then
  current=$(git -C "$CACTI_DIR" rev-parse HEAD 2>/dev/null || echo "")
  if [[ "$current" == "$CACTI_COMMIT" ]]; then
    ok "já clonado no commit correto — nada a fazer"
    exit 0
  fi
  warn_msg "clone existente está em ${current:0:12}, esperado ${CACTI_COMMIT:0:12}"
  step "buscando o commit pinado"
  git -C "$CACTI_DIR" fetch --depth 1 origin "$CACTI_COMMIT"
  git -C "$CACTI_DIR" checkout --force FETCH_HEAD
  ok "atualizado para ${CACTI_COMMIT:0:12}"
  exit 0
fi

# Clone raso de um commit específico: baixa só a árvore daquele ponto.
# O monorepo é grande; isso reduz o download de ~1 GB para algumas centenas
# de MB. O GitHub permite buscar SHA arbitrário.
step "clonando (raso, apenas o commit pinado)"
mkdir -p "$CACTI_DIR"
git -C "$CACTI_DIR" init -q
git -C "$CACTI_DIR" remote add origin "$CACTI_REPO"

if git -C "$CACTI_DIR" fetch --depth 1 origin "$CACTI_COMMIT" 2>/dev/null; then
  git -C "$CACTI_DIR" checkout -q FETCH_HEAD
  ok "clone raso concluído"
else
  warn_msg "o servidor recusou buscar o SHA direto; caindo para clone completo"
  rm -rf "$CACTI_DIR"
  git clone --filter=blob:none --no-checkout "$CACTI_REPO" "$CACTI_DIR"
  git -C "$CACTI_DIR" checkout -q "$CACTI_COMMIT"
  ok "clone parcial concluído"
fi

section "Verificação"

check_path() {
  if [[ -e "$CACTI_DIR/$1" ]]; then
    ok "$1"
  else
    err "ausente: $1"
    return 1
  fi
}

missing=0
# Testbed das duas redes
check_path "weaver/tests/network-setups/fabric/dev/Makefile" || missing=1
# Biblioteca HTLC
check_path "weaver/core/network/fabric-interop-cc/libs/assetexchange" || missing=1
# Interop chaincode
check_path "weaver/core/network/fabric-interop-cc/contracts/interop" || missing=1
# Chaincode de aplicação usado no swap
check_path "weaver/samples/fabric/simpleasset" || missing=1
# CLI em Go (a que vamos usar, em vez da de TypeScript)
check_path "weaver/samples/fabric/go-cli" || missing=1

[[ "$missing" -eq 0 ]] || die "o clone não tem a estrutura esperada"

section "Versões declaradas pelo testbed"

mk="$CACTI_DIR/weaver/tests/network-setups/fabric/dev/Makefile"
grep -E '^(FABRIC_VERSION|FABRIC_CA_VERSION)' "$mk" | sed 's/^/  · /' || \
  warn_msg "não foi possível ler as versões do Makefile"

info "tamanho do clone: $(du -sh "$CACTI_DIR" 2>/dev/null | cut -f1)"

echo
info "Próximo passo: ./scripts/02-networks-up.sh (ou 'make networks-up')"
