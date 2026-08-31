package main

// O coordenador 2PC.
//
// Fase 1 (preparo): envia Prepare aos dois participantes e coleta os votos.
// Fase 2 (efetivação): persiste a decisão e a comunica, retentando até que
// ambos confirmem.
//
// Por que o coordenador é um processo EXTERNO e não um chaincode: no Fabric,
// InvokeChaincode para outro canal é somente leitura — o resultado não entra no
// write set nem é validado no commit. Não existe transação que abranja as duas
// cadeias. A limitação é conveniente para o experimento: um processo externo é
// trivial de matar num ponto escolhido, que é precisamente a falha que o 2PC
// sofre e o HTLC não.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/ledger"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/run"
)

// CrashPoint marca onde o coordenador deve morrer.
type CrashPoint string

const (
	CrashNone CrashPoint = ""
	// CrashPrepare: cai depois de coletar os votos, ANTES de persistir a
	// decisão. A recuperação pode decidir livremente — ninguém foi informado.
	CrashPrepare CrashPoint = "prepare"
	// CrashDecision: cai depois de persistir a decisão, antes de comunicá-la.
	// É o cenário T2, o mais importante deste braço: os participantes ficam em
	// incerteza com os ativos presos, e só a recuperação (ou o TimeoutAbort)
	// os liberta.
	CrashDecision CrashPoint = "decision"
	// CrashFirstCommit: cai depois de efetivar num participante e antes do
	// outro. Produz estado parcial temporário — o teste mais duro da
	// recuperação.
	CrashFirstCommit CrashPoint = "first-commit"
)

// ChaincodeName é o chaincode 2PC no canal. O config.json da go-cli aponta
// para o simpleasset (usado pelo braço HTLC), então precisamos sobrescrever.
const ChaincodeName = "twopc"

// ValidCrashPoints lista os valores aceitos.
var ValidCrashPoints = []string{"none", "prepare", "decision", "first-commit"}

// ErrCrashInjected sinaliza parada deliberada, não falha real.
var ErrCrashInjected = errors.New("crash injetado")

// Config descreve a transação a coordenar.
type Config struct {
	TxID string
	// Asset1 é o ativo da rede 1, oferecido por Owner1 a Owner2.
	Asset1, Owner1 string
	// Asset2 é o ativo da rede 2, oferecido por Owner2 a Owner1.
	Asset2, Owner2 string
	// Deadline é o prazo após o qual os participantes podem abortar
	// unilateralmente. Prazo generoso mantém o bloqueio observável; prazo curto
	// mede a válvula de escape.
	Deadline time.Duration
	// ParallelPhases envia Prepare (e depois Commit) às duas redes ao mesmo
	// tempo. É o que permite ao 2PC gastar 2 rodadas de bloco onde o HTLC gasta
	// 4 — a hipótese central de desempenho.
	ParallelPhases bool
	CrashAfter     CrashPoint
}

// Coordinator conduz uma transação.
type Coordinator struct {
	cfg Config
	wal *WAL
	rec *run.Recorder

	// Uma sessão por participante, sob a identidade do DONO do ativo naquela
	// rede: só o dono pode oferecer o próprio ativo.
	s1, s2 *ledger.Session
	// Sessão de leitura para observar o estado sem se passar por participante.
	obs1, obs2 *ledger.Session

	transactions int
	preparedAt   time.Time
	settledAt    time.Time
}

// New monta as sessões necessárias.
func New(lcfg ledger.Config, walletRoot string, cfg Config, wal *WAL, rec *run.Recorder) (*Coordinator, error) {
	c := &Coordinator{cfg: cfg, wal: wal, rec: rec}

	type conn struct {
		target  **ledger.Session
		network string
		user    string
	}
	for _, k := range []conn{
		{&c.s1, "network1", cfg.Owner1},
		{&c.s2, "network2", cfg.Owner2},
		{&c.obs1, "network1", cfg.Owner2}, // observa a rede 1 sem ser dono
		{&c.obs2, "network2", cfg.Owner1},
	} {
		sess, err := ledger.ConnectContract(lcfg, walletRoot, k.network, k.user, ChaincodeName)
		if err != nil {
			c.Close()
			return nil, err
		}
		*k.target = sess
	}
	return c, nil
}

// Close libera as sessões.
func (c *Coordinator) Close() {
	for _, s := range []*ledger.Session{c.s1, c.s2, c.obs1, c.obs2} {
		if s != nil {
			s.Close()
		}
	}
}

// Run executa o protocolo completo.
func (c *Coordinator) Run() error {
	cfg := c.cfg
	deadline := time.Now().Add(cfg.Deadline).Unix()

	rec := Record{
		TxID:      cfg.TxID,
		Phase:     PhaseStarted,
		Deadline:  deadline,
		StartedAt: time.Now(),
		Participants: []Participant{
			{Network: "network1", AssetID: cfg.Asset1, Owner: cfg.Owner1, NewOwner: cfg.Owner2},
			{Network: "network2", AssetID: cfg.Asset2, Owner: cfg.Owner2, NewOwner: cfg.Owner1},
		},
	}
	if err := c.wal.Put(rec); err != nil {
		return err
	}

	// ---------------------------------------------------------- fase 1
	votes := c.collectVotes(deadline)
	allYes := true
	for i, err := range votes {
		if err == nil {
			continue
		}
		allYes = false
		// Quem votou NÃO não prometeu nada, então não tem o que desfazer na
		// fase 2. Marcar como confirmado agora evita insistir em Abort contra
		// um participante que responderia "não foi preparada aqui" para sempre.
		rec.Participants[i].Acked = true
		rec.Participants[i].Vote = "NO"
	}
	for i := range rec.Participants {
		if rec.Participants[i].Vote == "" {
			rec.Participants[i].Vote = "YES"
		}
	}

	if allYes {
		rec.Phase = PhaseVoted
		if err := c.wal.Put(rec); err != nil {
			return err
		}
	}

	if cfg.CrashAfter == CrashPrepare {
		// Cai antes de decidir. Os participantes que votaram sim estão em
		// incerteza; nenhuma decisão foi tomada, então a recuperação é livre.
		c.rec.Note(2, "crash", "coordinator", "morto após coletar os votos, antes de decidir")
		return ErrCrashInjected
	}

	// ---------------------------------------------------------- decisão
	//
	// A decisão vai para o disco ANTES de qualquer Commit sair. Inverter esta
	// ordem é o bug clássico do 2PC: o coordenador informaria uma decisão que,
	// após um crash, não conseguiria reconstruir.
	if allYes {
		rec.Phase = PhaseCommitDecided
	} else {
		rec.Phase = PhaseAbortDecided
	}
	rec.DecidedAt = time.Now()

	_, err := c.rec.Time(3, "persist-decision", "coordinator", func() (string, error) {
		return string(rec.Phase), c.wal.Put(rec)
	})
	if err != nil {
		return fmt.Errorf("não foi possível persistir a decisão: %w", err)
	}

	if cfg.CrashAfter == CrashDecision {
		// Cenário T2. A decisão está em disco mas ninguém sabe dela: os dois
		// participantes seguem em incerteza, com os ativos presos, até a
		// recuperação ou o TimeoutAbort.
		c.rec.Note(3, "crash", "coordinator",
			"morto após persistir a decisão, antes de comunicá-la")
		return ErrCrashInjected
	}

	// ---------------------------------------------------------- fase 2
	return c.settle(&rec)
}

// collectVotes executa a fase 1 nos dois participantes.
func (c *Coordinator) collectVotes(deadline int64) []error {
	cfg := c.cfg
	dl := strconv.FormatInt(deadline, 10)

	calls := []call{
		{
			step: 1, label: "prepare-n1", network: "network1", session: c.s1,
			fn: "Prepare", args: []string{cfg.TxID, cfg.Asset1, cfg.Owner2, dl},
		},
		{
			step: 1, label: "prepare-n2", network: "network2", session: c.s2,
			fn: "Prepare", args: []string{cfg.TxID, cfg.Asset2, cfg.Owner1, dl},
		},
	}

	errs := c.dispatch(calls)
	if errs[0] == nil || errs[1] == nil {
		c.preparedAt = time.Now()
	}
	return errs
}

// settle executa a fase 2, retentando até que ambos confirmem.
func (c *Coordinator) settle(rec *Record) error {
	fn := "Commit"
	label := "commit"
	if !rec.Committed() {
		fn = "Abort"
		label = "abort"
	}

	// Um participante cujo Prepare falhou não tem nada a desfazer.
	var calls []call
	for i, p := range rec.Participants {
		if p.Acked {
			continue
		}
		sess := c.s1
		if p.Network == "network2" {
			sess = c.s2
		}
		calls = append(calls, call{
			step: 4, label: fmt.Sprintf("%s-n%d", label, i+1), network: p.Network,
			session: sess, fn: fn, args: []string{c.cfg.TxID}, index: i,
		})
	}

	// Com crash após o primeiro Commit, forçamos ordem sequencial: em
	// paralelo não haveria "primeiro".
	sequential := c.cfg.CrashAfter == CrashFirstCommit

	if sequential {
		for n, cl := range calls {
			errs := c.dispatch([]call{cl})
			if errs[0] == nil {
				rec.Participants[cl.index].Acked = true
				_ = c.wal.Put(*rec)
			}
			if n == 0 && c.cfg.CrashAfter == CrashFirstCommit {
				c.rec.Note(4, "crash", "coordinator",
					"morto após decidir no primeiro participante, antes do segundo")
				return ErrCrashInjected
			}
		}
	} else {
		errs := c.dispatch(calls)
		for i, err := range errs {
			if err == nil {
				rec.Participants[calls[i].index].Acked = true
			}
		}
	}

	// Retentativa: a decisão já é irrevogável, então insistimos. Commit e
	// Abort são idempotentes no chaincode, então reenviar é seguro.
	for attempt := 0; attempt < 3 && !allAcked(*rec); attempt++ {
		time.Sleep(time.Duration(attempt+1) * time.Second)
		var retry []call
		for i, p := range rec.Participants {
			if p.Acked {
				continue
			}
			sess := c.s1
			if p.Network == "network2" {
				sess = c.s2
			}
			retry = append(retry, call{
				step: 5, label: fmt.Sprintf("retry-%s-n%d", label, i+1),
				network: p.Network, session: sess, fn: fn,
				args: []string{c.cfg.TxID}, index: i,
			})
		}
		errs := c.dispatch(retry)
		for i, err := range errs {
			if err == nil {
				rec.Participants[retry[i].index].Acked = true
			}
		}
	}

	if allAcked(*rec) {
		rec.Phase = PhaseDone
		c.settledAt = time.Now()
	}
	if err := c.wal.Put(*rec); err != nil {
		return err
	}

	if !allAcked(*rec) {
		return fmt.Errorf("participantes não confirmaram a decisão após as retentativas")
	}
	return nil
}

// Recover retoma uma transação pendente a partir do WAL.
//
// A regra é a do 2PC: uma decisão persistida é irrevogável. Se o registro diz
// COMMIT_DECIDED, a recuperação efetiva — mesmo que agora fosse mais
// conveniente abortar. Se ainda não havia decisão, ela é livre para abortar.
func (c *Coordinator) Recover(rec Record) error {
	switch rec.Phase {
	case PhaseStarted, PhaseVoted:
		// Sem decisão durável: abortar é seguro e é a escolha correta, porque
		// nenhum participante pode ter ouvido "commit".
		c.rec.Note(6, "recover", "coordinator",
			fmt.Sprintf("sem decisão durável (%s): abortando", rec.Phase))
		rec.Phase = PhaseAbortDecided
		rec.DecidedAt = time.Now()
		if err := c.wal.Put(rec); err != nil {
			return err
		}
		return c.settle(&rec)

	case PhaseCommitDecided, PhaseAbortDecided:
		c.rec.Note(6, "recover", "coordinator",
			fmt.Sprintf("decisão durável encontrada (%s): retomando", rec.Phase))
		return c.settle(&rec)

	case PhaseDone:
		c.rec.Note(6, "recover", "coordinator", "nada pendente")
		return nil

	default:
		return fmt.Errorf("fase desconhecida no WAL: %q", rec.Phase)
	}
}

// ---------------------------------------------------------------- execução

// call descreve uma invocação a um participante.
type call struct {
	step    int
	label   string
	network string
	session *ledger.Session
	fn      string
	args    []string
	index   int
}

// dispatch executa as chamadas, em paralelo quando configurado.
//
// O paralelismo é a razão de o 2PC poder ser mais rápido que o HTLC: as duas
// redes cortam bloco simultaneamente, então uma fase custa uma rodada de bloco
// em vez de duas.
func (c *Coordinator) dispatch(calls []call) []error {
	errs := make([]error, len(calls))
	if len(calls) == 0 {
		return errs
	}

	if !c.cfg.ParallelPhases || len(calls) == 1 {
		for i, cl := range calls {
			errs[i] = c.invoke(cl)
		}
		return errs
	}

	// Em paralelo cada chamada precisa do próprio cronômetro; o Recorder não é
	// seguro para concorrência, então medimos aqui e registramos depois.
	type result struct {
		idx      int
		err      error
		start    time.Duration
		duration time.Duration
		detail   string
	}
	out := make(chan result, len(calls))

	for i, cl := range calls {
		go func(i int, cl call) {
			start := c.rec.Elapsed()
			t0 := time.Now()
			raw, err := cl.session.Contract.SubmitTransaction(cl.fn, cl.args...)
			out <- result{
				idx: i, err: err, start: start, duration: time.Since(t0),
				detail: strings.TrimSpace(string(raw)),
			}
		}(i, cl)
	}

	results := make([]result, len(calls))
	for range calls {
		r := <-out
		results[r.idx] = r
	}

	for i, r := range results {
		errs[i] = r.err
		c.rec.Record(calls[i].step, calls[i].label, calls[i].network,
			r.start, r.duration, r.err)
		if r.err == nil {
			c.transactions++
		}
	}
	return errs
}

// invoke executa uma chamada cronometrando pelo Recorder.
func (c *Coordinator) invoke(cl call) error {
	_, err := c.rec.Time(cl.step, cl.label, cl.network, func() (string, error) {
		raw, err := cl.session.Contract.SubmitTransaction(cl.fn, cl.args...)
		return strings.TrimSpace(string(raw)), err
	})
	if err == nil {
		c.transactions++
	}
	return err
}

func allAcked(rec Record) bool {
	for _, p := range rec.Participants {
		if !p.Acked {
			return false
		}
	}
	return true
}

// AdoptLockStart assume o início do bloqueio de uma execução anterior.
//
// Na recuperação, quem prendeu os ativos foi o coordenador que morreu. O tempo
// de bloqueio relevante é o que os ativos passaram presos ao todo — atravessando
// o crash — e não o que este processo levou para libertá-los.
func (c *Coordinator) AdoptLockStart(t time.Time) {
	if !t.IsZero() {
		c.preparedAt = t
	}
}

// Summarize monta as métricas agregadas. Deve ser chamado no fim, depois de
// eventual recuperação, para que o bloqueio e o custo incluam esse caminho.
func (c *Coordinator) Summarize(o run.Outcome, detail string) run.Summary {
	lock := time.Duration(0)
	if !c.preparedAt.IsZero() {
		end := c.settledAt
		if end.IsZero() {
			end = time.Now()
		}
		lock = end.Sub(c.preparedAt)
	}
	return run.Summary{
		Outcome:      o,
		LockDuration: lock,
		Transactions: c.transactions,
		Detail:       detail,
	}
}
