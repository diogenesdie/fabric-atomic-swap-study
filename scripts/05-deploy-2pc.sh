#!/usr/bin/env bash
#
# Instala o chaincode twopc nas duas redes, com o ciclo de vida completo do
# Fabric 2.x (package -> install -> approve -> commit -> init).
#
# O testbed do Weaver espera o código em shared/chaincode/<nome>; copiamos do
# nosso repositório para lá em vez de plantar o chaincode dentro do clone
# pinado, que deve permanecer intocado.
#
# Uso:
#   ./scripts/05-deploy-2pc.sh              # nas duas redes
#   ./scripts/05-deploy-2pc.sh --network1   # só na rede 1
#   ./scripts/05-deploy-2pc.sh --version=2  # nova sequência (upgrade)
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

CC_NAME="${CC_NAME:-twopc}"
PROFILE="${PROFILE:-1-node}"
VERSION="${CC_VERSION:-1}"

SRC="$REPO_ROOT/chaincode/$CC_NAME"
DEST="$CACTI_DIR/weaver/tests/network-setups/fabric/shared/chaincode/$CC_NAME"

only=""
for arg in "$@"; do
  case "$arg" in
    --network1) only=network1 ;;
    --network2) only=network2 ;;
    --version=*) VERSION="${arg#*=}" ;;
    *) die "argumento desconhecido: $arg" ;;
  esac
done

need_docker
need_cacti
[[ -d "$SRC" ]] || die "chaincode não encontrado em $SRC"

section "Chaincode $CC_NAME"

# ------------------------------------------------------------------------
# Correção no deployCC.sh do testbed.
#
# A linha que extrai o package-id dos pacotes instalados é:
#
#     sed -n "/$CC_CHAIN_CODE_${VERSION}/{...}"
#
# Em bash, `$CC_CHAIN_CODE_` é lido como a variável CC_CHAIN_CODE_ — com
# underscore no fim, que não existe. O padrão degenera para /<versão>/ e casa
# com qualquer linha contendo aquele dígito. Com um só pacote instalado
# funciona por acidente; a partir do segundo redeploy ele aprova o package-id
# ERRADO, e o peer segue executando o binário antigo enquanto o canal exibe a
# nova sequência — uma falha silenciosa, que só aparece como comportamento
# velho em código novo.
#
# Idempotente: só aplica se o defeito ainda estiver lá.
patch_deploy_script() {
  local f="$WEAVER_NET_DIR/scripts/deployCC.sh"
  [[ -f "$f" ]] || die "script de deploy do testbed não encontrado: $f"

  if ! grep -q 'CC_CHAIN_CODE_\${VERSION}' "$f"; then
    ok "deployCC.sh já corrigido"
    return
  fi
  [[ -f "$f.orig" ]] || cp "$f" "$f.orig"
  # Aspas simples no sed para não expandir as variáveis aqui.
  sed -i.bak 's/\$CC_CHAIN_CODE_\${VERSION}/${CC_CHAIN_CODE}_${VERSION}/' "$f"
  rm -f "$f.bak"
  ok "deployCC.sh corrigido: extração do package-id (original em deployCC.sh.orig)"
}

patch_deploy_script

# Testes antes do deploy: um chaincode com máquina de estados quebrada
# desperdiça vários minutos de ciclo de vida para falhar no fim.
step "testes unitários"
if ( cd "$SRC" && go test -v ./... >/tmp/twopc-test.log 2>&1 ); then
  ok "$(grep -c '^--- PASS' /tmp/twopc-test.log) testes passando"
else
  err "testes falharam — deploy interrompido"
  tail -20 /tmp/twopc-test.log | sed 's/^/      /'
  exit 1
fi

step "copiando o código para o testbed"
mkdir -p "$(dirname "$DEST")"
rm -rf "$DEST"
mkdir -p "$DEST"
# Sem os arquivos de teste: eles arrastam dependências (mocks do Weaver) que o
# builder do peer não precisa baixar para compilar o chaincode.
( cd "$SRC" && tar cf - --exclude='*_test.go' --exclude=vendor . ) | ( cd "$DEST" && tar xf - )
ok "$(find "$DEST" -name '*.go' | wc -l | tr -d ' ') arquivos Go em shared/chaincode/$CC_NAME"

deploy_to() {
  local nw=$1
  step "deploy em $nw (sequência $VERSION)"
  # O deployCC do testbed encadeia package, install, approveformyorg,
  # checkcommitreadiness, commit e a invocação de init (--isInit).
  # -v é ao mesmo tempo a versão e a SEQUÊNCIA do chaincode. Cada redeploy
  # precisa de sequência maior que a efetivada, senão o peer recusa com
  # "requested sequence is N, but new definition must be sequence N+1".
  if ( cd "$WEAVER_NET_DIR" && \
       ./network.sh deployCC -ch "$CC_NAME" -nw "$nw" -p "$PROFILE" -v "$VERSION" \
         >"/tmp/twopc-deploy-$nw.log" 2>&1 ); then
    ok "$nw: chaincode efetivado no canal"
  else
    err "$nw: deploy falhou"
    grep -iE "error|failed|panic" "/tmp/twopc-deploy-$nw.log" | tail -8 | sed 's/^/      /'
    return 1
  fi
}

case "$only" in
  network1) deploy_to network1 ;;
  network2) deploy_to network2 ;;
  *)        deploy_to network1 && deploy_to network2 ;;
esac

section "Verificação"

for nw in network1 network2; do
  [[ -n "$only" && "$only" != "$nw" ]] && continue
  n=$(docker ps --format '{{.Names}}' | grep -c "dev-peer0.org1.$nw.com-$CC_NAME" || true)
  if [[ "$n" -ge 1 ]]; then
    ok "$nw: container do chaincode em execução"
  else
    warn_msg "$nw: container do chaincode ainda não apareceu (sobe na primeira invocação)"
  fi
done

echo
info "Próximo passo: go run ./apps/coordinator -scenario=T1"
