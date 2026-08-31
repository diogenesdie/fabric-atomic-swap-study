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

Todas descobertas no spike da Etapa 1 (31.08.2026) e já tratadas nos scripts.
Cada item: sintoma, causa, solução.

### 1. Connection profile YAML não parseia

**Sintoma:** `While parsing config: yaml: line 88: could not find expected ':'`

**Causa:** na seção `certificateAuthorities`, o bloco literal do certificado é
aberto como item de lista (`- |`) e a primeira linha do PEM recebe 2 espaços de
indentação a mais que as seguintes. Em YAML a indentação do bloco é definida
pela primeira linha; as seguintes, menos indentadas, encerram o bloco e passam a
ser lidas como YAML.

**Solução:** `scripts/fix_connection_profiles.py` normaliza a indentação.
Não serve trocar para o `connection-org1.json`: ele é sintaticamente válido mas
**não tem a seção `orderers`** — é um perfil incompleto.

### 2. Registro de usuário falha por falta de registrar

**Sintoma:** `CA registrar not found`

**Causa:** o perfil gerado não traz a entrada `registrar`, que o SDK usa para
autenticar como admin da CA ao registrar novos usuários.

**Solução:** injetar `registrar: {enrollId: admin, enrollSecret: adminpw}`. A
identidade de bootstrap vem de `fabric-ca-server start -b admin:adminpw` em
`docker/docker-compose-ca.yaml`.

### 3. Nome da CA divergente

**Sintoma:** `Error Code: 19 - CA 'ca-org1' does not exist`

**Causa:** o perfil declara `caName: ca-org1`, mas o servidor foi iniciado com
`FABRIC_CA_SERVER_CA_NAME=ca.org1.<rede>.com`. Confirmável com
`curl -sk https://localhost:7054/cainfo`.

**Solução:** alinhar `caName` à chave da própria entrada, que já está correta.

### 4. Dados de exemplo com vencimento no passado

**Sintoma:** `maturity date can not be in past` — **e a CLI sai com código 0**,
fingindo sucesso. Só olhando o log se descobre que nada foi criado.

**Causa:** `data/assets.json` do Weaver tem vencimentos em 2022.

**Solução:** fixtures próprios em `experiments/fixtures/` com data futura, e o
setup valida as datas antes de usar. **Lição geral: a go-cli engole erros de
invocação e retorna 0** — sempre inspecionar o log em busca de `Invoke error`.

### 5. `ReadAsset` só responde ao dono

**Sintoma:** `cannot access Bond Asset a03` ao consultar com a identidade que
acabou de perder o ativo.

**Causa:** o `simpleasset` restringe a leitura ao proprietário. Não existe
identidade neutra de observação.

**Solução:** para descobrir o dono, tentar cada identidade candidata — quem
consegue ler é o dono — e confirmar pelo certificado do campo `owner`. É o que
`bond_owner()` faz em `scripts/04-spike-htlc.sh`.

### 6. Titularidade é certificado, não nome

O campo `owner` de um bond é o **certificado X.509 do dono em base64**, não a
string `alice`. O nome fica no atributo `hf.EnrollmentID`, embutido pelo Fabric
CA. `scripts/ledger_state.py` faz a resolução — essencial para classificar
desfechos no harness.

Tokens são diferentes: `GetMyWallet` devolve `token1="10000"` em claro.
`GetBalance` exige que o dono já tenha carteira e falha com
`owner does not have a wallet`.

### 7. `configure asset add` não é idempotente

Cada execução **emite novas unidades** de token. Rodar duas vezes dobra os
saldos (foi assim que alice apareceu com 20000 em vez de 10000 na primeira
tentativa). Só executar em estado limpo.

### 8. Verbosidade da CLI

A go-cli despeja stack traces enormes do `fabric-sdk-go` em toda falha. Sempre
redirecionar para arquivo e filtrar (`grep -oE 'Description: [^\\]*'`), senão o
sinal desaparece no ruído.

### 9. `DISCOVERY_AS_LOCALHOST` — a variável que ninguém documenta

**Sintoma:** toda consulta falha com
`Endorser Client Status Code: (2)` tentando alcançar
`peer0.org1.network1.com:7051`.

**Causa:** o *service discovery* do Fabric devolve os endereços **internos** dos
peers, que não resolvem a partir do host. Vale para qualquer cliente rodando
fora da rede Docker.

**Solução:** `os.Setenv("DISCOVERY_AS_LOCALHOST", "true")` antes de
`gateway.Connect`. O fabric-sdk-go então reescreve os endereços descobertos para
`localhost`. É o que a go-cli faz em `FabricHelper()`, sem nenhum comentário
explicando. Está em `apps/htlc-orchestrator/fabric.go`.

### 10. Reset de estado é caro — use um ativo por execução

Derrubar e recriar as duas redes leva ~3 minutos, inviável para uma matriz com
N ≥ 10 repetições por cenário. A saída: `experiments/fixtures/assets.json`
declara **doze bonds** (`a03`–`a14`), onze deles de alice. Cada execução recebe
um ativo virgem via `-bond-id`, e o reset completo só é necessário quando os
saldos de token precisam voltar ao início.

Os saldos de token, esses, acumulam entre execuções — o que não atrapalha,
porque a classificação do desfecho compara o estado **antes e depois** de cada
execução, não valores absolutos.

## Latência: dominada pelo corte de bloco

Medição do spike (baseline sem falha, `1-node`, arm64, Docker 7,7 GB):

| Passo | Tipo | Duração |
|---|---|---|
| 1 lock-bond | escrita | 2109 ms |
| 2 verify-bond-lock | leitura | 54 ms |
| 3 lock-tokens | escrita | 2094 ms |
| 4 verify-token-lock | leitura | 53 ms |
| 5 claim-tokens | escrita | 2092 ms |
| 6 claim-bond | escrita | 2106 ms |
| **protocolo** | | **8508 ms** |

Toda transação de escrita custa ~2,1 s; toda leitura, ~50 ms. A causa está em
`config/configtx.yaml`: **`BatchTimeout: 2s`** com `MaxMessageCount: 500`. Como
o experimento envia uma transação por vez, nunca se enche um lote e o orderer
sempre espera o timeout inteiro.

Consequências para o trabalho:

1. **A latência mede corte de bloco, não lógica de protocolo.** O que distingue
   HTLC de 2PC é o *número de escritas no caminho crítico* e se elas podem ir em
   paralelo. HTLC tem 4 escritas estritamente sequenciais (o passo 5 precisa
   vazar o segredo antes do 6). O 2PC também tem 4, mas `Prepare` nas duas redes
   e `Commit` nas duas podem ir em paralelo — previsão: ~2 rodadas de bloco
   contra 4, ou seja, cerca de metade da latência.
2. **Manter `BatchTimeout` no padrão** e declarar na metodologia. Reduzi-lo
   comprimiria as duas curvas e esconderia justamente o efeito de interesse.
3. O total fim a fim do spike (23,2 s) inclui as consultas de estado e a leitura
   da pré-imagem. Para o artigo, a latência do protocolo é a soma dos passos.

## Referências

Androulaki et al. EuroSys'18 (arquitetura) · Thakkar et al. MASCOTS'18
(desempenho) · documentação oficial do Fabric v2.5. Entradas completas em
[`docs/paper/refs.bib`](../paper/refs.bib).
