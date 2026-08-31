package main

// O protocolo HTLC instrumentado, com pontos de injeção de falha.
//
// Sequência (Nolan 2013; Herlihy 2018), com alice iniciando:
//
//   1. alice trava o bond na rede 1 com hash H e prazo T1
//   2. bob verifica o lock de alice
//   3. bob trava os tokens na rede 2 com o MESMO H e prazo T2 < T1
//   4. alice verifica o lock de bob
//   5. alice resgata os tokens apresentando o segredo -> o segredo vira público
//   6. bob lê o segredo do ledger da rede 2 e resgata o bond na rede 1
//
// A assimetria T1 > T2 protege quem revela o segredo primeiro: quando alice
// vaza o segredo no passo 5, bob ainda tem T1 - T2 de folga para agir, e alice
// não consegue reclamar o bond de volta antes disso.

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	am "github.com/hyperledger-cacti/cacti/weaver/sdks/fabric/go-sdk/v3/asset-manager"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/ledger"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/run"
)

// CrashPoint marca onde a execução deve ser interrompida.
type CrashPoint string

const (
	CrashNone   CrashPoint = ""       // execução completa
	CrashLock1  CrashPoint = "lock1"  // após alice travar: bob nunca responde
	CrashLock2  CrashPoint = "lock2"  // após ambos travarem: ninguém resgata
	CrashClaim1 CrashPoint = "claim1" // após alice resgatar: segredo já vazou
)

// ValidCrashPoints lista os valores aceitos, para mensagens de erro.
var ValidCrashPoints = []string{"none", "lock1", "lock2", "claim1"}

// SwapConfig descreve o que trocar e sob qual falha.
type SwapConfig struct {
	Secret     string
	T1         time.Duration // prazo de quem inicia (alice) — o maior
	T2         time.Duration // prazo de quem responde (bob) — o menor
	BondType   string
	BondID     string
	TokenType  string
	TokenQty   uint64
	CrashAfter CrashPoint
	// BobClockSkew desloca o relógio de bob em relação ao das redes.
	//
	// No Fabric o timelock não é nativo: o prazo é um instante absoluto gravado
	// no estado, e quem o compara é o peer, usando o timestamp da transação. Um
	// participante com relógio errado calcula o próprio prazo — e a própria
	// margem de segurança — em cima de uma referência que a rede não
	// compartilha.
	//
	// Positivo = relógio de bob adiantado. É o caso perigoso: ele fixa um prazo
	// mais tarde do que pretendia e, ao conferir a margem com o próprio
	// relógio, conclui que está protegido quando não está.
	BobClockSkew time.Duration
}

// Validate recusa configurações que violariam a segurança do protocolo.
func (c SwapConfig) Validate() error {
	if c.Secret == "" {
		return errors.New("segredo vazio")
	}
	if c.T1 <= c.T2 {
		return fmt.Errorf(
			"T1 (%s) precisa ser maior que T2 (%s): quem revela o segredo primeiro "+
				"precisa da folga para a contraparte agir (ver docs/notas/htlc.md)",
			c.T1, c.T2)
	}
	if c.TokenQty == 0 {
		return errors.New("quantidade de tokens igual a zero")
	}
	return nil
}

// Swap conduz uma execução do protocolo.
type Swap struct {
	cfg SwapConfig
	rec *run.Recorder

	// Sessões: uma por (rede, participante). O HTLC precisa das duas
	// identidades em cada rede, porque locker e recipient são designados por
	// certificado.
	n1Alice, n1Bob *ledger.Session
	n2Alice, n2Bob *ledger.Session

	hash         string
	tokenLockID  string // contractId do lock fungível
	lockedAt     time.Time
	unlockedAt   time.Time
	transactions int
}

// NewSwap monta as quatro sessões necessárias.
func NewSwap(cfg ledger.Config, walletRoot string, sc SwapConfig, rec *run.Recorder) (*Swap, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}

	s := &Swap{cfg: sc, rec: rec}

	type conn struct {
		target  **ledger.Session
		network string
		user    string
	}
	for _, c := range []conn{
		{&s.n1Alice, "network1", "alice"},
		{&s.n1Bob, "network1", "bob"},
		{&s.n2Alice, "network2", "alice"},
		{&s.n2Bob, "network2", "bob"},
	} {
		sess, err := ledger.Connect(cfg, walletRoot, c.network, c.user)
		if err != nil {
			s.Close()
			return nil, err
		}
		*c.target = sess
	}
	return s, nil
}

// Close libera todas as sessões.
func (s *Swap) Close() {
	for _, sess := range []*ledger.Session{s.n1Alice, s.n1Bob, s.n2Alice, s.n2Bob} {
		if sess != nil {
			sess.Close()
		}
	}
}

// Hash devolve o hash publicado no passo 1.
func (s *Swap) Hash() string { return s.hash }

// TokenLockID devolve o contractId do lock fungível (necessário ao resgate).
func (s *Swap) TokenLockID() string { return s.tokenLockID }

// Run executa o protocolo até o fim ou até o ponto de crash configurado.
func (s *Swap) Run() (run.Summary, error) {
	cfg := s.cfg

	// Cada parte fixa o PRÓPRIO prazo no instante em que age — não no início da
	// execução. É como o protocolo funciona de fato, e a diferença é
	// substantiva: o tempo decorrido entre os dois locks come a margem de
	// segurança. Alice trava em A com prazo A+T1; bob trava em B com prazo
	// B+T2. A proteção de bob exige B+T2 < A+T1, ou seja T2 < T1 - (B-A).
	// Sob atraso de rede, (B-A) cresce e pode inverter a ordem dos prazos —
	// que é precisamente o modo de falha que o cenário H3 investiga.
	t1 := uint64(time.Now().Add(cfg.T1).Unix())

	// ---- passo 1: alice trava o bond na rede 1 -----------------------
	out, err := s.rec.Time(1, "lock-bond", "network1", func() (string, error) {
		return am.CreateHTLC(
			s.n1Alice.Contract,
			cfg.BondType, cfg.BondID,
			s.n1Bob.CertB64,
			hashOf(cfg.Secret),
			t1,
		)
	})
	if err != nil {
		return s.summarize(run.OutcomeError, "falha ao travar o bond"), err
	}
	s.transactions++
	s.lockedAt = time.Now()
	s.hash = hashOf(cfg.Secret)
	s.rec.Note(1, "hash-published", "network1", s.hash)
	_ = out

	if cfg.CrashAfter == CrashLock1 {
		// H1: bob nunca trava. O bond de alice fica preso até T1 expirar.
		// Não é violação de atomicidade — nenhum lado efetivou — mas é
		// imobilização de capital, que é o custo que queremos medir.
		s.rec.Skip(3, "lock-tokens", "network2", "crash injetado após lock1")
		return s.summarize(run.OutcomeBlocked,
			"bond travado, contraparte ausente: aguardando expiração de T1"), nil
	}

	// ---- passo 2: bob confere o lock de alice ------------------------
	_, err = s.rec.Time(2, "verify-bond-lock", "network1", func() (string, error) {
		return am.IsAssetLockedInHTLC(
			s.n1Bob.Contract,
			cfg.BondType, cfg.BondID,
			s.n1Bob.CertB64, s.n1Alice.CertB64,
		)
	})
	if err != nil {
		return s.summarize(run.OutcomeError, "bob não confirmou o lock de alice"), err
	}

	// ---- passo 3: bob trava os tokens na rede 2 ----------------------
	//
	// Bob calcula o prazo dele agora, e confere se ainda sobra margem. Um
	// cliente HTLC correto RECUSA travar quando o próprio prazo não caberia
	// antes do de alice: travar nessa situação é entregar o ativo a quem pode
	// resgatar dos dois lados.
	// Bob usa o RELÓGIO DELE, que pode estar deslocado em relação ao da rede.
	bobNow := time.Now().Add(cfg.BobClockSkew)
	t2 := uint64(bobNow.Add(cfg.T2).Unix())

	// E confere a margem com a mesma referência errada. É esse o ponto: a
	// verificação de segurança do protocolo é feita contra o relógio local de
	// quem verifica, não contra o da rede.
	margin := int64(t1) - int64(t2)
	s.rec.Note(3, "safety-margin", "network2",
		fmt.Sprintf("T1-T2 restante: %ds (relógio de bob)", margin))

	if cfg.BobClockSkew != 0 {
		realMargin := int64(t1) - int64(time.Now().Add(cfg.T2).Unix())
		s.rec.Note(3, "clock-skew", "network2",
			fmt.Sprintf("desvio %s: bob crê ter %ds de margem, tem %ds",
				cfg.BobClockSkew, margin, realMargin))
	}

	if margin <= 0 {
		s.rec.Skip(3, "lock-tokens", "network2",
			fmt.Sprintf("margem esgotada (%ds): bob recusa travar", margin))
		return s.summarize(run.OutcomeAbortedBoth,
			fmt.Sprintf("bob recusou travar: o atraso consumiu a margem entre os "+
				"prazos (%ds). O bond de alice fica preso até T1", margin)), nil
	}

	lockID, err := s.rec.Time(3, "lock-tokens", "network2", func() (string, error) {
		return am.CreateFungibleHTLC(
			s.n2Bob.Contract,
			cfg.TokenType, cfg.TokenQty,
			s.n2Alice.CertB64,
			s.hash,
			t2,
		)
	})
	if err != nil {
		return s.summarize(run.OutcomeError, "falha ao travar os tokens"), err
	}
	s.transactions++
	s.tokenLockID = strings.TrimSpace(lockID)
	s.rec.Note(3, "token-lock-id", "network2", s.tokenLockID)

	if cfg.CrashAfter == CrashLock2 {
		// Ambos travados, ninguém resgata: os dois prazos expiram e cada um
		// recupera o seu. Atomicidade preservada, bloqueio máximo.
		s.rec.Skip(5, "claim-tokens", "network2", "crash injetado após lock2")
		return s.summarize(run.OutcomeBlocked,
			"ambos travados, nenhum resgate: aguardando expiração"), nil
	}

	// ---- passo 4: alice confere o lock de bob ------------------------
	_, err = s.rec.Time(4, "verify-token-lock", "network2", func() (string, error) {
		return am.IsFungibleAssetLockedInHTLC(s.n2Alice.Contract, s.tokenLockID)
	})
	if err != nil {
		return s.summarize(run.OutcomeError, "alice não confirmou o lock de bob"), err
	}

	// ---- passo 5: alice resgata e REVELA o segredo -------------------
	_, err = s.rec.Time(5, "claim-tokens", "network2", func() (string, error) {
		return am.ClaimFungibleAssetInHTLC(
			s.n2Alice.Contract,
			s.tokenLockID,
			base64.StdEncoding.EncodeToString([]byte(cfg.Secret)),
		)
	})
	if err != nil {
		return s.summarize(run.OutcomeError, "alice não conseguiu resgatar os tokens"), err
	}
	s.transactions++

	if cfg.CrashAfter == CrashClaim1 {
		// H2, o cenário perigoso: alice já levou os tokens E o segredo é
		// público. Se bob não resgatar antes de T1, alice recupera o bond e
		// fica com os dois ativos — violação de atomicidade. Enquanto T1 não
		// expira, ainda é recuperável.
		s.rec.Skip(6, "claim-bond", "network1", "crash injetado após claim1")
		return s.summarize(run.OutcomeViolated,
			"alice recebeu os tokens; bond ainda travado — bob precisa resgatar antes de T1"), nil
	}

	// ---- passo 6: bob descobre o segredo e resgata -------------------
	//
	// Aqui está o mecanismo do HTLC: bob NÃO fala com alice. Ele lê a
	// pré-imagem do ledger da rede 2, onde o resgate de alice a tornou pública.
	revealed, err := s.readRevealedSecret()
	if err != nil {
		return s.summarize(run.OutcomeViolated,
			"bob não recuperou o segredo do ledger"), err
	}
	s.rec.Note(6, "secret-from-ledger", "network2", revealed)
	if revealed != cfg.Secret {
		return s.summarize(run.OutcomeError,
				fmt.Sprintf("segredo lido (%q) difere do original", revealed)),
			errors.New("segredo divergente")
	}

	_, err = s.rec.Time(6, "claim-bond", "network1", func() (string, error) {
		return am.ClaimAssetInHTLC(
			s.n1Bob.Contract,
			cfg.BondType, cfg.BondID,
			s.n1Alice.CertB64,
			base64.StdEncoding.EncodeToString([]byte(revealed)),
		)
	})
	if err != nil {
		// Caso mais grave: alice levou os tokens e bob perdeu o bond.
		return s.summarize(run.OutcomeViolated,
			"alice recebeu os tokens mas bob falhou ao resgatar o bond"), err
	}
	s.transactions++
	s.unlockedAt = time.Now()

	return s.summarize(run.OutcomeCommittedBoth, "troca completa"), nil
}

// readRevealedSecret lê a pré-imagem publicada no ledger da rede 2.
func (s *Swap) readRevealedSecret() (string, error) {
	raw, err := s.n2Bob.Contract.EvaluateTransaction(
		"GetHTLCHashPreImageByContractId", s.tokenLockID)
	if err != nil {
		return "", fmt.Errorf("não foi possível ler a pré-imagem: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.Trim(string(raw), `"`))
	if err != nil {
		return "", fmt.Errorf("pré-imagem não é base64 válido: %w", err)
	}
	return string(decoded), nil
}

// ReclaimBond devolve o bond a alice após a expiração de T1.
//
// No Fabric nada dispara sozinho: sem um cliente externo invocando isto, o
// ativo fica preso mesmo com o prazo vencido. É o "watchdog" da vítima.
func (s *Swap) ReclaimBond() error {
	_, err := s.rec.Time(7, "reclaim-bond", "network1", func() (string, error) {
		return am.ReclaimAssetInHTLC(
			s.n1Alice.Contract,
			s.cfg.BondType, s.cfg.BondID,
			s.n1Bob.CertB64,
		)
	})
	if err == nil {
		s.transactions++
		s.unlockedAt = time.Now()
	}
	return err
}

// ReclaimTokens devolve os tokens a bob após a expiração de T2.
func (s *Swap) ReclaimTokens() error {
	if s.tokenLockID == "" {
		return errors.New("nenhum lock fungível registrado nesta execução")
	}
	_, err := s.rec.Time(8, "reclaim-tokens", "network2", func() (string, error) {
		return am.ReclaimFungibleAssetInHTLC(s.n2Bob.Contract, s.tokenLockID)
	})
	if err == nil {
		s.transactions++
	}
	return err
}

// Summarize recalcula as métricas agregadas no instante da chamada.
//
// Precisa ser chamado DEPOIS de eventuais resgates: o tempo de bloqueio só
// termina quando o ativo volta ao dono, e o resgate é uma escrita a mais no
// custo. Calcular durante Run() subestimaria as duas coisas.
func (s *Swap) Summarize(o run.Outcome, detail string) run.Summary {
	return s.summarize(o, detail)
}

func (s *Swap) summarize(o run.Outcome, detail string) run.Summary {
	lock := time.Duration(0)
	if !s.lockedAt.IsZero() {
		end := s.unlockedAt
		if end.IsZero() {
			end = time.Now()
		}
		lock = end.Sub(s.lockedAt)
	}
	return run.Summary{
		Outcome:      o,
		LockDuration: lock,
		Transactions: s.transactions,
		Detail:       detail,
	}
}

func hashOf(secret string) string {
	return am.GenerateSHA256HashInBase64Form(secret)
}

// SeedBond cria um bond pertencente a alice.
//
// Existe para o harness: cada execução precisa de um ativo virgem, e recriar
// as duas redes entre execuções levaria ~3 minutos.
//
// Dois detalhes que o simpleasset impõe:
//   - o campo owner é gravado LITERALMENTE, e todo o resto do chaincode o trata
//     como certificado. Passar a string "alice" cria um ativo que ninguém
//     consegue ler, nem alice.
//   - a data de vencimento precisa estar no futuro e no formato RFC822.
func (s *Swap) SeedBond(id string) error {
	maturity := time.Now().AddDate(5, 0, 0).Format(time.RFC822)
	_, err := s.n1Alice.Contract.SubmitTransaction(
		"CreateAsset", s.cfg.BondType, id, s.n1Alice.CertB64, "treasury", "500", maturity)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil // idempotente: reexecutar o runner não deve falhar
		}
		return fmt.Errorf("não foi possível criar %s:%s: %w", s.cfg.BondType, id, err)
	}
	return nil
}
