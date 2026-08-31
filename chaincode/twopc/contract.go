// Package main implementa o lado on-chain de um commit atômico em duas fases
// (2PC) entre duas redes Hyperledger Fabric.
//
// O contrato é o RECURSO GERENCIADO, não o coordenador. Ele sabe apenas
// prometer (Prepare), cumprir (Commit) e desistir (Abort) — quem decide é um
// cliente externo. Essa separação não é escolha de estilo: no Fabric,
// InvokeChaincode entre canais é somente leitura, então não existe transação
// que abranja as duas cadeias e o coordenador precisa ser externo.
//
// A nomenclatura segue o IETF SATP (draft-ietf-satp-core), cujo Stage 3 é
// literalmente um 2PC entre gateways: o voto do participante é uma
// LockAssertion, a efetivação é CommitFinal e a desistência é Rollback.
// Alinhar o vocabulário custa zero e evita que a comparação seja lida como um
// 2PC de fabricação própria, escolhido para perder.
//
// Diferenças deliberadas em relação ao simpleasset do Weaver:
//
//   - o dono é o NOME do participante, não o certificado em base64. A posse é
//     verificada contra o enrollment ID de quem submete, então a segurança é a
//     mesma, mas o estado fica legível e a classificação do experimento não
//     precisa decodificar X.509.
//   - a leitura é livre. No simpleasset, ReadAsset só responde ao dono, o que
//     torna impossível observar o ledger de fora sem tentar cada identidade.
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// TxState é o estado de uma transação 2PC neste participante.
type TxState string

const (
	// StatePrepared: o participante votou sim e está EM INCERTEZA. O ativo
	// está em custódia e ele não pode mais decidir sozinho — só esperar. É
	// exatamente esta janela que o experimento mede quando o coordenador cai.
	StatePrepared TxState = "PREPARED"
	// StateCommitted: efetivado, a titularidade mudou.
	StateCommitted TxState = "COMMITTED"
	// StateAborted: desfeito, o ativo voltou ao dono original.
	StateAborted TxState = "ABORTED"
)

// Prefixos de chave no estado mundial.
const (
	assetKeyPrefix = "asset"
	txKeyPrefix    = "tx2pc"
)

// Asset é o recurso disputado.
type Asset struct {
	ID    string `json:"id"`
	Owner string `json:"owner"`
	Value int    `json:"value"`
	// EscrowTxID é a transação 2PC que mantém o ativo em custódia. Vazio
	// significa livre. Um ativo em custódia não aceita novo Prepare — é o
	// mecanismo que faz o participante votar não.
	EscrowTxID string `json:"escrowTxId"`
}

// TxRecord é o registro durável da participação nesta transação.
//
// Equivale ao log de preparo do 2PC clássico (Gray 1978): sem ele, um
// participante que reinicia não sabe que prometeu, e a promessa se perde.
type TxRecord struct {
	TxID string `json:"txId"`
	// AssetID e PreviousOwner guardam o que desfazer num Rollback.
	AssetID       string  `json:"assetId"`
	PreviousOwner string  `json:"previousOwner"`
	NewOwner      string  `json:"newOwner"`
	State         TxState `json:"state"`
	// Deadline é o instante (Unix) a partir do qual o participante pode
	// abortar unilateralmente. É a válvula de escape contra o bloqueio.
	Deadline   int64 `json:"deadline"`
	PreparedAt int64 `json:"preparedAt"`
	// Sem omitempty: o fabric-contract-api-go gera um schema em que todos os
	// campos são obrigatórios e valida a resposta contra ele. Um zero omitido
	// faz a chamada falhar com "Value did not match schema" — que não diz qual
	// campo faltou. Zero aqui significa "ainda não decidida".
	DecidedAt int64 `json:"decidedAt"`
}

// SmartContract é o contrato 2PC.
type SmartContract struct {
	contractapi.Contract
}

// ------------------------------------------------------------------ chaves

func assetKey(ctx contractapi.TransactionContextInterface, id string) (string, error) {
	return ctx.GetStub().CreateCompositeKey(assetKeyPrefix, []string{id})
}

func txKey(ctx contractapi.TransactionContextInterface, txID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey(txKeyPrefix, []string{txID})
}

// ------------------------------------------------------------------ tempo

// txTime devolve o instante da transação segundo o timestamp da proposta.
//
// time.Now() é proibido em chaincode: cada peer endossante executaria num
// momento diferente e produziria write sets divergentes, quebrando a política
// de endosso. GetTxTimestamp() é o mesmo valor para todos os endossantes.
func txTime(ctx contractapi.TransactionContextInterface) (int64, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return 0, fmt.Errorf("não foi possível ler o timestamp da transação: %w", err)
	}
	return ts.AsTime().Unix(), nil
}

// caller devolve o enrollment ID de quem submeteu a transação.
func caller(ctx contractapi.TransactionContextInterface) (string, error) {
	id, ok, err := ctx.GetClientIdentity().GetAttributeValue("hf.EnrollmentID")
	if err != nil {
		return "", fmt.Errorf("não foi possível ler a identidade do cliente: %w", err)
	}
	if !ok || id == "" {
		return "", fmt.Errorf("certificado do cliente não traz hf.EnrollmentID")
	}
	return id, nil
}

// ------------------------------------------------------------ ciclo de vida

// InitLedger é chamado no deploy (--isInit). O testbed do Weaver invoca
// initLedger com um único argumento vazio para chaincodes que não conhece.
func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface, _ string) error {
	return nil
}

// ----------------------------------------------------------------- ativos

// CreateAsset registra um novo ativo livre.
func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface,
	id, owner string, value int) error {

	if id == "" || owner == "" {
		return fmt.Errorf("id e owner são obrigatórios")
	}
	key, err := assetKey(ctx, id)
	if err != nil {
		return err
	}
	existing, err := ctx.GetStub().GetState(key)
	if err != nil {
		return fmt.Errorf("falha ao ler o estado de %s: %w", id, err)
	}
	if existing != nil {
		return fmt.Errorf("o ativo %s já existe", id)
	}

	return putJSON(ctx, key, Asset{ID: id, Owner: owner, Value: value})
}

// ReadAsset devolve um ativo. Sem restrição de leitura: o observador do
// experimento precisa enxergar o ledger sem se passar pelo dono.
func (s *SmartContract) ReadAsset(ctx contractapi.TransactionContextInterface,
	id string) (*Asset, error) {

	key, err := assetKey(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler o ativo %s: %w", id, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("o ativo %s não existe", id)
	}
	var a Asset
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("ativo %s corrompido no estado: %w", id, err)
	}
	return &a, nil
}

// GetAllAssets lista todos os ativos.
func (s *SmartContract) GetAllAssets(ctx contractapi.TransactionContextInterface) ([]*Asset, error) {
	it, err := ctx.GetStub().GetStateByPartialCompositeKey(assetKeyPrefix, []string{})
	if err != nil {
		return nil, fmt.Errorf("falha ao varrer os ativos: %w", err)
	}
	defer it.Close()

	assets := []*Asset{}
	for it.HasNext() {
		kv, err := it.Next()
		if err != nil {
			return nil, err
		}
		var a Asset
		if err := json.Unmarshal(kv.Value, &a); err != nil {
			return nil, fmt.Errorf("ativo corrompido no estado: %w", err)
		}
		assets = append(assets, &a)
	}
	return assets, nil
}

// ------------------------------------------------------------------ fase 1

// Prepare é o voto do participante — a LockAssertion do SATP.
//
// Ao retornar sem erro, o participante PROMETEU: o ativo entra em custódia e
// ele perde o direito de decidir sozinho até a fase 2 ou até o prazo expirar.
// Um erro aqui é um voto NÃO, e o coordenador deve abortar a transação inteira.
//
// Recusa se: o ativo não existe, quem submete não é o dono, ou o ativo já está
// em custódia de outra transação. O último caso é o que serializa transações
// concorrentes sobre o mesmo recurso.
func (s *SmartContract) Prepare(ctx contractapi.TransactionContextInterface,
	txID, assetID, newOwner string, deadline int64) error {

	if txID == "" || assetID == "" || newOwner == "" {
		return fmt.Errorf("txId, assetId e newOwner são obrigatórios")
	}

	tKey, err := txKey(ctx, txID)
	if err != nil {
		return err
	}
	if raw, err := ctx.GetStub().GetState(tKey); err != nil {
		return fmt.Errorf("falha ao ler o registro de %s: %w", txID, err)
	} else if raw != nil {
		// Reenvio do coordenador após uma resposta perdida: o voto já foi
		// dado e é o mesmo. Idempotente, não é erro.
		var rec TxRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return fmt.Errorf("registro de %s corrompido: %w", txID, err)
		}
		if rec.State == StatePrepared {
			return nil
		}
		return fmt.Errorf("a transação %s já foi decidida como %s", txID, rec.State)
	}

	asset, err := s.ReadAsset(ctx, assetID)
	if err != nil {
		return err
	}

	who, err := caller(ctx)
	if err != nil {
		return err
	}
	if asset.Owner != who {
		return fmt.Errorf("apenas o dono pode oferecer %s: dono é %s, quem submete é %s",
			assetID, asset.Owner, who)
	}
	if asset.EscrowTxID != "" {
		return fmt.Errorf("o ativo %s já está em custódia da transação %s",
			assetID, asset.EscrowTxID)
	}

	now, err := txTime(ctx)
	if err != nil {
		return err
	}
	if deadline <= now {
		return fmt.Errorf("o prazo %d já passou (agora é %d)", deadline, now)
	}

	// Ordem importa: o registro durável primeiro, a custódia depois. Ambos
	// entram no mesmo write set, então commitam juntos ou nenhum commita.
	rec := TxRecord{
		TxID:          txID,
		AssetID:       assetID,
		PreviousOwner: asset.Owner,
		NewOwner:      newOwner,
		State:         StatePrepared,
		Deadline:      deadline,
		PreparedAt:    now,
	}
	if err := putJSON(ctx, tKey, rec); err != nil {
		return err
	}

	asset.EscrowTxID = txID
	aKey, err := assetKey(ctx, assetID)
	if err != nil {
		return err
	}
	if err := putJSON(ctx, aKey, *asset); err != nil {
		return err
	}

	return emit(ctx, "LockAssertion", rec)
}

// ------------------------------------------------------------------ fase 2

// Commit efetiva a transferência — o CommitFinal do SATP.
//
// Idempotente: recuperação de coordenador implica reenvio, e reenviar um
// Commit já aplicado não pode falhar nem duplicar o efeito.
func (s *SmartContract) Commit(ctx contractapi.TransactionContextInterface, txID string) error {
	return s.decide(ctx, txID, StateCommitted, false)
}

// Abort desfaz a promessa — o Rollback do SATP. Idempotente.
func (s *SmartContract) Abort(ctx contractapi.TransactionContextInterface, txID string) error {
	return s.decide(ctx, txID, StateAborted, false)
}

// TimeoutAbort permite ao participante abortar unilateralmente após o prazo.
//
// É a válvula de escape contra o bloqueio do 2PC: sem ela, um coordenador que
// cai entre as fases deixa o ativo preso para sempre. Com ela, o participante
// recupera a autonomia — ao custo de reintroduzir uma hipótese temporal, que é
// justamente do que o 2PC pretendia escapar. Medir com e sem é um resultado por
// si só.
//
// ATENÇÃO: nada dispara isto sozinho. O Fabric não tem temporizadores; é
// preciso um cliente externo invocando. Um ativo com prazo vencido continua
// preso enquanto ninguém chamar.
func (s *SmartContract) TimeoutAbort(ctx contractapi.TransactionContextInterface, txID string) error {
	return s.decide(ctx, txID, StateAborted, true)
}

// decide aplica a decisão da fase 2. requireExpiry distingue o aborto
// unilateral por prazo do aborto ordenado pelo coordenador.
func (s *SmartContract) decide(ctx contractapi.TransactionContextInterface,
	txID string, target TxState, requireExpiry bool) error {

	if txID == "" {
		return fmt.Errorf("txId é obrigatório")
	}
	tKey, err := txKey(ctx, txID)
	if err != nil {
		return err
	}
	raw, err := ctx.GetStub().GetState(tKey)
	if err != nil {
		return fmt.Errorf("falha ao ler o registro de %s: %w", txID, err)
	}
	if raw == nil {
		return fmt.Errorf("a transação %s não foi preparada aqui", txID)
	}

	var rec TxRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return fmt.Errorf("registro de %s corrompido: %w", txID, err)
	}

	// Idempotência: repetir a mesma decisão é sucesso; contradizê-la, não.
	// Um participante nunca muda de decisão (propriedade AC2).
	if rec.State != StatePrepared {
		if rec.State == target {
			return nil
		}
		return fmt.Errorf("a transação %s já foi decidida como %s; não pode virar %s",
			txID, rec.State, target)
	}

	now, err := txTime(ctx)
	if err != nil {
		return err
	}
	if requireExpiry && now < rec.Deadline {
		return fmt.Errorf(
			"o prazo de %s ainda não venceu: faltam %ds — aguarde ou aborte pelo coordenador",
			txID, rec.Deadline-now)
	}

	asset, err := s.ReadAsset(ctx, rec.AssetID)
	if err != nil {
		return err
	}
	if asset.EscrowTxID != txID {
		return fmt.Errorf("o ativo %s não está em custódia de %s", rec.AssetID, txID)
	}

	if target == StateCommitted {
		asset.Owner = rec.NewOwner
	} else {
		asset.Owner = rec.PreviousOwner
	}
	asset.EscrowTxID = ""

	aKey, err := assetKey(ctx, rec.AssetID)
	if err != nil {
		return err
	}
	if err := putJSON(ctx, aKey, *asset); err != nil {
		return err
	}

	rec.State = target
	rec.DecidedAt = now
	if err := putJSON(ctx, tKey, rec); err != nil {
		return err
	}

	event := "CommitFinal"
	if target == StateAborted {
		event = "Rollback"
	}
	return emit(ctx, event, rec)
}

// --------------------------------------------------------------- consultas

// GetTxRecord devolve o registro de uma transação, ou erro se não existir.
func (s *SmartContract) GetTxRecord(ctx contractapi.TransactionContextInterface,
	txID string) (*TxRecord, error) {

	tKey, err := txKey(ctx, txID)
	if err != nil {
		return nil, err
	}
	raw, err := ctx.GetStub().GetState(tKey)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler o registro de %s: %w", txID, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("não há registro da transação %s", txID)
	}
	var rec TxRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("registro de %s corrompido: %w", txID, err)
	}
	return &rec, nil
}

// GetEscrowed lista os ativos presos em custódia.
//
// É a medida direta do bloqueio: numa execução em que o coordenador caiu entre
// as fases, o que sobra aqui é o capital imobilizado.
func (s *SmartContract) GetEscrowed(ctx contractapi.TransactionContextInterface) ([]*Asset, error) {
	all, err := s.GetAllAssets(ctx)
	if err != nil {
		return nil, err
	}
	escrowed := []*Asset{}
	for _, a := range all {
		if a.EscrowTxID != "" {
			escrowed = append(escrowed, a)
		}
	}
	return escrowed, nil
}

// GetPendingTxs lista as transações ainda em incerteza neste participante.
// Serve ao watchdog externo que exerce o TimeoutAbort.
func (s *SmartContract) GetPendingTxs(ctx contractapi.TransactionContextInterface) ([]*TxRecord, error) {
	it, err := ctx.GetStub().GetStateByPartialCompositeKey(txKeyPrefix, []string{})
	if err != nil {
		return nil, fmt.Errorf("falha ao varrer as transações: %w", err)
	}
	defer it.Close()

	pending := []*TxRecord{}
	for it.HasNext() {
		kv, err := it.Next()
		if err != nil {
			return nil, err
		}
		var rec TxRecord
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			return nil, fmt.Errorf("registro corrompido: %w", err)
		}
		if rec.State == StatePrepared {
			pending = append(pending, &rec)
		}
	}
	return pending, nil
}

// ---------------------------------------------------------------- auxiliar

func putJSON(ctx contractapi.TransactionContextInterface, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("falha ao serializar para %s: %w", key, err)
	}
	if err := ctx.GetStub().PutState(key, raw); err != nil {
		return fmt.Errorf("falha ao gravar %s: %w", key, err)
	}
	return nil
}

// emit publica um evento de chaincode. O coordenador não depende deles — usa
// as respostas das transações — mas eles dão uma trilha auditável do protocolo
// no ledger, útil na análise.
func emit(ctx contractapi.TransactionContextInterface, name string, rec TxRecord) error {
	payload, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("falha ao serializar o evento %s: %w", name, err)
	}
	if err := ctx.GetStub().SetEvent(name, payload); err != nil {
		return fmt.Errorf("falha ao emitir o evento %s: %w", name, err)
	}
	return nil
}

// Unix é auxiliar de teste e documentação: converte um time.Time no formato
// que Prepare espera em deadline.
func Unix(t time.Time) int64 { return t.Unix() }
