package main

// Leitura do estado dos dois ledgers e classificação do desfecho.
//
// A tabela de classificação é a mesma do braço HTLC — precisa ser, senão os
// dois braços não são comparáveis. O que muda é como o estado é lido: aqui o
// chaincode twopc expõe o dono em claro e permite leitura por qualquer
// identidade, então não é preciso adivinhar quem consegue enxergar o quê.

import (
	"encoding/json"
	"fmt"

	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/ledger"
	"github.com/pucrs-ppgcc/htlc-vs-2pc/internal/run"
)

// Asset espelha a projeção devolvida pelo chaincode twopc.
type Asset struct {
	ID         string `json:"id"`
	Owner      string `json:"owner"`
	Value      int    `json:"value"`
	EscrowTxID string `json:"escrowTxId"`
}

// Side é o estado de um participante.
type Side struct {
	Network string
	Asset   Asset
	Found   bool
}

// HeldBy diz se o ativo está preso em custódia DESTA transação.
//
// A distinção importa: um ativo preso por outra transação não é bloqueio
// causado por esta execução, e contá-lo inflaria a taxa de BLOCKED.
func (s Side) HeldBy(txID string) bool { return s.Asset.EscrowTxID == txID && txID != "" }

// LedgerState é uma fotografia dos dois lados.
type LedgerState struct {
	N1, N2 Side
}

// String descreve o estado para o terminal.
func (st LedgerState) String() string {
	return fmt.Sprintf("n1[%s dono=%s custódia=%q]  n2[%s dono=%s custódia=%q]",
		st.N1.Asset.ID, st.N1.Asset.Owner, st.N1.Asset.EscrowTxID,
		st.N2.Asset.ID, st.N2.Asset.Owner, st.N2.Asset.EscrowTxID)
}

// ReadState fotografa os dois ledgers.
func (c *Coordinator) ReadState() (LedgerState, error) {
	var st LedgerState

	read := func(sess *ledger.Session, network, assetID string) (Side, error) {
		side := Side{Network: network}
		raw, err := sess.Contract.EvaluateTransaction("ReadAsset", assetID)
		if err != nil {
			// Ativo inexistente é informação, não falha da leitura.
			return side, nil
		}
		if err := json.Unmarshal(raw, &side.Asset); err != nil {
			return side, fmt.Errorf("ativo %s em %s não é JSON válido: %w",
				assetID, network, err)
		}
		side.Found = true
		return side, nil
	}

	var err error
	if st.N1, err = read(c.obs1, "network1", c.cfg.Asset1); err != nil {
		return st, err
	}
	if st.N2, err = read(c.obs2, "network2", c.cfg.Asset2); err != nil {
		return st, err
	}
	return st, nil
}

// Classify compara os estados antes e depois e devolve o desfecho.
//
// Mesma tabela do braço HTLC:
//   - ambos mudaram de dono           -> COMMITTED_BOTH
//   - nenhum mudou e nada em custódia -> ABORTED_BOTH
//   - só um mudou                     -> VIOLATED  (falha de segurança)
//   - nada mudou mas há custódia      -> BLOCKED   (falha de vivacidade)
//
// BLOCKED é o desfecho característico deste braço: é o que aparece quando o
// coordenador morre entre as fases e ninguém liberta os ativos.
func Classify(before, after LedgerState, txID string) (run.Outcome, string) {
	moved1 := before.N1.Asset.Owner != after.N1.Asset.Owner
	moved2 := before.N2.Asset.Owner != after.N2.Asset.Owner
	held := after.N1.HeldBy(txID) || after.N2.HeldBy(txID)

	switch {
	case moved1 && moved2:
		return run.OutcomeCommittedBoth, "os dois ativos trocaram de dono"

	case moved1 != moved2:
		which := "rede 1"
		if moved2 {
			which = "rede 2"
		}
		return run.OutcomeViolated,
			fmt.Sprintf("só o ativo da %s trocou de dono", which)

	case held:
		var where []string
		if after.N1.HeldBy(txID) {
			where = append(where, "rede 1")
		}
		if after.N2.HeldBy(txID) {
			where = append(where, "rede 2")
		}
		return run.OutcomeBlocked,
			fmt.Sprintf("nenhum lado efetivou e há ativo preso em custódia (%v)", where)

	default:
		return run.OutcomeAbortedBoth, "nenhum lado efetivou; nada em custódia"
	}
}
