package main

// Instrumentação: cronometragem por passo e gravação em CSV.
//
// O formato é o mesmo que o coordenador 2PC vai usar, para que a análise
// (analysis/analyze.py) trate os dois braços com o mesmo código. Qualquer
// mudança aqui precisa ser espelhada lá.

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Outcome classifica o desfecho de uma execução olhando os dois ledgers.
// É a variável dependente central do experimento.
type Outcome string

const (
	// OutcomeCommittedBoth indica troca completa: ambos os lados efetivaram.
	OutcomeCommittedBoth Outcome = "COMMITTED_BOTH"
	// OutcomeAbortedBoth indica aborto correto: nenhum lado efetivou.
	OutcomeAbortedBoth Outcome = "ABORTED_BOTH"
	// OutcomeViolated indica falha de ATOMICIDADE: só um lado efetivou.
	OutcomeViolated Outcome = "VIOLATED"
	// OutcomeBlocked indica falha de VIVACIDADE: ativo preso, sem resolução.
	OutcomeBlocked Outcome = "BLOCKED"
	// OutcomeError indica falha da própria execução, não do protocolo.
	OutcomeError Outcome = "ERROR"
)

// Step é uma linha de medição.
type Step struct {
	Index    int
	Label    string
	Network  string
	Start    time.Duration // desde o início da execução
	Duration time.Duration
	Result   string // OK, FALHOU ou SKIPPED
	Detail   string
}

// Recorder acumula as medições de uma execução e as grava em CSV.
type Recorder struct {
	RunID    string
	Scenario string
	Protocol string

	origin time.Time
	steps  []Step
}

// NewRecorder inicia a cronometragem. O relógio zero é este instante.
func NewRecorder(runID, scenario, protocol string) *Recorder {
	return &Recorder{
		RunID:    runID,
		Scenario: scenario,
		Protocol: protocol,
		origin:   time.Now(),
	}
}

// Elapsed devolve o tempo desde o início da execução.
func (r *Recorder) Elapsed() time.Duration { return time.Since(r.origin) }

// Time executa fn cronometrando e registra o resultado.
//
// Devolve o erro de fn para que o chamador decida se interrompe: um passo que
// falha pode ser exatamente o comportamento esperado do cenário.
func (r *Recorder) Time(index int, label, network string, fn func() (string, error)) (string, error) {
	start := r.Elapsed()
	t0 := time.Now()
	out, err := fn()
	dur := time.Since(t0)

	step := Step{
		Index:    index,
		Label:    label,
		Network:  network,
		Start:    start,
		Duration: dur,
		Result:   "OK",
	}
	if err != nil {
		step.Result = "FALHOU"
		step.Detail = truncate(err.Error(), 200)
	}
	r.steps = append(r.steps, step)
	return out, err
}

// Skip registra um passo deliberadamente não executado (por injeção de falha),
// para que a contagem de transações do cenário fique explícita no CSV.
func (r *Recorder) Skip(index int, label, network, reason string) {
	r.steps = append(r.steps, Step{
		Index:   index,
		Label:   label,
		Network: network,
		Start:   r.Elapsed(),
		Result:  "SKIPPED",
		Detail:  reason,
	})
}

// Note registra um evento sem duração (leitura de estado, marco do protocolo).
func (r *Recorder) Note(index int, label, network, detail string) {
	r.steps = append(r.steps, Step{
		Index:   index,
		Label:   label,
		Network: network,
		Start:   r.Elapsed(),
		Result:  "INFO",
		Detail:  truncate(detail, 200),
	})
}

// Summary agrega o que a análise precisa por execução.
type Summary struct {
	Outcome Outcome
	// LockDuration é o tempo em que o ativo ficou indisponível ao dono:
	// do travamento até o resgate ou a devolução. É a métrica de bloqueio.
	LockDuration time.Duration
	// Transactions conta as escritas efetivamente submetidas (custo).
	Transactions int
	Detail       string
}

// Write grava o CSV da execução. Uma linha por passo, mais a linha "total".
func (r *Recorder) Write(path string, s Summary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("não foi possível criar %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("não foi possível escrever %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"run_id", "protocol", "scenario", "step", "label", "network",
		"t_start_ms", "duration_ms", "result", "outcome",
		"lock_duration_ms", "transactions", "detail",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, st := range r.steps {
		row := []string{
			r.RunID, r.Protocol, r.Scenario,
			strconv.Itoa(st.Index), st.Label, st.Network,
			ms(st.Start), ms(st.Duration), st.Result,
			"", "", "", st.Detail,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	// Linha de fechamento: carrega o desfecho e as métricas agregadas.
	total := []string{
		r.RunID, r.Protocol, r.Scenario,
		"total", "end-to-end", "both",
		"0", ms(r.Elapsed()), "OK", string(s.Outcome),
		ms(s.LockDuration), strconv.Itoa(s.Transactions), s.Detail,
	}
	return w.Write(total)
}

// Steps expõe as medições para impressão no terminal.
func (r *Recorder) Steps() []Step { return r.steps }

func ms(d time.Duration) string {
	return strconv.FormatInt(d.Milliseconds(), 10)
}

func truncate(s string, n int) string {
	// Quebras de linha arruinariam o CSV; o SDK produz stack traces enormes.
	out := make([]rune, 0, n)
	for _, r := range s {
		if r == '\n' || r == '\r' {
			r = ' '
		}
		out = append(out, r)
		if len(out) >= n {
			break
		}
	}
	return string(out)
}
