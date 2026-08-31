#!/usr/bin/env bash
#
# Prepara o braço HTLC: compila a CLI do Weaver, corrige os connection profiles
# gerados pelo testbed, registra os usuários e popula os ativos.
#
# Precisa rodar depois de cada recriação das redes, porque os connection
# profiles e as wallets são regerados.
#
# Uso: ./scripts/03-setup-htlc.sh
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

GO_CLI="$CACTI_DIR/weaver/samples/fabric/go-cli"
SHARED="$CACTI_DIR/weaver/tests/network-setups/fabric/shared"
FIXTURES="$REPO_ROOT/experiments/fixtures"
USERS=(alice bob)

need_docker
need_cacti
[[ -d "$GO_CLI" ]] || die "go-cli não encontrado em $GO_CLI"

# ------------------------------------------------------------------ perfis

section "Connection profiles"

# O testbed gera perfis YAML com três defeitos; ver o cabeçalho de
# fix_connection_profiles.py para o diagnóstico de cada um.
profiles=()
for nw in network1 network2; do
  p="$SHARED/$nw/peerOrganizations/org1.$nw.com/connection-org1.yaml"
  [[ -f "$p" ]] || die "perfil não encontrado: $p (as redes estão no ar?)"
  profiles+=("$p")
done

python3 "$REPO_ROOT/scripts/fix_connection_profiles.py" "${profiles[@]}" \
  || die "falha ao corrigir os connection profiles"

# ------------------------------------------------------------------ CLI

section "CLI do Weaver (go-cli)"

# config.json aponta a CLI para os perfis das duas redes.
cat > "$GO_CLI/config.json" <<EOF
{
  "network1": {
    "connProfilePath": "$SHARED/network1/peerOrganizations/org1.network1.com/connection-org1.yaml",
    "relayEndpoint": "localhost:9080",
    "mspId": "Org1MSP",
    "channelName": "mychannel",
    "chaincode": "simpleasset"
  },
  "network2": {
    "connProfilePath": "$SHARED/network2/peerOrganizations/org1.network2.com/connection-org1.yaml",
    "relayEndpoint": "localhost:9083",
    "mspId": "Org1MSP",
    "channelName": "mychannel",
    "chaincode": "simpleasset"
  }
}
EOF
ok "config.json escrito"

cat > "$GO_CLI/.env" <<EOF
DEFAULT_CHANNEL=mychannel
DEFAULT_CHAINCODE=interop
MEMBER_CREDENTIAL_FOLDER=$GO_CLI/data/credentials
LOCAL=true
DEFAULT_APPLICATION_CHAINCODE=simpleasset
CONFIG_PATH=$GO_CLI/config.json
EOF
ok ".env escrito"

if [[ ! -x "$GO_CLI/bin/fabric-cli" ]]; then
  step "compilando a CLI"
  ( cd "$GO_CLI" && make build >/dev/null 2>&1 ) || die "falha ao compilar a go-cli"
  ok "compilada"
else
  ok "binário já existe (remova $GO_CLI/bin para recompilar)"
fi

cli() { ( cd "$GO_CLI" && ./bin/fabric-cli "$@" ); }

# ------------------------------------------------------------------ usuários

section "Usuários"

for nw in network1 network2; do
  for u in "${USERS[@]}"; do
    if [[ -f "$GO_CLI/wallets/$nw/$u.id" ]]; then
      ok "$nw/$u já na wallet"
      continue
    fi
    if cli user add --target-network="$nw" --id="$u" --secret="${u}pw" \
         >"/tmp/htlc-user-$nw-$u.log" 2>&1; then
      ok "$nw/$u registrado"
    else
      err "falha ao registrar $nw/$u:"
      grep -oE 'msg="[^"]{0,180}' "/tmp/htlc-user-$nw-$u.log" | tail -1 | sed 's/^/      /'
      exit 1
    fi
  done
done

# ------------------------------------------------------------------ ativos

section "Ativos"

# Os dados de exemplo do Weaver têm vencimento em 2022 e o chaincode recusa
# ("maturity date can not be in past"). Geramos os nossos com data futura.
mkdir -p "$FIXTURES"
if [[ ! -f "$FIXTURES/assets.json" ]]; then
  die "fixture ausente: $FIXTURES/assets.json"
fi

# Recusa fixtures que envelheceram, em vez de falhar silenciosamente adiante.
python3 - "$FIXTURES/assets.json" <<'PY' || exit 1
import datetime, json, sys
data = json.load(open(sys.argv[1]))
today = datetime.date.today()
for key, asset in data.items():
    raw = asset.get("maturitydate", "")
    try:
        when = datetime.datetime.strptime(raw.split(" 00:00")[0], "%d %b %y").date()
    except ValueError:
        print(f"  ! não foi possível interpretar a data de {key}: {raw!r}")
        continue
    if when <= today:
        print(f"  ✗ {key}: vencimento {raw} está no passado — o chaincode vai recusar.")
        print("    Regere o fixture com data futura (ver docs/notas/fabric.md).")
        sys.exit(1)
print("  ✓ datas de vencimento no futuro")
PY

# ATENÇÃO: 'configure asset add' NÃO é idempotente para tokens — cada execução
# emite novas unidades. Só rode em estado limpo, ou os saldos acumulam.
add_assets() {
  local nw=$1 type=$2 file=$3 log=/tmp/htlc-assets-$nw.log
  cli configure asset add --target-network="$nw" --type="$type" \
      --data-file="$file" >"$log" 2>&1 || true
  # A CLI sai com 0 mesmo quando a invocação falha; é preciso olhar o log.
  if grep -q 'Invoke error' "$log"; then
    err "falha ao popular $type em $nw:"
    grep -oE 'Description: [^\\]{0,140}' "$log" | sort -u | head -3 | sed 's/^/      /'
    return 1
  fi
  ok "$nw: $type populado"
}

add_assets network1 bond  "$FIXTURES/assets.json"  || exit 1
add_assets network2 token "$FIXTURES/tokens.json"  || exit 1

echo
info "Próximo passo: ./scripts/04-spike-htlc.sh (ou 'make spike')"
