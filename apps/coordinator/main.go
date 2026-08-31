// Coordenador 2PC instrumentado.
//
// Conduz uma troca de ativos entre as duas redes Fabric pelo protocolo de duas
// fases, medindo cada passo, com pontos de injeção de falha e write-ahead log.
// É o braço 2PC do estudo comparativo; emite CSV no mesmo formato do
// orquestrador HTLC.
//
// Exemplos:
//
//	# caso sem falha
//	go run ./apps/coordinator -scenario=B2 -a1=w10 -a2=w20
//
//	# T2: coordenador morto entre as fases — o cenário central deste braço
//	go run ./apps/coordinator -scenario=T2 -a1=w11 -a2=w21 -crash-after=decision
//
//	# retomar o que ficou pendente no WAL
//	go run ./apps/coordinator -recover
//
//	# preparar ativos para uma execução
//	go run ./apps/coordinator -seed=w10:w20
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/ledger"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/run"
)

func main() {
	var (
		scenario   = flag.String("scenario", "B2", "identificador do cenário (B2, T1, T2, T3...)")
		crashAfter = flag.String("crash-after", "none",
			"ponto de interrupção: none|prepare|decision|first-commit")
		asset1   = flag.String("a1", "w10", "ativo na rede 1 (de alice)")
		asset2   = flag.String("a2", "w20", "ativo na rede 2 (de bob)")
		owner1   = flag.String("owner1", "alice", "dono do ativo na rede 1")
		owner2   = flag.String("owner2", "bob", "dono do ativo na rede 2")
		deadline = flag.Duration("deadline", 10*time.Minute,
			"prazo após o qual os participantes podem abortar unilateralmente")
		parallel = flag.Bool("parallel", true,
			"enviar cada fase às duas redes simultaneamente")
		seed = flag.String("seed", "",
			"criar os ativos e sair, no formato <ativo-n1>:<ativo-n2>")
		recover   = flag.Bool("recover", false, "retomar transações pendentes do WAL")
		timeoutCC = flag.Bool("timeout-abort", false,
			"invocar TimeoutAbort nas transações pendentes (o watchdog do participante)")
		stateOnly = flag.Bool("state-only", false, "apenas imprimir o estado dos ledgers")
		txID      = flag.String("tx", "", "identificador da transação (padrão: derivado do run-id)")
		runID     = flag.String("run-id", "", "identificador da execução (padrão: timestamp)")
		outDir    = flag.String("out", "", "diretório dos CSVs")
		walDir    = flag.String("wal", "", "diretório do write-ahead log")
		cfgPath   = flag.String("config", "", "config.json (padrão: o da go-cli)")
		walletDir = flag.String("wallets", "", "raiz das wallets (padrão: a da go-cli)")
		verbose   = flag.Bool("verbose", false, "logs do SDK")
	)
	flag.Parse()

	log.SetLevel(log.ErrorLevel)
	if *verbose {
		log.SetLevel(log.InfoLevel)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}
	goCLI := filepath.Join(repoRoot, "cacti", "weaver", "samples", "fabric", "go-cli")

	if *cfgPath == "" {
		*cfgPath = filepath.Join(goCLI, "config.json")
	}
	if *walletDir == "" {
		*walletDir = filepath.Join(goCLI, "wallets")
	}
	if *outDir == "" {
		*outDir = filepath.Join(repoRoot, "experiments", "results")
	}
	if *walDir == "" {
		*walDir = filepath.Join(repoRoot, "apps", "coordinator", "wal")
	}
	if *runID == "" {
		*runID = fmt.Sprintf("%s-%s", *scenario, time.Now().Format("20060102-150405"))
	}
	if *txID == "" {
		*txID = "tx-" + *runID
	}

	cp, err := parseCrashPoint(*crashAfter)
	if err != nil {
		fatal(err)
	}

	lcfg, err := ledger.LoadConfig(*cfgPath)
	if err != nil {
		fatal(fmt.Errorf("%w\n  Rode ./scripts/03-setup-htlc.sh antes", err))
	}

	wal, err := OpenWAL(*walDir)
	if err != nil {
		fatal(err)
	}

	cfg := Config{
		TxID:           *txID,
		Asset1:         *asset1,
		Owner1:         *owner1,
		Asset2:         *asset2,
		Owner2:         *owner2,
		Deadline:       *deadline,
		ParallelPhases: *parallel,
		CrashAfter:     cp,
	}

	rec := run.NewRecorder(*runID, *scenario, "2PC")

	coord, err := New(lcfg, *walletDir, cfg, wal, rec)
	if err != nil {
		fatal(err)
	}
	defer coord.Close()

	switch {
	case *seed != "":
		doSeed(coord, *seed, *owner1, *owner2)
		return
	case *stateOnly:
		st, err := coord.ReadState()
		if err != nil {
			fatal(err)
		}
		fmt.Println(st)
		return
	case *timeoutCC:
		doTimeoutAbort(coord, wal)
		return
	case *recover:
		doRecover(coord, wal, rec, *outDir, *runID)
		return
	}

	// ------------------------------------------------------------ execução
	header(*runID, *scenario, cfg)

	before, err := coord.ReadState()
	if err != nil {
		fatal(fmt.Errorf("não foi possível ler o estado inicial: %w", err))
	}
	fmt.Printf("  estado inicial : %s\n", before)

	if !before.N1.Found || !before.N2.Found {
		fatal(fmt.Errorf(
			"os ativos %s (rede 1) e %s (rede 2) precisam existir.\n"+
				"  Crie-os: go run ./apps/coordinator -seed=%s:%s",
			cfg.Asset1, cfg.Asset2, cfg.Asset1, cfg.Asset2))
	}

	fmt.Println("\n  execução do protocolo:")
	runErr := coord.Run()
	printSteps(rec)

	if errors.Is(runErr, ErrCrashInjected) {
		fmt.Println("\n  >> coordenador interrompido (falha injetada)")
	} else if runErr != nil {
		fmt.Printf("\n  ! erro: %v\n", runErr)
	}

	finish(coord, rec, before, *outDir, *runID)
}

// doSeed cria os ativos usados por uma execução.
//
// Cada execução usa ativos novos: recriar as redes leva minutos, trocar de
// ativo não leva nada.
func doSeed(coord *Coordinator, spec, owner1, owner2 string) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		fatal(fmt.Errorf("formato de -seed inválido: use <ativo-n1>:<ativo-n2>"))
	}

	for _, s := range []struct {
		sess    *ledger.Session
		network string
		asset   string
		owner   string
	}{
		{coord.s1, "network1", parts[0], owner1},
		{coord.s2, "network2", parts[1], owner2},
	} {
		_, err := s.sess.Contract.SubmitTransaction("CreateAsset", s.asset, s.owner, "100")
		if err != nil {
			if strings.Contains(err.Error(), "já existe") {
				fmt.Printf("  · %s: %s já existe (dono %s)\n", s.network, s.asset, s.owner)
				continue
			}
			fatal(fmt.Errorf("falha ao criar %s em %s: %w", s.asset, s.network, err))
		}
		fmt.Printf("  ✓ %s: %s criado para %s\n", s.network, s.asset, s.owner)
	}
}

// doTimeoutAbort exerce a válvula de escape do participante.
//
// É o watchdog externo: no Fabric nada dispara sozinho, então sem esta
// invocação um ativo com prazo vencido continua preso indefinidamente.
func doTimeoutAbort(coord *Coordinator, wal *WAL) {
	pending, err := wal.Pending()
	if err != nil {
		fatal(err)
	}
	if len(pending) == 0 {
		fmt.Println("  nada pendente no WAL")
		return
	}

	for _, rec := range pending {
		fmt.Printf("\n  %s (fase %s)\n", rec.TxID, rec.Phase)
		for _, p := range rec.Participants {
			sess := coord.s1
			if p.Network == "network2" {
				sess = coord.s2
			}
			_, err := sess.Contract.SubmitTransaction("TimeoutAbort", rec.TxID)
			switch {
			case err == nil:
				fmt.Printf("    ✓ %s: ativo liberado por expiração\n", p.Network)
			case strings.Contains(err.Error(), "ainda não venceu"):
				fmt.Printf("    · %s: prazo ainda não venceu\n", p.Network)
			case strings.Contains(err.Error(), "não foi preparada"):
				fmt.Printf("    · %s: nada preparado aqui\n", p.Network)
			default:
				fmt.Printf("    ! %s: %s\n", p.Network, run.Truncate(err.Error(), 120))
			}
		}
	}
}

// doRecover retoma as transações pendentes a partir do WAL.
func doRecover(coord *Coordinator, wal *WAL, rec *run.Recorder, outDir, runID string) {
	pending, err := wal.Pending()
	if err != nil {
		fatal(err)
	}
	if len(pending) == 0 {
		fmt.Println("  nada pendente no WAL")
		return
	}

	fmt.Printf("\nRecuperação — %d transação(ões) pendente(s)\n", len(pending))

	for i, r := range pending {
		// A configuração precisa apontar para os ativos DESTA transação antes
		// de qualquer leitura: comparar o estado inicial de um ativo com o
		// final de outro produz classificação sem sentido.
		coord.cfg.TxID = r.TxID
		for _, p := range r.Participants {
			if p.Network == "network1" {
				coord.cfg.Asset1 = p.AssetID
			} else {
				coord.cfg.Asset2 = p.AssetID
			}
		}

		before, err := coord.ReadState()
		if err != nil {
			fatal(err)
		}

		fmt.Printf("\n  %s (fase %s)\n", r.TxID, r.Phase)
		fmt.Printf("  estado inicial : %s\n", before)

		// O bloqueio começou quando o coordenador original preparou, não
		// agora: é o tempo que os ativos passaram presos, atravessando o
		// crash. Sem isto o cenário T2 reportaria bloqueio ~zero, escondendo
		// justamente o custo que ele existe para medir.
		coord.AdoptLockStart(r.StartedAt)

		if err := coord.Recover(r); err != nil {
			fmt.Printf("    ! %v\n", err)
		}
		printSteps(rec)

		id := runID
		if len(pending) > 1 {
			id = fmt.Sprintf("%s-%d", runID, i+1)
		}
		finish(coord, rec, before, outDir, id)
	}
}

// finish lê o estado final, classifica e grava o CSV.
func finish(coord *Coordinator, rec *run.Recorder, before LedgerState, outDir, runID string) {
	after, err := coord.ReadState()
	if err != nil {
		fatal(fmt.Errorf("não foi possível ler o estado final: %w", err))
	}
	fmt.Printf("\n  estado final   : %s\n", after)

	observed, detail := Classify(before, after, coord.cfg.TxID)
	summary := coord.Summarize(observed, detail)

	csvPath := filepath.Join(outDir, runID+".csv")
	if err := rec.Write(csvPath, summary); err != nil {
		fatal(err)
	}

	fmt.Printf("\n  desfecho       : %s — %s\n", observed, detail)
	fmt.Printf("  bloqueio       : %s\n", summary.LockDuration.Round(time.Millisecond))
	fmt.Printf("  transações     : %d escritas\n", summary.Transactions)
	fmt.Printf("  CSV            : %s\n", csvPath)
}

func header(runID, scenario string, cfg Config) {
	fmt.Printf("\n2PC — %s (cenário %s)\n", runID, scenario)
	fmt.Printf("  transação      : %s\n", cfg.TxID)
	fmt.Printf("  troca          : %s de %s (rede 1)  <->  %s de %s (rede 2)\n",
		cfg.Asset1, cfg.Owner1, cfg.Asset2, cfg.Owner2)
	fmt.Printf("  prazo          : %s\n", cfg.Deadline)
	mode := "sequenciais"
	if cfg.ParallelPhases {
		mode = "paralelas"
	}
	fmt.Printf("  fases          : %s\n", mode)
	if cfg.CrashAfter != CrashNone {
		fmt.Printf("  falha injetada : coordenador morto após %s\n", cfg.CrashAfter)
	}
	fmt.Println()
}

var printedSteps int

func printSteps(rec *run.Recorder) {
	steps := rec.Steps()
	for _, st := range steps[printedSteps:] {
		switch st.Result {
		case "INFO":
			fmt.Printf("    · %-20s %s\n", st.Label, st.Detail)
		case "SKIPPED":
			fmt.Printf("    - %-20s pulado (%s)\n", st.Label, st.Detail)
		case "OK":
			fmt.Printf("    ✓ %-20s %6dms  [%s]\n",
				st.Label, st.Duration.Milliseconds(), st.Network)
		default:
			fmt.Printf("    ✗ %-20s %6dms  %s\n",
				st.Label, st.Duration.Milliseconds(), st.Detail)
		}
	}
	printedSteps = len(steps)
}

func parseCrashPoint(s string) (CrashPoint, error) {
	switch s {
	case "none", "":
		return CrashNone, nil
	case "prepare":
		return CrashPrepare, nil
	case "decision":
		return CrashDecision, nil
	case "first-commit":
		return CrashFirstCommit, nil
	default:
		return CrashNone, fmt.Errorf(
			"ponto de interrupção inválido %q (use um de: %v)", s, ValidCrashPoints)
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("raiz do repositório não encontrada (go.mod ausente)")
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "\nerro: %v\n", err)
	os.Exit(1)
}
