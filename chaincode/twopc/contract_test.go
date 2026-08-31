package main

// Testes da máquina de estados 2PC.
//
// Em vez de programar retornos fixos nos mocks, ligamos o stub a um mapa em
// memória. Assim os testes exercitam as transições de verdade — o que importa
// aqui é o comportamento do protocolo (idempotência, recusa de decisão
// contraditória, prazo), não se o código chama PutState.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	wmocks "github.com/hyperledger-cacti/cacti/weaver/core/network/fabric-interop-cc/libs/testutils/mocks"
)

const (
	alice = "alice"
	bob   = "bob"
)

// ledger é o estado mundial simulado, com o comportamento que o contrato usa.
type ledger struct {
	state map[string][]byte
	now   int64
	who   string
	// events guarda os eventos emitidos, para conferir a trilha do protocolo.
	events []string
}

// setup monta contexto e stub ligados a um ledger em memória.
func setup(t *testing.T) (*wmocks.TransactionContext, *ledger) {
	t.Helper()

	l := &ledger{
		state: map[string][]byte{},
		now:   time.Now().Unix(),
		who:   alice,
	}

	stub := &wmocks.ChaincodeStub{}
	ctx := &wmocks.TransactionContext{}
	ctx.GetStubReturns(stub)

	stub.GetStateStub = func(key string) ([]byte, error) {
		return l.state[key], nil
	}
	stub.PutStateStub = func(key string, value []byte) error {
		// Cópia: o contrato pode reusar o buffer depois.
		buf := make([]byte, len(value))
		copy(buf, value)
		l.state[key] = buf
		return nil
	}
	stub.DelStateStub = func(key string) error {
		delete(l.state, key)
		return nil
	}
	stub.CreateCompositeKeyStub = func(prefix string, attrs []string) (string, error) {
		return prefix + "\x00" + strings.Join(attrs, "\x00") + "\x00", nil
	}
	stub.GetTxTimestampStub = func() (*timestamppb.Timestamp, error) {
		return timestamppb.New(time.Unix(l.now, 0)), nil
	}
	stub.SetEventStub = func(name string, _ []byte) error {
		l.events = append(l.events, name)
		return nil
	}
	stub.GetStateByPartialCompositeKeyStub = func(prefix string, _ []string) (shim.StateQueryIteratorInterface, error) {
		return l.iterator(prefix), nil
	}

	identity := &wmocks.ClientIdentity{}
	identity.GetAttributeValueStub = func(attr string) (string, bool, error) {
		if attr == "hf.EnrollmentID" {
			return l.who, true, nil
		}
		return "", false, nil
	}
	ctx.GetClientIdentityReturns(identity)

	return ctx, l
}

// iterator devolve as entradas cujo prefixo de chave composta bate.
func (l *ledger) iterator(prefix string) shim.StateQueryIteratorInterface {
	type kv struct {
		key   string
		value []byte
	}
	var items []kv
	for k, v := range l.state {
		if strings.HasPrefix(k, prefix+"\x00") {
			items = append(items, kv{k, v})
		}
	}

	it := &wmocks.StateQueryIterator{}
	idx := 0
	it.HasNextStub = func() bool { return idx < len(items) }
	it.NextStub = func() (*queryresult.KV, error) {
		if idx >= len(items) {
			return nil, fmt.Errorf("iterador esgotado")
		}
		item := items[idx]
		idx++
		return &queryresult.KV{Key: item.key, Value: item.value}, nil
	}
	it.CloseStub = func() error { return nil }
	return it
}

// asset lê um ativo direto do estado simulado.
func (l *ledger) asset(t *testing.T, id string) Asset {
	t.Helper()
	raw, ok := l.state["asset\x00"+id+"\x00"]
	require.True(t, ok, "ativo %s não está no estado", id)
	var a Asset
	require.NoError(t, json.Unmarshal(raw, &a))
	return a
}

// prepared é o atalho para "ativo criado e transação preparada".
func prepared(t *testing.T, sc *SmartContract, ctx *wmocks.TransactionContext,
	l *ledger, txID string) {
	t.Helper()
	require.NoError(t, sc.CreateAsset(ctx, "a1", alice, 100))
	require.NoError(t, sc.Prepare(ctx, txID, "a1", bob, l.now+600))
}

// --------------------------------------------------------------- caminho ok

func TestPrepareThenCommitTransfersOwnership(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")

	// Depois do Prepare o ativo está em custódia, e o dono AINDA não mudou:
	// a promessa não é a efetivação.
	a := l.asset(t, "a1")
	require.Equal(t, alice, a.Owner, "o dono não deve mudar na fase 1")
	require.Equal(t, "tx1", a.EscrowTxID)

	rec, err := sc.GetTxRecord(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, StatePrepared, rec.State)

	require.NoError(t, sc.Commit(ctx, "tx1"))

	a = l.asset(t, "a1")
	require.Equal(t, bob, a.Owner, "o Commit deve transferir a titularidade")
	require.Empty(t, a.EscrowTxID, "o Commit deve liberar a custódia")

	rec, err = sc.GetTxRecord(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, StateCommitted, rec.State)

	require.Equal(t, []string{"LockAssertion", "CommitFinal"}, l.events)
}

func TestPrepareThenAbortRestoresOwner(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")
	require.NoError(t, sc.Abort(ctx, "tx1"))

	a := l.asset(t, "a1")
	require.Equal(t, alice, a.Owner, "o Abort deve devolver o ativo ao dono original")
	require.Empty(t, a.EscrowTxID)
	require.Equal(t, []string{"LockAssertion", "Rollback"}, l.events)
}

// ------------------------------------------------------- idempotência (AC2)

func TestDecisionsAreIdempotent(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")

	// A recuperação do coordenador reenvia a decisão; reenviar não pode
	// falhar nem duplicar o efeito.
	require.NoError(t, sc.Commit(ctx, "tx1"))
	require.NoError(t, sc.Commit(ctx, "tx1"), "repetir o Commit deve ser sucesso")
	require.NoError(t, sc.Commit(ctx, "tx1"))

	require.Equal(t, bob, l.asset(t, "a1").Owner)
}

func TestParticipantNeverReversesItsDecision(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")
	require.NoError(t, sc.Commit(ctx, "tx1"))

	// Propriedade AC2: um processo não muda de decisão.
	err := sc.Abort(ctx, "tx1")
	require.Error(t, err, "abortar depois de efetivar deve falhar")
	require.Contains(t, err.Error(), "já foi decidida")

	require.Equal(t, bob, l.asset(t, "a1").Owner, "o estado não deve mudar")
}

func TestRepeatedPrepareIsAcceptedWhileUncertain(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")
	// Reenvio do Prepare por resposta perdida: o voto já foi dado e é o mesmo.
	require.NoError(t, sc.Prepare(ctx, "tx1", "a1", bob, l.now+600))
}

// ------------------------------------------------------------- votos "não"

func TestPrepareRefusesAssetAlreadyEscrowed(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")

	// Segunda transação sobre o mesmo recurso: é o voto NÃO que serializa
	// transações concorrentes.
	err := sc.Prepare(ctx, "tx2", "a1", bob, l.now+600)
	require.Error(t, err)
	require.Contains(t, err.Error(), "já está em custódia")
}

func TestPrepareRefusesNonOwner(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	require.NoError(t, sc.CreateAsset(ctx, "a1", alice, 100))

	l.who = bob // bob tenta oferecer o ativo de alice
	err := sc.Prepare(ctx, "tx1", "a1", bob, l.now+600)
	require.Error(t, err)
	require.Contains(t, err.Error(), "apenas o dono")
}

func TestPrepareRefusesExpiredDeadline(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	require.NoError(t, sc.CreateAsset(ctx, "a1", alice, 100))

	err := sc.Prepare(ctx, "tx1", "a1", bob, l.now-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "prazo")
}

func TestPrepareRefusesUnknownAsset(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	err := sc.Prepare(ctx, "tx1", "inexistente", bob, l.now+600)
	require.Error(t, err)
}

// --------------------------------------------------- a válvula de escape

func TestTimeoutAbortRefusedBeforeDeadline(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")

	// Antes do prazo o participante segue em incerteza: é exatamente o
	// bloqueio do 2PC, e não pode ser burlado.
	err := sc.TimeoutAbort(ctx, "tx1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ainda não venceu")
	require.Equal(t, "tx1", l.asset(t, "a1").EscrowTxID, "o ativo deve seguir preso")
}

func TestTimeoutAbortReleasesAssetAfterDeadline(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")

	l.now += 601 // o prazo vence
	require.NoError(t, sc.TimeoutAbort(ctx, "tx1"))

	a := l.asset(t, "a1")
	require.Equal(t, alice, a.Owner)
	require.Empty(t, a.EscrowTxID)
}

func TestCommitRefusedAfterTimeoutAbort(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	prepared(t, sc, ctx, l, "tx1")
	l.now += 601
	require.NoError(t, sc.TimeoutAbort(ctx, "tx1"))

	// O coordenador acorda e tenta efetivar uma decisão que o participante já
	// abandonou. Precisa falhar: é a inconsistência que o TimeoutAbort
	// introduz, e o experimento tem de conseguir observá-la.
	err := sc.Commit(ctx, "tx1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "já foi decidida")
	require.Equal(t, alice, l.asset(t, "a1").Owner)
}

// -------------------------------------------------------------- consultas

func TestGetEscrowedListsOnlyHeldAssets(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	require.NoError(t, sc.CreateAsset(ctx, "a1", alice, 100))
	require.NoError(t, sc.CreateAsset(ctx, "a2", alice, 200))
	require.NoError(t, sc.Prepare(ctx, "tx1", "a1", bob, l.now+600))

	escrowed, err := sc.GetEscrowed(ctx)
	require.NoError(t, err)
	require.Len(t, escrowed, 1)
	require.Equal(t, "a1", escrowed[0].ID)
}

func TestGetPendingTxsListsOnlyUncertain(t *testing.T) {
	ctx, l := setup(t)
	sc := &SmartContract{}

	require.NoError(t, sc.CreateAsset(ctx, "a1", alice, 100))
	require.NoError(t, sc.CreateAsset(ctx, "a2", alice, 200))
	require.NoError(t, sc.Prepare(ctx, "tx1", "a1", bob, l.now+600))
	require.NoError(t, sc.Prepare(ctx, "tx2", "a2", bob, l.now+600))
	require.NoError(t, sc.Commit(ctx, "tx1"))

	pending, err := sc.GetPendingTxs(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1, "só tx2 segue em incerteza")
	require.Equal(t, "tx2", pending[0].TxID)
}

func TestDecideRefusesUnknownTransaction(t *testing.T) {
	ctx, _ := setup(t)
	sc := &SmartContract{}

	err := sc.Commit(ctx, "nunca-preparada")
	require.Error(t, err)
	require.Contains(t, err.Error(), "não foi preparada")
}

func TestCreateAssetRejectsDuplicate(t *testing.T) {
	ctx, _ := setup(t)
	sc := &SmartContract{}

	require.NoError(t, sc.CreateAsset(ctx, "a1", alice, 100))
	err := sc.CreateAsset(ctx, "a1", alice, 100)
	require.Error(t, err)
	require.Contains(t, err.Error(), "já existe")
}
