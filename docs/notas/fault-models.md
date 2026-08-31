# Modelos de falha e cenários do experimento

## Vocabulário

- **Crash (fail-stop)** — o processo para e não volta a agir; não produz
  comportamento incorreto. É o modelo do 2PC clássico.
- **Bizantino** — desvio arbitrário do protocolo: mentir, enviar mensagens
  contraditórias, agir por interesse próprio. O cenário cross-chain, com partes
  mutuamente desconfiadas, pede este modelo.
- **Omissão / atraso** — mensagens perdidas ou atrasadas sem que ninguém pare.
  Sob sincronia parcial, indistinguível de crash — e é justamente aí que o
  timelock do HTLC quebra.
- **Particionamento** — subconjuntos que não se comunicam. Skeen (1981) provou
  que nenhum protocolo de commit é não-bloqueante sob partição.

## O que cada protocolo assume

| | HTLC | 2PC |
|---|---|---|
| Confiança | nenhuma entre as partes | coordenador confiável (ou tolerante a falhas) |
| Tempo | **sincronia obrigatória** (o timelock depende dela) | assíncrono; sem prazo, apenas bloqueia |
| Crash de participante | tolera — timelock devolve o ativo | tolera — coordenador aborta |
| Crash do coordenador | não existe coordenador | **bloqueia** — a falha central |
| Comportamento egoísta | resiste (o segredo é o compromisso) | um coordenador malicioso quebra tudo |
| Atraso além do prazo | **pode violar atomicidade** | apenas mais lento |

A leitura resumida: **os dois protocolos deslocam a fragilidade para lugares
diferentes**. O HTLC troca confiança por uma hipótese temporal; o 2PC troca a
hipótese temporal por confiança num coordenador. Não há almoço grátis — e é isso
que o experimento quantifica.

## Como cada falha é injetada

| Falha | Mecanismo |
|---|---|
| Queda de participante | `docker stop` / `docker pause` no peer da vítima |
| Queda do coordenador | `SIGKILL` no processo, em ponto marcado (`--crash-after`) |
| Abandono do cliente HTLC | o próprio orquestrador se mata no passo indicado |
| Atraso de rede | `tc`/`netem` **dentro** dos containers Linux (ou via pumba) |
| Clock skew | ajuste de relógio / `faketime` no container do peer |

No macOS o `tc` não roda no host, mas roda normalmente dentro dos containers
Linux — é onde ele precisa estar de qualquer forma. Testar cedo, na Etapa 4.

## Matriz de cenários

Cada cenário roda N ≥ 10 vezes, com estado limpo entre execuções.

| # | Protocolo | Falha | Resultado esperado |
|---|---|---|---|
| **B1** | HTLC | nenhuma | baseline: latência e nº de transações |
| **B2** | 2PC | nenhuma | baseline; deve concluir mais rápido que B1 |
| **H1** | HTLC | Bob nunca trava (crash após o lock de Alice) | Alice recupera por expiração; sem violação; bloqueio ≈ `t₁` |
| **H2** | HTLC | Bob cai depois de Alice revelar o segredo | segredo já público; Bob recupera até `t₂`; janela de risco |
| **H3** | HTLC | atraso de rede maior que a margem `t₁ − t₂` | **violação de atomicidade possível** — testa a crítica de Zakhary |
| **T1** | 2PC | participante cai após `Prepare` | coordenador aborta os dois lados; sem violação |
| **T2** | 2PC | **coordenador morto entre as fases** | **bloqueio** até recuperação pelo WAL ou `TimeoutAbort` |
| **T3** | 2PC | atraso de rede nas duas fases | latência cresce; **sem violação** (contraste com H3) |
| **X1** | HTLC | clock skew entre peers das duas redes | ângulo original — ninguém mediu |

O par **H3 × T3** é o coração do artigo: mesma falha (atraso), consequências
qualitativamente diferentes — o HTLC pode perder segurança, o 2PC só perde
desempenho. O par **T2 × H1** é o espelho: o 2PC bloqueia onde o HTLC se
resolve sozinho.

## Classificação do desfecho

Ao fim de cada execução, o runner lê o estado final **das duas cadeias** e
classifica:

| Desfecho | Significado |
|---|---|
| `COMMITTED_BOTH` | ambos os lados efetivaram — troca completa e correta |
| `ABORTED_BOTH` | nenhum efetivou — aborto correto, atomicidade preservada |
| `VIOLATED` | **só um lado efetivou** — falha de atomicidade |
| `BLOCKED` | ativo em escrow/lock sem resolução dentro do prazo do cenário |

`VIOLATED` e `BLOCKED` são os desfechos interessantes: o primeiro é falha de
**segurança**, o segundo de **vivacidade**. A hipótese é que eles se distribuam
de forma complementar entre os dois protocolos.

## Métricas

- **Segurança** — houve efetivação parcial? (contagem de `VIOLATED`)
- **Latência de conclusão** — do início ao desfecho estável
- **Tempo de bloqueio** — quanto tempo cada ativo ficou indisponível ao dono
- **Custo** — número de transações e de invocações de chaincode

Definições de vazão e latência seguem o white paper de métricas do Hyperledger
Performance and Scale WG, que também exige divulgar o ambiente — o que alimenta
a seção *Threats to Validity*.

## Referências

Skeen (1981) · Gray & Lamport (2006) · Zakhary et al. PVLDB'20 (§3, o ataque ao
HTLC) · Hajdu et al. IEEE Access'20 (injeção de falhas em Fabric) ·
Sondhi et al. DASC'21, arXiv:2108.08441 (chaos engineering em blockchains
permissionadas). Entradas completas em [`docs/paper/refs.bib`](../paper/refs.bib).
