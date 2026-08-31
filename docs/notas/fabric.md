# Hyperledger Fabric — o que importa para este trabalho

Notas sobre a plataforma, com foco no que afeta o desenho dos dois protocolos.
Pegadinhas encontradas durante a execução vão sendo anotadas na seção final.

## Arquitetura em uma passada

Fabric é *execute-order-validate*, ao contrário do *order-execute* do
Bitcoin/Ethereum:

1. **Execute** — o cliente envia uma *proposta* aos peers endossantes. Cada um
   simula o chaincode contra seu estado atual e devolve um **read/write set**
   assinado. Nada foi gravado ainda.
2. **Order** — o cliente junta os endossos e envia ao *ordering service*, que
   define a ordem total e monta blocos.
3. **Validate** — cada peer valida a política de endosso e checa se as chaves
   lidas mudaram desde a simulação. Se mudaram, a transação é marcada
   **inválida** (`MVCC_READ_CONFLICT`) e o write set é descartado.

Consequência que atravessa todo o projeto: **uma transação pode ser "aceita" na
ordenação e ainda assim falhar na validação**. Sucesso é confirmação de commit
válido no bloco, não retorno OK do endosso.

## Três pegadinhas que moldam o desenho

### 1. Invocação cross-channel é somente leitura

`stub.InvokeChaincode` com canal de destino diferente executa **como query**: o
resultado não entra no write set e não é validado no commit. Não existe
transação atômica abrangendo dois canais — que é, no fundo, a razão de este
trabalho existir.

**Efeito:** o coordenador 2PC **tem de ser um cliente externo**, dirigindo cada
cadeia por propostas separadas. Bom para nós: um processo externo é trivial de
matar num ponto escolhido.

### 2. Não há relógio nem temporizador no chaincode

`time.Now()` é não-determinístico: peers endossantes executam em momentos
diferentes e produziriam write sets divergentes, quebrando a política de
endosso.

**Efeito:**
- Prazos usam `stub.GetTxTimestamp()` — o timestamp da proposta do cliente,
  aceito pelos peers dentro de uma janela de tolerância.
- **Nada dispara sozinho.** Expiração de timelock (HTLC) e `TimeoutAbort` (2PC)
  precisam ser invocados por um cliente externo. Se ninguém invoca, o ativo fica
  travado mesmo com o prazo vencido — e isso precisa entrar na interpretação das
  métricas de tempo de bloqueio.
- A segurança temporal passa a depender de **sincronia aproximada de relógios**
  entre peers. Hipótese a declarar; e é o que o cenário X1 (clock skew) explora.

### 3. Conflito MVCC não é aborto de protocolo

Dois `Prepare` concorrentes sobre a mesma chave: o segundo falha na **validação**
(não no endosso), com `MVCC_READ_CONFLICT`.

**Efeito:** o coordenador precisa de retentativa, e o coletor de métricas precisa
**distinguir conflito de concorrência de um voto `NO`**. Contá-los juntos
inflaria artificialmente a taxa de aborto do 2PC e falsearia a comparação.

## Canais vs redes

Um **canal** é uma sub-rede com ledger e estado próprios — o jeito leve de ter
múltiplos ledgers no mesmo ambiente. Mas canais **compartilham o serviço de
ordenação**, o que enfraquece a analogia com cadeias independentes e impede
falhar o orderer de uma cadeia sem afetar a outra.

**Decisão do trabalho: duas redes separadas**, cada uma com seus peers, CA e
orderer. Além de mais fiel ao modelo, é *menos* trabalho — o testbed do Weaver
já entrega as duas prontas. Custo: ~16 GB de RAM.

## Ciclo de vida do chaincode (Fabric 2.x)

Necessário para o `03-deploy-2pc.sh`:

```
package  →  install (em cada peer)  →  approveformyorg (cada org)
         →  checkcommitreadiness    →  commit (uma vez por canal)
         →  invoke / query
```

O modelo 2.x exige **aprovação de organizações suficientes** pela política do
canal antes da efetivação. Um upgrade de chaincode repete o ciclo com `--sequence`
incrementado. Pegadinha clássica: esquecer de incrementar a sequência e receber
uma mensagem de erro pouco informativa.

## O testbed do Weaver

`cacti/weaver/tests/network-setups/fabric/dev/`:

- `FABRIC_VERSION=2.5.16`, `FABRIC_CA_VERSION=1.5.15` (fixados no Makefile)
- `network1.env` / `network2.env` — portas e nomes distintos por rede
- Alvos: `make start`, `start-network1`, `start-network2`,
  `start-interop CHAINCODE_NAME=simpleasset`, `stop`, `clean`
- Sobe, por rede: CA, peers, orderer e CouchDB, mais o interop chaincode e o
  chaincode de aplicação

**Atenção:** o `base.env` ainda define `SYS_CHANNEL=system-channel`, o modelo
legado de canal de sistema. Funciona no 2.5 (depreciado, mas suportado) e foi
removido no Fabric 3.x. Não planejar migração para o 3.x.

**Atenção 2:** os READMEs do monorepo estão defasados em relação ao código (por
exemplo, exigências de versão do Node). Confiar em `go.mod` e `package.json`.

## Pegadinhas encontradas na prática

> Preencher durante as Etapas 1–4. Cada item: sintoma, causa, solução.

*(nenhuma registrada ainda — o spike da Etapa 1 é o primeiro contato)*

## Referências

Androulaki et al. EuroSys'18 (arquitetura) · Thakkar et al. MASCOTS'18
(desempenho) · documentação oficial do Fabric v2.5. Entradas completas em
[`docs/paper/refs.bib`](../paper/refs.bib).
