// Orquestrador HTLC instrumentado.
//
// Conduz um swap atômico entre as duas redes Fabric medindo cada passo, com
// pontos de injeção de falha, e classifica o desfecho lendo os dois ledgers.
// É o braço HTLC do estudo comparativo; o coordenador 2PC escreve CSV no mesmo
// formato para que a análise trate os dois igualmente.
//
// Exemplos:
//
//	# caso sem falha
//	go run ./apps/htlc-orchestrator -scenario=B1
//
//	# H1: bob nunca trava; alice recupera por expiração
//	go run ./apps/htlc-orchestrator -scenario=H1 -crash-after=lock1 -t1=20s -t2=10s -reclaim
//
//	# H2: alice resgata e bob não age — o cenário perigoso
//	go run ./apps/htlc-orchestrator -scenario=H2 -crash-after=claim1
//
//	# só ler o estado atual dos dois ledgers
//	go run ./apps/htlc-orchestrator -state-only
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/ledger"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/run"
	log "github.com/sirupsen/logrus"
)

func main() {
	var (
		scenario   = flag.String("scenario", "B1", "identificador do cenário (B1, H1, H2, H3...)")
		crashAfter = flag.String("crash-after", "none",
			"ponto de interrupção: none|lock1|lock2|claim1")
		secret    = flag.String("secret", "", "segredo do hashlock (padrão: gerado)")
		t1        = flag.Duration("t1", 10*time.Minute, "prazo de quem inicia (alice)")
		t2        = flag.Duration("t2", 5*time.Minute, "prazo de quem responde (bob)")
		bondType  = flag.String("bond-type", "bond01", "tipo do ativo não fungível")
		bondID    = flag.String("bond-id", "a03", "identificador do ativo")
		tokenType = flag.String("token-type", "token1", "tipo do ativo fungível")
		tokenQty  = flag.Uint64("token-qty", 100, "unidades a trocar")
		reclaim   = flag.Bool("reclaim", false,
			"após a interrupção, esperar a expiração e exercer o resgate por prazo")
		stateOnly = flag.Bool("state-only", false, "apenas imprimir o estado dos ledgers")
		runID     = flag.String("run-id", "", "identificador da execução (padrão: timestamp)")
		outDir    = flag.String("out", "", "diretório dos CSVs (padrão: experiments/results)")
		cfgPath   = flag.String("config", "", "config.json (padrão: o da go-cli)")
		walletDir = flag.String("wallets", "", "raiz das wallets (padrão: a da go-cli)")
		verbose   = flag.Bool("verbose", false, "logs do SDK")
	)
	flag.Parse()

	// O SDK do Fabric é extremamente verboso; por padrão só mostramos erros.
	log.SetLevel(log.ErrorLevel)
	if *verbose {
		log.SetLevel(log.InfoLevel)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}
	goCLI := filepath.Join(repoRoot,
		"cacti", "weaver", "samples", "fabric", "go-cli")

	if *cfgPath == "" {
		*cfgPath = filepath.Join(goCLI, "config.json")
	}
	if *walletDir == "" {
		*walletDir = filepath.Join(goCLI, "wallets")
	}
	if *outDir == "" {
		*outDir = filepath.Join(repoRoot, "experiments", "results")
	}
	if *runID == "" {
		*runID = fmt.Sprintf("%s-%s", *scenario, time.Now().Format("20060102-150405"))
	}
	if *secret == "" {
		*secret = fmt.Sprintf("secret-%s", *runID)
	}

	cp, err := parseCrashPoint(*crashAfter)
	if err != nil {
		fatal(err)
	}

	cfg, err := ledger.LoadConfig(*cfgPath)
	if err != nil {
		fatal(fmt.Errorf("%w\n  Rode ./scripts/03-setup-htlc.sh antes", err))
	}

	sc := SwapConfig{
		Secret:     *secret,
		T1:         *t1,
		T2:         *t2,
		BondType:   *bondType,
		BondID:     *bondID,
		TokenType:  *tokenType,
		TokenQty:   *tokenQty,
		CrashAfter: cp,
	}

	rec := run.NewRecorder(*runID, *scenario, "HTLC")

	swap, err := NewSwap(cfg, *walletDir, sc, rec)
	if err != nil {
		fatal(err)
	}
	defer swap.Close()

	// ---- modo somente leitura ----------------------------------------
	if *stateOnly {
		st, err := swap.ReadState()
		if err != nil {
			fatal(err)
		}
		fmt.Printf("rede 1: bond %s:%s dono=%s travado=%t\n",
			sc.BondType, sc.BondID, st.BondOwner, st.BondLocked)
		fmt.Printf("rede 2: %s alice=%d bob=%d\n",
			sc.TokenType, st.TokenBalances["alice"], st.TokenBalances["bob"])
		return
	}

	// ---- execução ----------------------------------------------------
	header(*runID, *scenario, sc)

	before, err := swap.ReadState()
	if err != nil {
		fatal(fmt.Errorf("não foi possível ler o estado inicial: %w", err))
	}
	fmt.Printf("  estado inicial : %s\n", before)

	if before.BondOwner != "alice" {
		fatal(fmt.Errorf(
			"o bond deveria começar com alice, está com %q.\n"+
				"  Reinicie o estado: make clean-networks networks-up setup-htlc",
			before.BondOwner))
	}

	fmt.Println("\n  execução do protocolo:")
	_, runErr := swap.Run()
	printSteps(rec)

	// ---- resgate por expiração (o watchdog da vítima) ----------------
	if *reclaim && cp != CrashNone {
		exerciseReclaim(swap, rec, sc, cp)
	}

	// ---- desfecho observado ------------------------------------------
	after, err := swap.ReadState()
	if err != nil {
		fatal(fmt.Errorf("não foi possível ler o estado final: %w", err))
	}
	fmt.Printf("\n  estado final   : %s\n", after)

	// O resumo é montado agora, depois de eventuais resgates, para que o tempo
	// de bloqueio e a contagem de escritas incluam o caminho de devolução.
	observed, detail := Classify(before, after, sc.TokenQty)
	summary := swap.Summarize(observed, detail)

	csvPath := filepath.Join(*outDir, *runID+".csv")
	if err := rec.Write(csvPath, summary); err != nil {
		fatal(err)
	}

	fmt.Printf("\n  desfecho       : %s — %s\n", observed, detail)
	fmt.Printf("  bloqueio       : %s\n", summary.LockDuration.Round(time.Millisecond))
	fmt.Printf("  transações     : %d escritas\n", summary.Transactions)
	fmt.Printf("  CSV            : %s\n", csvPath)

	// Um erro de execução não é necessariamente falha do experimento: em
	// cenários de injeção, a interrupção é o comportamento pretendido.
	if runErr != nil && observed == run.OutcomeError {
		fatal(runErr)
	}
}

// exerciseReclaim espera a expiração do prazo relevante e exerce o resgate.
//
// No Fabric nada expira sozinho: sem esta invocação o ativo fica preso mesmo
// com o prazo vencido. O tempo de espera é parte da métrica de bloqueio.
func exerciseReclaim(swap *Swap, rec *run.Recorder, sc SwapConfig, cp CrashPoint) {
	// Quem precisa resgatar, e após qual prazo, depende de onde paramos.
	var wait time.Duration
	switch cp {
	case CrashLock1, CrashClaim1:
		wait = sc.T1 // alice recupera o bond
	case CrashLock2:
		wait = sc.T2 // bob recupera os tokens primeiro (prazo menor)
	default:
		return
	}

	fmt.Printf("\n  aguardando expiração (%s) para exercer o resgate...\n", wait)
	// Margem para o timestamp da transação ficar seguramente após o prazo.
	time.Sleep(wait + 3*time.Second)

	if cp == CrashLock2 {
		if err := swap.ReclaimTokens(); err != nil {
			fmt.Printf("  ! resgate dos tokens falhou: %v\n", err)
		} else {
			fmt.Println("  ✓ bob recuperou os tokens por expiração de T2")
		}
	}

	if err := swap.ReclaimBond(); err != nil {
		fmt.Printf("  ! resgate do bond falhou: %v\n", err)
	} else {
		fmt.Println("  ✓ alice recuperou o bond por expiração de T1")
	}
	printSteps(rec)
}

func header(runID, scenario string, sc SwapConfig) {
	fmt.Printf("\nHTLC — %s (cenário %s)\n", runID, scenario)
	fmt.Printf("  prazos         : T1=%s (alice) > T2=%s (bob)\n", sc.T1, sc.T2)
	fmt.Printf("  ativos         : %s:%s  <->  %d %s\n",
		sc.BondType, sc.BondID, sc.TokenQty, sc.TokenType)
	if sc.CrashAfter != CrashNone {
		fmt.Printf("  falha injetada : interrupção após %s\n", sc.CrashAfter)
	}
	fmt.Println()
}

// printSteps imprime só as medições ainda não impressas.
var printedSteps int

func printSteps(rec *run.Recorder) {
	steps := rec.Steps()
	for _, st := range steps[printedSteps:] {
		switch st.Result {
		case "INFO":
			fmt.Printf("    · %-22s %s\n", st.Label, st.Detail)
		case "SKIPPED":
			fmt.Printf("    - %-22s pulado (%s)\n", st.Label, st.Detail)
		case "OK":
			fmt.Printf("    ✓ %-22s %6dms  [%s]\n",
				st.Label, st.Duration.Milliseconds(), st.Network)
		default:
			fmt.Printf("    ✗ %-22s %6dms  %s\n",
				st.Label, st.Duration.Milliseconds(), st.Detail)
		}
	}
	printedSteps = len(steps)
}

func parseCrashPoint(s string) (CrashPoint, error) {
	switch s {
	case "none", "":
		return CrashNone, nil
	case "lock1":
		return CrashLock1, nil
	case "lock2":
		return CrashLock2, nil
	case "claim1":
		return CrashClaim1, nil
	default:
		return CrashNone, fmt.Errorf(
			"ponto de interrupção inválido %q (use um de: %v)", s, ValidCrashPoints)
	}
}

// findRepoRoot sobe os diretórios até achar a raiz do repositório, para que o
// programa funcione de qualquer working directory.
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
