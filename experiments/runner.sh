#!/usr/bin/env bash
#
# Executa cenários de experimento com injeção de falha, N vezes cada, e agrega
# os desfechos.
#
# Cada execução recebe ativos VIRGENS. Derrubar e recriar as duas redes levaria
# ~3 minutos por repetição, o que inviabilizaria N >= 10; trocar de ativo não
# custa nada. Os saldos de token acumulam entre execuções, mas isso não
# atrapalha: a classificação compara o estado antes e depois de cada execução,
# não valores absolutos.
#
# Uso:
#   ./experiments/runner.sh --scenario=H2 --repeat=10
#   ./experiments/runner.sh --all --repeat=10
#   ./experiments/runner.sh --list
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/../scripts/lib.sh"

SCENARIO_DIR="$REPO_ROOT/experiments/scenarios"
FAULTS="$REPO_ROOT/experiments/faults"
RESULTS="$REPO_ROOT/experiments/results"
RAW="$RESULTS/raw"
AGGREGATE="$RESULTS/aggregated.csv"

REPEAT=1
ONLY=""
RUN_ALL=0

for arg in "$@"; do
  case "$arg" in
    --scenario=*) ONLY="${arg#*=}" ;;
    --repeat=*)   REPEAT="${arg#*=}" ;;
    --all)        RUN_ALL=1 ;;
    --list)
      section "Cenários disponíveis"
      for f in "$SCENARIO_DIR"/*.env; do
        id=$(basename "$f" .env)
        # shellcheck disable=SC1090
        ( source "$f"; printf '  %-5s %-6s %s\n' "$id" "$PROTOCOL" "$DESCRIPTION" )
      done
      exit 0
      ;;
    *) die "argumento desconhecido: $arg" ;;
  esac
done

[[ "$RUN_ALL" -eq 1 || -n "$ONLY" ]] || \
  die "informe --scenario=<id> ou --all (use --list para ver os cenários)"

need_docker
mkdir -p "$RAW"

# Contador global de execuções: garante identificadores de ativo únicos entre
# invocações do runner, inclusive depois de reiniciar a máquina.
COUNTER_FILE="$RESULTS/.run-counter"
[[ -f "$COUNTER_FILE" ]] || echo 0 > "$COUNTER_FILE"

next_id() {
  local n
  n=$(( $(cat "$COUNTER_FILE") + 1 ))
  echo "$n" > "$COUNTER_FILE"
  printf 'r%04d' "$n"
}

# Cabeçalho do agregado, criado uma vez.
if [[ ! -f "$AGGREGATE" ]]; then
  echo "run_id,protocol,scenario,repetition,outcome,expected,matched,lock_duration_ms,transactions,protocol_latency_ms,detail" \
    > "$AGGREGATE"
fi

# ------------------------------------------------------------------ falhas

apply_faults() {
  local delay=${NETEM_DELAY:-} targets=${NETEM_TARGETS:-}
  [[ -n "$delay" && -n "$targets" ]] || return 0
  for c in $targets; do
    "$FAULTS/netem.sh" apply "$c" "$delay" >/dev/null
  done
  info "atraso de $delay aplicado em: $targets"

  # Com NETEM_DURATION o atraso é levantado sozinho depois de N segundos.
  # Serve para que a LEITURA FINAL do estado aconteça com a rede sã: sob
  # atraso pesado a consulta estoura o timeout e a execução seria perdida por
  # falha da observação, não do protocolo. Modela também o caso realista de
  # uma degradação que passa.
  if [[ -n "${NETEM_DURATION:-}" ]]; then
    ( sleep "$NETEM_DURATION"
      for c in $targets; do
        "$FAULTS/netem.sh" clear "$c" >/dev/null 2>&1 || true
      done ) &
    # Guardamos o PID: sem isso, o job pendente de uma repetição derruba o
    # atraso da SEGUINTE, e a falha injetada simplesmente não acontece — erro
    # que já custou uma execução silenciosamente bem-sucedida.
    NETEM_TIMER_PID=$!
    info "o atraso será removido após ${NETEM_DURATION}s"
  fi
}

clear_faults() {
  # Cancela um removedor agendado que ainda não disparou.
  if [[ -n "${NETEM_TIMER_PID:-}" ]]; then
    kill "$NETEM_TIMER_PID" 2>/dev/null || true
    wait "$NETEM_TIMER_PID" 2>/dev/null || true
    NETEM_TIMER_PID=""
  fi
  # Sempre limpar, mesmo em caso de erro: um atraso esquecido contaminaria
  # todas as execuções seguintes.
  "$FAULTS/netem.sh" clear-all >/dev/null 2>&1 || true
  "$FAULTS/crash-peer.sh" restore-all >/dev/null 2>&1 || true
}
# ATENÇÃO: não usar `trap clear_faults EXIT`.
#
# Em bash, o trap de EXIT também dispara quando uma SUBSHELL termina — e
# substituição de comando ($(...)) cria subshell. O efeito era devastador e
# silencioso: qualquer $(...) executado depois de aplicar o atraso o removia na
# hora, e os cenários rodavam sem falha nenhuma, reportando sucesso.
#
# Limpamos explicitamente: no início da invocação, ao fim de cada execução, e
# por interrupção do usuário.
trap 'clear_faults; exit 130' INT TERM

# ------------------------------------------------------------------ execução

# extrai_total <csv> <coluna> — lê um campo da linha "total" do CSV da execução
extrai_total() {
  awk -F, -v col="$2" 'NR==1{for(i=1;i<=NF;i++) if($i==col) c=i; next}
                       $4=="total"{print $c; exit}' "$1"
}

# janela_protocolo <csv> — latência do protocolo em tempo de parede.
#
# Somar as durações dos passos estaria ERRADO para o 2PC: as duas fases vão às
# redes em paralelo, e a soma contaria duas vezes o que acontece ao mesmo
# tempo, apagando exatamente a vantagem que queremos medir. Usamos a janela:
# do início do primeiro passo ao fim do último.
janela_protocolo() {
  awk -F, '
    $4 ~ /^[0-9]+$/ && $9=="OK" {
      ini = $7 + 0; fim = $7 + $8
      if (!seen++ || ini < min) min = ini
      if (fim > max) max = fim
    }
    END { printf "%d", seen ? max - min : 0 }' "$1"
}

seed_htlc() {
  go run "$REPO_ROOT/apps/htlc-orchestrator" -seed="$1" >/dev/null 2>&1
}

run_htlc() {
  local run_id=$1 bond=$2
  local args=(-scenario="$SCENARIO" -run-id="$run_id" -bond-id="$bond"
              -out="$RAW" -t1="${T1:-10m}" -t2="${T2:-5m}")
  [[ -n "${CRASH_AFTER:-}" ]] && args+=(-crash-after="$CRASH_AFTER")
  [[ -n "${BOB_CLOCK_SKEW:-}" ]] && args+=(-bob-clock-skew="$BOB_CLOCK_SKEW")
  [[ "${RECLAIM:-false}" == "true" ]] && args+=(-reclaim)

  go run "$REPO_ROOT/apps/htlc-orchestrator" "${args[@]}" >"$RAW/$run_id.log" 2>&1 || true
}

seed_2pc() {
  local run_id=$1 a1=$2 a2=$3
  go run "$REPO_ROOT/apps/coordinator" -seed="$a1:$a2" >/dev/null 2>&1

  # Um voto NÃO é produzido prendendo o ativo numa transação anterior que
  # nunca é resolvida — é o que acontece na prática quando um recurso está
  # comprometido em outra transação em andamento.
  if [[ -n "${VOTE_NO_ON:-}" ]]; then
    local holder="hold-$run_id"
    go run "$REPO_ROOT/apps/coordinator" -seed="${a2}h:${a2}" >/dev/null 2>&1
    go run "$REPO_ROOT/apps/coordinator" -tx="$holder" -a1="${a2}h" -a2="$a2" \
      -crash-after=decision -deadline=30m -wal="$RAW/wal-$run_id" >/dev/null 2>&1 || true
  fi
}

run_2pc() {
  local run_id=$1 a1=$2 a2=$3
  local args=(-scenario="$SCENARIO" -run-id="$run_id" -a1="$a1" -a2="$a2"
              -out="$RAW" -wal="$RAW/wal-$run_id" -deadline="${DEADLINE:-10m}"
              -parallel="${PARALLEL:-true}")
  [[ -n "${CRASH_AFTER:-}" ]] && args+=(-crash-after="$CRASH_AFTER")

  go run "$REPO_ROOT/apps/coordinator" "${args[@]}" >"$RAW/$run_id.log" 2>&1 || true

  # Válvula de escape num participante só (cenário T4).
  if [[ -n "${TIMEOUT_ABORT_ON:-}" ]]; then
    sleep "${RECOVER_DELAY:-18}"
    local net=$TIMEOUT_ABORT_ON asset=$a2 user=bob
    [[ "$net" == "network1" ]] && { asset=$a1; user=alice; }
    ( cd "$CACTI_DIR/weaver/samples/fabric/go-cli" && \
      ./bin/fabric-cli chaincode invoke --local-network="$net" --user="$user" \
        mychannel twopc TimeoutAbort "[\"tx-$run_id\"]" ) >>"$RAW/$run_id.log" 2>&1 || true
  fi

  if [[ "${RECOVER:-false}" == "true" ]]; then
    [[ -n "${TIMEOUT_ABORT_ON:-}" ]] || sleep "${RECOVER_DELAY:-20}"
    go run "$REPO_ROOT/apps/coordinator" -recover -scenario="$SCENARIO" \
      -run-id="$run_id" -out="$RAW" -wal="$RAW/wal-$run_id" \
      >>"$RAW/$run_id.log" 2>&1 || true
  fi
}

run_once() {
  local rep=$1
  local tag; tag=$(next_id)
  local run_id="${SCENARIO}-${tag}"

  # O PREPARO acontece sem falha injetada: criar ativos é montagem do cenário,
  # não parte do que se mede. Deixá-lo sob atraso além de distorcer o tempo,
  # consumia a janela do NETEM_DURATION antes de o protocolo começar — e o
  # experimento rodava com a rede já sã.
  case "$PROTOCOL" in
    htlc) seed_htlc "$tag" ;;
    2pc)  seed_2pc "$run_id" "${tag}a" "${tag}b" ;;
  esac

  apply_faults

  # Confirma que a falha está de fato ativa antes de executar. Um cenário que
  # roda sem a falha injetada produz um resultado silenciosamente errado — o
  # pior tipo de defeito num experimento.
  if [[ -n "${NETEM_DELAY:-}" ]]; then
    local first; first=$(echo "${NETEM_TARGETS:-}" | awk '{print $1}')
    if ! "$FAULTS/netem.sh" status "$first" 2>/dev/null | grep -q netem; then
      die "o atraso deveria estar ativo em $first mas não está — execução abortada"
    fi
  fi

  case "$PROTOCOL" in
    htlc) run_htlc "$run_id" "$tag" ;;
    2pc)  run_2pc  "$run_id" "${tag}a" "${tag}b" ;;
    *)    die "protocolo desconhecido no cenário $SCENARIO: $PROTOCOL" ;;
  esac

  clear_faults

  local csv="$RAW/$run_id.csv"
  if [[ ! -f "$csv" ]]; then
    warn_msg "rep $rep: execução não produziu CSV — ver $RAW/$run_id.log"
    echo "$run_id,$PROTOCOL,$SCENARIO,$rep,NO_CSV,${EXPECTED:-},nao,,,," >> "$AGGREGATE"
    return
  fi

  local outcome lock txs latency matched
  outcome=$(extrai_total "$csv" outcome)
  lock=$(extrai_total "$csv" lock_duration_ms)
  txs=$(extrai_total "$csv" transactions)
  latency=$(janela_protocolo "$csv")
  matched=$([[ "$outcome" == "${EXPECTED:-}" ]] && echo sim || echo nao)

  printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
    "$run_id" "$PROTOCOL" "$SCENARIO" "$rep" "$outcome" "${EXPECTED:-}" \
    "$matched" "$lock" "$txs" "$latency" "$(extrai_total "$csv" detail)" \
    >> "$AGGREGATE"

  local mark="✓"; [[ "$matched" == "sim" ]] || mark="!"
  printf '    %s rep %-3s %-16s bloqueio=%-8s protocolo=%-8s txs=%s\n' \
    "$mark" "$rep" "$outcome" "${lock}ms" "${latency}ms" "$txs"
}

run_scenario() {
  local id=$1
  local file="$SCENARIO_DIR/$id.env"
  [[ -f "$file" ]] || die "cenário desconhecido: $id (use --list)"

  # Cada cenário roda num subshell: as variáveis de um não vazam para o outro.
  (
    # shellcheck disable=SC1090
    source "$file"
    SCENARIO=$id

    section "$id — $DESCRIPTION"
    info "protocolo: $PROTOCOL | esperado: ${EXPECTED:-?} | repetições: $REPEAT"
    [[ -n "${NETEM_DELAY:-}" ]] && info "atraso injetado: $NETEM_DELAY"

    for rep in $(seq 1 "$REPEAT"); do
      run_once "$rep"
    done
  )
}

# ------------------------------------------------------------------ principal

section "Harness de experimentos"
info "repetições por cenário: $REPEAT"
info "resultados brutos:      $RAW"
info "agregado:               $AGGREGATE"

clear_faults

if [[ "$RUN_ALL" -eq 1 ]]; then
  for f in "$SCENARIO_DIR"/*.env; do
    run_scenario "$(basename "$f" .env)"
  done
else
  run_scenario "$ONLY"
fi

section "Resumo"
python3 "$REPO_ROOT/analysis/summarize.py" "$AGGREGATE" 2>/dev/null || {
  info "linhas no agregado: $(( $(wc -l < "$AGGREGATE") - 1 ))"
  info "veja $AGGREGATE"
}
