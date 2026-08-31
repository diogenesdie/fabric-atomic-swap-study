# 2PC — Two-Phase Commit

*Two-Phase Commit* (protocolo de efetivação em duas fases). Sigla **2PC**.

## Commit atômico ≠ consenso

Distinção que precisa estar clara antes de tudo:

| | Consenso | Commit atômico |
|---|---|---|
| Decisão | por maioria/quórum | **unanimidade para efetivar** |
| Uma recusa | é minoria, ignorada | **aborta tudo** |
| Progresso | maioria viva basta | depende de todos responderem |

Em consenso, se a maioria diz "sim", decidiu-se "sim". Em commit atômico, **uma
única recusa aborta a transação inteira**. É uma propriedade mais forte e por
isso mais frágil a falhas — não dá para "tirar a média".

As propriedades formais (AC1–AC5 em Bernstein/Hadzilacos/Goodman, cap. 7):
todos os processos que decidem decidem igual; um processo não muda de decisão;
`commit` só se todos votaram sim; sem falhas e com todos votando sim, a decisão
é `commit`; e toda execução acaba decidindo.

## O protocolo (Gray 1978)

**Fase 1 — preparo (voting).** O coordenador envia `PREPARE` a todos os
participantes. Cada um verifica se consegue efetivar, **persiste o estado
preparado em log durável** e responde `VOTE-YES` ou `VOTE-NO`. Ao votar sim, o
participante abre mão do direito de abortar unilateralmente — fica em
**incerteza**.

**Fase 2 — efetivação (decision).** Se todos votaram sim, o coordenador
**persiste a decisão** e envia `COMMIT`; se algum votou não (ou não respondeu),
envia `ABORT`. Participantes aplicam e confirmam.

A ordem "persiste antes de enviar" não é detalhe de implementação: é o que
permite ao coordenador reconstruir a decisão após um crash. No nosso coordenador
isso é o **write-ahead log**, e é a peça central do cenário T2.

## Por que 2PC bloqueia

**Este é o ponto delicado do lado 2PC, simétrico ao dos timelocks no HTLC.**

Se o coordenador cai **entre as fases** — depois de coletar votos, antes de
comunicar a decisão — os participantes que votaram sim ficam em incerteza:

- não podem efetivar (talvez alguém tenha votado não);
- não podem abortar (talvez a decisão tenha sido efetivar e alguém já a
  conheça);
- só podem **esperar**.

Esperar significa manter os recursos travados. Cross-chain, significa **ativos
imobilizados numa cadeia por tempo indeterminado**. E não é um caso raro: é o
comportamento previsto do protocolo.

Skeen (1981) provou que nenhum protocolo de commit é não-bloqueante sob
particionamento de rede, e propôs o **3PC**, não-bloqueante sob falhas de crash
(ao custo de uma fase extra). Gray & Lamport (2006) atacam pelo outro lado com o
**Paxos Commit**: substituem o coordenador único por um conjunto replicado por
consenso — a decisão sobrevive à queda de qualquer nó.

É exatamente esse o movimento que o mundo cross-chain faz: **"substituir o
coordenador confiável por entidade tolerante a falhas"** — no AC3WN, por uma
blockchain testemunha.

## 2PC cross-chain

Adaptações necessárias:

- **Escrow no lugar de lock de banco de dados.** Não há gerenciador de
  transações no ledger. O `Prepare` move o ativo para um estado de custódia
  (escrow) gravado no próprio ledger; `Commit` transfere ao novo dono, `Abort`
  devolve.
- **O coordenador não pode ser on-chain.** No Fabric, `InvokeChaincode` para
  outro canal é **somente leitura** — o resultado não entra no write set nem é
  validado no commit. Não existe transação que abranja duas cadeias. O
  coordenador é obrigatoriamente um **cliente externo**.
- **Idempotência obrigatória.** Recuperação implica reenvio; `Commit` e `Abort`
  precisam ser seguros para executar duas vezes.
- **Escape não-bloqueante.** `TimeoutAbort` permite ao participante abortar
  unilateralmente depois de um prazo — mitiga o bloqueio, mas reintroduz uma
  hipótese temporal, aproximando o 2PC do HTLC nesse aspecto. Medir com e sem é
  um resultado interessante por si só.

Que o coordenador seja externo é **conveniente para o experimento**: matá-lo com
`SIGKILL` num ponto escolhido é trivial, e é precisamente a falha que queremos
estudar.

## O que existe publicado

Levantamento feito em agosto de 2026:

- **AC3WN** (Zakhary et al. 2020) — substitui o coordenador por uma blockchain
  testemunha permissionless. **Sem código publicado**, e inaplicável ao nosso
  ambiente. Vale pela §3 do arXiv, que é o argumento de falha contra o HTLC.
- **Lu, Jajoo & Namjoshi** (DIN 2024) — protocolo de duas fases (congelar →
  executar com rollback) sobre LayerZero, em Solidity. **Sem código publicado.**
  Útil pelos números de LOC, que validam nossa estimativa.
- **2PC4BC** (Falazi et al. 2025) — *Resource Manager Smart Contracts*, o design
  publicado mais próximo do que vamos construir. Sem repositório.
- **IETF SATP** — o Stage 3 do padrão é literalmente um 2PC entre gateways
  (`CommitPreparation` → `CommitReady` → `CommitFinal` → `TransferComplete`,
  com `Rollback`). Implementado no plugin SATP-Hermes do Cacti (TypeScript,
  pesado, caminho testado é Fabric↔Besu).

**Conclusão: não existe 2PC cross-chain público utilizável para Fabric.** O
braço 2PC é implementação nossa. Adotamos o vocabulário do SATP para as
mensagens — custo zero e evita a acusação de estarmos comparando contra um
espantalho de fabricação própria.

## Nosso desenho

Chaincode `twopc` (idêntico nas duas redes):

| Função | Papel no protocolo |
|---|---|
| `Prepare(txID, assetID, newOwner, deadline)` | fase 1: valida, move para escrow, grava `TxRecord{PREPARED}` |
| `Commit(txID)` | fase 2: escrow → novo dono. Idempotente |
| `Abort(txID)` | fase 2: escrow → dono original. Idempotente |
| `TimeoutAbort(txID)` | escape unilateral após o prazo, via `GetTxTimestamp()` |
| `GetTxRecord`, `GetEscrowed` | consultas para o runner classificar o desfecho |

Coordenador (Go, cliente externo): `Prepare` nas duas redes → coleta votos →
**persiste a decisão no WAL** → `Commit`/`Abort` nas duas → retenta até
confirmação. Ao reiniciar, lê o WAL e retoma o que ficou pendente.

## Referências

Gray (1978) · Skeen (1981) · Bernstein/Hadzilacos/Goodman (1987, cap. 7) ·
Gray & Lamport (2006) · Zakhary et al. PVLDB'20 · Falazi et al. ACM DLT'25 ·
Ezhilchelvan et al. CryBlock'18 · Lu et al. DIN'24 + arXiv:2403.07248 ·
IETF draft-ietf-satp-core-16.
Entradas completas em [`docs/paper/refs.bib`](../paper/refs.bib).
