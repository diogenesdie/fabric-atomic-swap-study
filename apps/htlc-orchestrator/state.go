package main

// Leitura do estado dos dois ledgers, para classificar o desfecho de uma
// execução. É a parte que transforma "o protocolo rodou" em "o protocolo
// preservou atomicidade".

import (
	"encoding/json"
	"fmt"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/ledger"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/run"
	"regexp"
	"strconv"
	"strings"
)

// LedgerState é uma fotografia dos dois ledgers.
type LedgerState struct {
	// BondOwner é o nome do participante que detém o bond na rede 1, ou
	// "AUSENTE" se nenhuma identidade conhecida consegue lê-lo.
	BondOwner string
	// BondLocked indica se há lock HTLC ativo sobre o bond.
	BondLocked bool
	// TokenBalances mapeia participante -> saldo na rede 2.
	TokenBalances map[string]uint64
}

// asset é a projeção do ativo devolvida pelo chaincode simpleasset.
type asset struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Owner string `json:"owner"` // certificado X.509 do dono, em base64
}

var walletEntryRe = regexp.MustCompile(`([A-Za-z0-9_.-]+)="(\d+)"`)

// ReadState fotografa os dois ledgers.
//
// Duas particularidades do simpleasset guiam esta implementação:
//
//  1. ReadAsset só responde ao dono ("cannot access Bond Asset ..."), então não
//     existe identidade neutra de observação: é preciso tentar cada candidata.
//  2. O campo owner é o certificado do dono, não o nome. Comparamos com os
//     certificados das sessões — exato, e dispensa decodificar X.509.
func (s *Swap) ReadState() (LedgerState, error) {
	st := LedgerState{
		BondOwner:     "AUSENTE",
		TokenBalances: map[string]uint64{},
	}

	// --- rede 1: quem detém o bond ---
	// Guardamos os erros: se TODAS as candidatas falharem, provavelmente é
	// problema de conexão, não de titularidade — e silenciar isso já custou
	// caro uma vez (ver docs/notas/fabric.md).
	var readErrs []string
	candidates := []*ledger.Session{s.n1Alice, s.n1Bob}
	for _, sess := range candidates {
		raw, err := sess.Contract.EvaluateTransaction(
			"ReadAsset", s.cfg.BondType, s.cfg.BondID)
		if err != nil {
			// Erro esperado quando esta identidade não é a dona.
			readErrs = append(readErrs,
				fmt.Sprintf("%s: %s", sess.User, run.Truncate(err.Error(), 120)))
			continue
		}
		var a asset
		if err := json.Unmarshal(raw, &a); err != nil {
			return st, fmt.Errorf("ativo devolvido não é JSON válido: %w", err)
		}
		st.BondOwner = s.nameOfCert(a.Owner)
		readErrs = nil
		break
	}
	if len(readErrs) == len(candidates) {
		return st, fmt.Errorf(
			"nenhuma identidade conseguiu ler %s:%s — %s",
			s.cfg.BondType, s.cfg.BondID, strings.Join(readErrs, " | "))
	}

	// --- rede 1: há lock ativo sobre o bond? ---
	// IsAssetLockedInHTLC devolve erro quando não há lock, o que aqui é
	// informação e não falha.
	if locked, err := s.n1Bob.Contract.EvaluateTransaction(
		"IsAssetLocked", s.cfg.BondType, s.cfg.BondID); err == nil {
		st.BondLocked = strings.Contains(string(locked), "true")
	}

	// --- rede 2: saldos de token ---
	for _, sess := range []*ledger.Session{s.n2Alice, s.n2Bob} {
		raw, err := sess.Contract.EvaluateTransaction("GetMyWallet")
		if err != nil {
			// "owner does not have a wallet" quando o participante nunca
			// recebeu tokens: saldo zero, não erro.
			st.TokenBalances[sess.User] = 0
			continue
		}
		st.TokenBalances[sess.User] = parseWalletBalance(string(raw), s.cfg.TokenType)
	}

	return st, nil
}

// nameOfCert resolve um certificado base64 para o nome do participante,
// comparando com os certificados conhecidos das sessões.
func (s *Swap) nameOfCert(certB64 string) string {
	switch certB64 {
	case s.n1Alice.CertB64, s.n2Alice.CertB64:
		return "alice"
	case s.n1Bob.CertB64, s.n2Bob.CertB64:
		return "bob"
	case "":
		return "AUSENTE"
	default:
		return "DESCONHECIDO"
	}
}

// parseWalletBalance extrai o saldo de um tipo de token da saída de
// GetMyWallet, que vem no formato `token1="10000"` (uma entrada por linha).
//
// Um grep ingênuo por dígitos casaria o "1" de "token1"; daí a regex explícita
// sobre o valor entre aspas.
func parseWalletBalance(wallet, tokenType string) uint64 {
	for _, m := range walletEntryRe.FindAllStringSubmatch(wallet, -1) {
		if m[1] != tokenType {
			continue
		}
		n, err := strconv.ParseUint(m[2], 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// Classify compara o estado antes e depois e devolve o desfecho observado.
//
// A tabela de decisão é a do experimento:
//   - bond mudou de dono E tokens se moveram  -> COMMITTED_BOTH
//   - nada mudou e nada está travado          -> ABORTED_BOTH
//   - só um dos lados mudou                   -> VIOLATED (falha de segurança)
//   - nada mudou mas algo segue travado       -> BLOCKED (falha de vivacidade)
func Classify(before, after LedgerState, expectedQty uint64) (run.Outcome, string) {
	bondMoved := before.BondOwner != after.BondOwner && after.BondOwner == "bob"

	aliceGained := after.TokenBalances["alice"] >= before.TokenBalances["alice"]+expectedQty
	bobPaid := before.TokenBalances["bob"] >= after.TokenBalances["bob"]+expectedQty
	tokensMoved := aliceGained && bobPaid

	switch {
	case bondMoved && tokensMoved:
		return run.OutcomeCommittedBoth, "bond transferido e tokens movidos"

	case bondMoved && !tokensMoved:
		return run.OutcomeViolated,
			"bond foi transferido mas os tokens não se moveram"

	case !bondMoved && tokensMoved:
		return run.OutcomeViolated,
			"tokens se moveram mas o bond não foi transferido"

	case after.BondLocked:
		return run.OutcomeBlocked,
			"nenhum lado efetivou e o bond continua travado"

	default:
		return run.OutcomeAbortedBoth, "nenhum lado efetivou; nada permanece travado"
	}
}

// String descreve o estado de forma legível no terminal.
func (st LedgerState) String() string {
	return fmt.Sprintf("bond=%s locked=%t alice=%d bob=%d",
		st.BondOwner, st.BondLocked,
		st.TokenBalances["alice"], st.TokenBalances["bob"])
}
