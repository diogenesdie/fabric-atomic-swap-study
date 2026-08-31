package main

// Write-ahead log do coordenador.
//
// É a peça mais importante deste braço do experimento. O 2PC clássico
// (Gray 1978) exige que o coordenador PERSISTA A DECISÃO ANTES de comunicá-la:
// se ele cai depois de mandar um Commit e não sabia que havia decidido
// efetivar, ao reiniciar poderia abortar — e um participante já teria
// efetivado. Atomicidade violada por amnésia do coordenador.
//
// Persistir antes é o que torna o cenário T2 interessante em vez de trivial:
// o coordenador cai entre as fases, os participantes ficam em incerteza com os
// ativos presos, e a recuperação consegue retomar a decisão exata que havia
// sido tomada.
//
// Formato: um arquivo JSON por transação, reescrito a cada avanço de fase.
// Simples de inspecionar durante o experimento, e a durabilidade é garantida
// por fsync — sem ele, `write` retorna com os bytes ainda no cache do sistema
// operacional, e um SIGKILL não os perderia, mas uma queda de energia sim.
// Como matamos o processo com SIGKILL, o cache sobrevive; mesmo assim usamos
// fsync, porque é o que um coordenador correto faria e o custo entra na
// medição de latência.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Phase é o ponto do protocolo registrado de forma durável.
type Phase string

const (
	// PhaseStarted: a transação começou; nenhum voto coletado ainda.
	// Recuperar daqui significa abortar — ninguém prometeu nada.
	PhaseStarted Phase = "STARTED"
	// PhaseVoted: todos votaram sim. O coordenador ainda NÃO decidiu.
	// Recuperar daqui permite decidir livremente.
	PhaseVoted Phase = "VOTED"
	// PhaseCommitDecided: a decisão é efetivar, e está em disco. A partir
	// daqui a recuperação é obrigada a efetivar, nunca abortar.
	PhaseCommitDecided Phase = "COMMIT_DECIDED"
	// PhaseAbortDecided: a decisão é abortar, e está em disco.
	PhaseAbortDecided Phase = "ABORT_DECIDED"
	// PhaseDone: todos os participantes confirmaram a decisão.
	PhaseDone Phase = "DONE"
)

// Participant identifica um lado da transação.
type Participant struct {
	Network  string `json:"network"`
	AssetID  string `json:"assetId"`
	Owner    string `json:"owner"`
	NewOwner string `json:"newOwner"`
	// Vote é o voto da fase 1: YES, NO ou vazio se ainda não votou.
	Vote string `json:"vote"`
	// Acked registra que este participante não precisa mais de nada na fase 2 —
	// ou porque confirmou a decisão, ou porque votou NÃO e não tem o que
	// desfazer.
	Acked bool `json:"acked"`
}

// Record é o estado durável de uma transação 2PC.
type Record struct {
	TxID         string        `json:"txId"`
	Phase        Phase         `json:"phase"`
	Participants []Participant `json:"participants"`
	Deadline     int64         `json:"deadline"`
	StartedAt    time.Time     `json:"startedAt"`
	DecidedAt    time.Time     `json:"decidedAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

// Committed diz se a decisão registrada é efetivar.
func (r Record) Committed() bool { return r.Phase == PhaseCommitDecided }

// Decided diz se já existe decisão durável.
func (r Record) Decided() bool {
	return r.Phase == PhaseCommitDecided || r.Phase == PhaseAbortDecided
}

// WAL é o log em disco.
type WAL struct {
	dir string
}

// OpenWAL abre (criando se preciso) o diretório do log.
func OpenWAL(dir string) (*WAL, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("não foi possível criar o diretório do WAL %s: %w", dir, err)
	}
	return &WAL{dir: dir}, nil
}

func (w *WAL) path(txID string) string {
	// O txID entra em nome de arquivo; barras o quebrariam.
	safe := strings.NewReplacer("/", "_", string(os.PathSeparator), "_").Replace(txID)
	return filepath.Join(w.dir, safe+".json")
}

// Put grava o registro de forma durável.
//
// Escreve num temporário, faz fsync do arquivo, renomeia sobre o definitivo e
// faz fsync do diretório. A troca por rename é atômica no POSIX, então nunca
// existe um registro pela metade: ou o anterior, ou o novo.
func (w *WAL) Put(rec Record) error {
	rec.UpdatedAt = time.Now()

	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("não foi possível serializar o registro de %s: %w", rec.TxID, err)
	}

	final := w.path(rec.TxID)
	tmp := final + ".tmp"

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("não foi possível abrir %s: %w", tmp, err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return fmt.Errorf("falha ao escrever %s: %w", tmp, err)
	}
	// O ponto de todo o exercício: sem este fsync, "gravado" é só uma promessa
	// do cache do sistema operacional.
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("falha no fsync de %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("falha ao fechar %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("falha ao renomear para %s: %w", final, err)
	}

	// fsync do diretório para que o próprio rename seja durável.
	if d, err := os.Open(w.dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// Get lê o registro de uma transação.
func (w *WAL) Get(txID string) (Record, error) {
	var rec Record
	raw, err := os.ReadFile(w.path(txID))
	if err != nil {
		return rec, fmt.Errorf("não há registro de %s no WAL: %w", txID, err)
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return rec, fmt.Errorf("registro de %s corrompido: %w", txID, err)
	}
	return rec, nil
}

// Pending lista as transações que não chegaram a DONE, da mais antiga para a
// mais recente. É o que a recuperação precisa retomar.
func (w *WAL) Pending() ([]Record, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler o WAL em %s: %w", w.dir, err)
	}

	var pending []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(w.dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var rec Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			// Um registro ilegível é grave: pode esconder uma decisão tomada.
			return nil, fmt.Errorf("registro %s corrompido: %w", e.Name(), err)
		}
		if rec.Phase != PhaseDone {
			pending = append(pending, rec)
		}
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].StartedAt.Before(pending[j].StartedAt)
	})
	return pending, nil
}
