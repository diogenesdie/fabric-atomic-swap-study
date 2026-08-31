#!/usr/bin/env bash
#
# Spike de viabilidade: executa um swap atômico HTLC completo entre as duas
# redes Fabric e verifica o estado final dos dois ledgers.
#
# É o portão da Etapa 1: se isto passa, o resto do trabalho é caminhável.
#
# O swap (Nolan 2013 / Herlihy 2018):
#   1. alice trava bond01:a03 na rede 1 com hash H, prazo T1 (longo)
#   2. bob verifica o lock de alice
#   3. bob trava token1:100 na rede 2 com o mesmo H, prazo T2 (curto)
#   4. alice verifica o lock de bob
#   5. alice resgata os tokens revelando o segredo -> o segredo vira público
#   6. bob lê o segredo do ledger da rede 2 e resgata o bond na rede 1
#
# T1 > T2 é a assimetria que protege quem revela o segredo primeiro.
#
# Uso: ./scripts/04-spike-htlc.sh [--secret=<texto>] [--t1=<seg>] [--t2=<seg>]
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

GO_CLI="$CACTI_DIR/weaver/samples/fabric/go-cli"
RESULTS_DIR="$REPO_ROOT/experiments/results"

SECRET="spike-secret-$$"
T1=600   # prazo de quem inicia (alice) — maior
T2=300   # prazo de quem responde (bob) — menor
BOND_TYPE="bond01"
BOND_ID="a03"
TOKEN_TYPE="token1"
TOKEN_QTY="100"

for arg in "$@"; do
  case "$arg" in
    --secret=*) SECRET="${arg#*=}" ;;
    --t1=*)     T1="${arg#*=}" ;;
    --t2=*)     T2="${arg#*=}" ;;
    *)          die "argumento desconhecido: $arg" ;;
  esac
done

[[ "$T1" -gt "$T2" ]] || die "T1 ($T1) precisa ser maior que T2 ($T2) — ver docs/notas/htlc.md"

need_docker
need_cacti
[[ -x "$GO_CLI/bin/fabric-cli" ]] || die "CLI não compilada — rode ./scripts/03-setup-htlc.sh"

RUN_ID="spike-$(date +%Y%m%d-%H%M%S)"
CSV="$RESULTS_DIR/$RUN_ID.csv"
mkdir -p "$RESULTS_DIR"
echo "run_id,scenario,step,label,network,t_start_ms,duration_ms,result" > "$CSV"

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }
T_ZERO=$(now_ms)

cli() { ( cd "$GO_CLI" && ./bin/fabric-cli "$@" ); }

# Extrai o payload de uma query e desfaz o escape do log.
#
# A CLI imprime `msg="state from network: <payload>"`. Duas armadilhas:
#  - o `"` que fecha o atributo msg= entra na captura e precisa sair, senão o
#    payload JSON fica inválido (`{...}"`);
#  - o payload vem com escapes de log (\" e \n), desfeitos com unicode_escape.
query_state() {
  local nw=$1 user=$2 fn=$3 args=$4
  cli chaincode query --local-network="$nw" --user="$user" \
      mychannel simpleasset "$fn" "$args" 2>&1 \
    | grep -oE 'state from network: .*' \
    | sed -e 's/^state from network: //' -e 's/"$//' \
    | python3 -c "import sys; print(sys.stdin.read().encode().decode('unicode_escape'), end='')"
}

# Descobre o dono do bond.
#
# O simpleasset restringe ReadAsset ao dono ("cannot access Bond Asset ..."),
# então não existe uma identidade neutra para consultar: é preciso tentar cada
# candidata. Quem consegue ler é o dono — e confirmamos pelo certificado no
# campo owner, que é a fonte de verdade.
bond_owner() {
  local candidate json owner
  for candidate in alice bob; do
    json=$(query_state network1 "$candidate" ReadAsset \
             "[\"$BOND_TYPE\",\"$BOND_ID\"]" 2>/dev/null) || true
    [[ -n "$json" ]] || continue
    owner=$(python3 "$REPO_ROOT/scripts/ledger_state.py" owner-of "$json" 2>/dev/null) || true
    if [[ -n "$owner" ]]; then
      echo "$owner"
      return
    fi
  done
  echo "AUSENTE"
}

# GetBalance exige que o dono tenha carteira registrada; GetMyWallet é o
# caminho confiável. O saldo sai como `token1="10000"` — extraímos só o número
# entre aspas, porque um grep ingênuo por dígitos casaria o "1" de "token1".
token_balance() {
  local user=$1
  query_state network2 "$user" GetMyWallet '[]' 2>/dev/null \
    | grep -oE "$TOKEN_TYPE=\"[0-9]+\"" \
    | sed -E 's/.*="([0-9]+)"/\1/' \
    | head -1
}

# Executa um passo do protocolo, cronometra e registra no CSV
run_step() {
  local step=$1 label=$2 nw=$3; shift 3
  local log="/tmp/spike-step$step.log"
  local t0 t1 dur result

  t0=$(now_ms)
  if cli asset exchange-step --step="$step" --target-network="$nw" "$@" \
       >"$log" 2>&1 && grep -q "step $step completed\|all steps completed" "$log"; then
    result=OK
  else
    result=FALHOU
  fi
  t1=$(now_ms)
  dur=$((t1 - t0))

  echo "$RUN_ID,baseline,$step,$label,$nw,$((t0 - T_ZERO)),$dur,$result" >> "$CSV"

  if [[ "$result" == OK ]]; then
    ok "passo $step ($label) em ${dur}ms"
  else
    err "passo $step ($label) falhou:"
    grep -oE 'Description: [^\\]{0,160}|msg="[^"]{0,160}' "$log" | tail -2 | sed 's/^/      /'
    return 1
  fi
}

# ------------------------------------------------------------------ início

section "Spike HTLC — $RUN_ID"
info "segredo:      $SECRET"
info "prazos:       T1=${T1}s (alice, inicia) > T2=${T2}s (bob, responde)"
info "ativos:       $BOND_TYPE:$BOND_ID (rede 1)  <->  $TOKEN_QTY $TOKEN_TYPE (rede 2)"
info "resultados:   $CSV"

section "Estado inicial"
OWNER_BEFORE=$(bond_owner)
ALICE_BEFORE=$(token_balance alice)
BOB_BEFORE=$(token_balance bob)
info "rede 1 — $BOND_TYPE:$BOND_ID dono: $OWNER_BEFORE"
info "rede 2 — token1 alice: ${ALICE_BEFORE:-?}  bob: ${BOB_BEFORE:-?}"

[[ "$OWNER_BEFORE" == "alice" ]] || \
  die "o bond deveria começar com alice, está com '$OWNER_BEFORE'. Rode ./scripts/03-setup-htlc.sh em estado limpo."

section "Execução do protocolo"

run_step 1 lock-bond network1 \
  --secret="$SECRET" --timeout-duration="$T1" \
  --locker=alice --recipient=bob --param="$BOND_TYPE:$BOND_ID"

HASH=$(grep -oE 'hashBase64: [^"]*' /tmp/spike-step1.log | head -1 | sed 's/hashBase64: //')
[[ -n "$HASH" ]] || die "não foi possível extrair o hash do passo 1"
info "hash publicado: $HASH"

run_step 2 verify-bond-lock network1 \
  --locker=alice --recipient=bob --param="$BOND_TYPE:$BOND_ID"

run_step 3 lock-tokens network2 \
  --hash="$HASH" --timeout-duration="$T2" \
  --locker=bob --recipient=alice --param="$TOKEN_TYPE:$TOKEN_QTY"

CID=$(grep -oE 'contractId: [^"]*' /tmp/spike-step3.log | head -1 | sed 's/contractId: //')
[[ -n "$CID" ]] || die "não foi possível extrair o contractId do passo 3"
info "contractId do lock fungível: $CID"

run_step 4 verify-token-lock network2 \
  --locker=bob --recipient=alice --contract-id="$CID"

run_step 5 claim-tokens network2 \
  --secret="$SECRET" --locker=bob --recipient=alice --contract-id="$CID"

# Aqui está a essência do HTLC: bob descobre o segredo pelo LEDGER, sem
# nenhuma comunicação com alice.
step "bob lê a pré-imagem do ledger da rede 2"
REVEALED_B64=$(query_state network2 bob GetHTLCHashPreImageByContractId "[\"$CID\"]" | tr -d '"')
REVEALED=$(printf '%s' "$REVEALED_B64" | base64 -d 2>/dev/null || echo "")
if [[ "$REVEALED" == "$SECRET" ]]; then
  ok "segredo recuperado do ledger: '$REVEALED'"
else
  err "o segredo lido ('$REVEALED') não corresponde ao original"
  exit 1
fi

run_step 6 claim-bond network1 \
  --secret="$REVEALED" --locker=alice --recipient=bob --param="$BOND_TYPE:$BOND_ID"

# ------------------------------------------------------------------ desfecho

section "Estado final"
OWNER_AFTER=$(bond_owner)
ALICE_AFTER=$(token_balance alice)
BOB_AFTER=$(token_balance bob)
info "rede 1 — $BOND_TYPE:$BOND_ID dono: $OWNER_BEFORE -> $OWNER_AFTER"
info "rede 2 — token1 alice: ${ALICE_BEFORE:-?} -> ${ALICE_AFTER:-?}"
info "rede 2 — token1 bob:   ${BOB_BEFORE:-?} -> ${BOB_AFTER:-?}"

section "Veredito"

fails=0

if [[ "$OWNER_AFTER" == "bob" ]]; then
  ok "o bond passou para bob na rede 1"
else
  err "o bond deveria estar com bob, está com '$OWNER_AFTER'"
  fails=$((fails + 1))
fi

if [[ -n "$ALICE_BEFORE" && -n "$ALICE_AFTER" ]]; then
  if [[ $((ALICE_AFTER - ALICE_BEFORE)) -eq "$TOKEN_QTY" ]]; then
    ok "alice recebeu $TOKEN_QTY $TOKEN_TYPE na rede 2"
  else
    err "alice variou $((ALICE_AFTER - ALICE_BEFORE)), esperado +$TOKEN_QTY"
    fails=$((fails + 1))
  fi
fi

if [[ -n "$BOB_BEFORE" && -n "$BOB_AFTER" ]]; then
  if [[ $((BOB_BEFORE - BOB_AFTER)) -eq "$TOKEN_QTY" ]]; then
    ok "bob cedeu $TOKEN_QTY $TOKEN_TYPE na rede 2"
  else
    err "bob variou $((BOB_AFTER - BOB_BEFORE)), esperado -$TOKEN_QTY"
    fails=$((fails + 1))
  fi
fi

TOTAL_MS=$(( $(now_ms) - T_ZERO ))
echo "$RUN_ID,baseline,total,end-to-end,both,0,$TOTAL_MS,$([[ $fails -eq 0 ]] && echo OK || echo FALHOU)" >> "$CSV"

echo
if [[ "$fails" -eq 0 ]]; then
  ok "SWAP ATÔMICO COMPLETO — ambos os lados efetivaram (COMMITTED_BOTH)"
  info "latência fim a fim: ${TOTAL_MS}ms | 6 transações de protocolo"
  info "CSV: $CSV"
  echo
  ok "PORTÃO DA ETAPA 1: APROVADO"
  exit 0
else
  err "$fails verificação(ões) falharam — ver $CSV"
  exit 1
fi
