# Transações atômicas entre múltiplas blockchains

Trabalho de mestrado para a disciplina **Plataformas Computacionais para Sistemas
Distribuídos** (PPGCC/PUCRS, 2026/2), Prof. Fernando Luís Dotti.

## Equipe

- Diógenes Dietrich de Morais
- José Francisco Dias Venturini
- Pedro Augusto Pereira

## Objetivo

Estudo comparativo entre **HTLC** e **2PC** para atomicidade cross-chain, com
implementação prática das duas abordagens em **Hyperledger Fabric** e medição
sob os mesmos cenários de falha.

A tese central: HTLC e 2PC resolvem a mesma propriedade de segurança
(atomicidade — ou todos os lados efetivam, ou nenhum) sob premissas diferentes
de confiança, falha e tempo. O trabalho compara empiricamente.

### O que diferencia este trabalho

As comparações existentes na literatura são feitas pelos *proponentes* de uma
das abordagens, dentro do argumento de motivação do próprio protocolo (o
Zakhary 2020 critica o HTLC para justificar o AC3WN). Ninguém colocou as duas
famílias lado a lado, na mesma plataforma, sob os mesmos cenários de falha,
medindo em vez de argumentar. É essa a lacuna.

## Conceitos-chave

- **Commit atômico** ≠ **consenso**. Consenso admite decisão por maioria/quórum;
  commit atômico exige unanimidade para efetivar — uma única recusa aborta tudo.
- **HTLC (Hashed Timelock Contract)**: substitui o coordenador confiável por
  criptografia + tempo. *Hashlock* libera o ativo só a quem apresentar a
  pré-imagem de um hash publicado; ao revelar, o segredo torna-se público e
  habilita a contraparte na outra cadeia. *Timelock* devolve o ativo ao dono se
  a troca não completar no prazo.
- **2PC cross-chain**: mantém as fases de preparo e efetivação, mas substitui o
  coordenador confiável por entidade tolerante a falhas.
- **Modelos de falha**: crash (para parar, sem comportamento incorreto) e
  bizantino (desvio arbitrário do protocolo). O 2PC clássico assume crash; o
  cenário cross-chain exige lidar com o segundo.
- **Canal (Fabric)**: sub-rede com ledger e estado próprios. Permite instanciar
  múltiplos ledgers num mesmo ambiente.

### Pontos delicados a lembrar

- **HTLC depende de hipótese de sincronia.** O timelock só é seguro se houver
  limite conhecido para atraso de mensagens e produção de blocos. Zakhary (2020)
  ataca exatamente isso: timelock expirado pode violar a atomicidade quando um
  participante honesto não consegue executar a tempo.
- **Os dois timelocks precisam de prazos diferentes.** Quem revela o segredo
  primeiro fica em desvantagem temporal; o prazo de quem inicia precisa ser
  maior, com folga para a contraparte resgatar depois que o segredo vaza.
- **2PC é bloqueante.** Se o coordenador cai entre as fases, os participantes
  ficam em incerteza — cross-chain, isso significa ativos travados.
- **Canais do Fabric não são cadeias totalmente independentes** — podem
  compartilhar o serviço de ordenação. Limitação a declarar explicitamente;
  avaliar se convém usar orderers distintos.

## Experimento

Quatro etapas:

1. **Configurar** — duas redes/ledgers independentes no Hyperledger Fabric
2. **Implementar** — swap atômico com HTLC e com 2PC
3. **Falhar** — executar os dois protocolos sob falhas controladas
4. **Verificar** — analisar atomicidade, latência, bloqueio e comportamento sob falha

### Falhas a injetar

- Queda de participante depois do travamento do ativo
- Queda antes da revelação do segredo
- Queda do coordenador entre as fases (caso 2PC)
- Expiração de prazo / atraso de rede (`tc`/`netem`)

### Métricas

- **Segurança**: ocorre efetivação parcial (só um lado) em algum cenário?
- **Latência** de conclusão
- **Tempo de bloqueio** dos ativos
- **Custo**: número de transações / chamadas de função

### Hipótese

HTLC preserva atomicidade sem coordenador, ao custo de bloqueio mais longo e
dependência de hipóteses temporais. A variante 2PC conclui mais rápido no caso
sem falhas, mas fica exposta ao bloqueio quando o coordenador falha.

## Stack

- **Docker** + **Docker Compose v2** (base de tudo)
- **Hyperledger Fabric 2.5.16** — via testbed do Weaver (`FABRIC_VERSION=2.5.16`,
  `FABRIC_CA_VERSION=1.5.15`)
- **Go** em todo o código próprio: chaincode 2PC, coordenador, orquestrador HTLC
- **Fabric Gateway SDK** (Go) para as aplicações cliente
- `git`, `curl`, `jq`
- **Python** (pandas, matplotlib) para análise dos resultados
- Linux/macOS nativo; Windows via WSL2

Hardware: **16 GB de RAM** — são duas redes Fabric completas na mesma máquina.

## Decisões fechadas (30.08.2026)

| Questão | Decisão | Motivo |
|---|---|---|
| Linguagem | **Go** em tudo | Weaver é Go; o esqueleto `asset-transfer-basic` é Go; evita o toolchain Node/yarn do monorepo Cacti, apontado como a maior fonte de dor de setup |
| Topologia | **Duas redes separadas** | O testbed oficial do Weaver já entrega duas redes prontas — é *menos* trabalho que adaptar para dois canais, e permite falhar orderers independentemente |
| Contratos | **Weaver para HTLC, implementação própria para 2PC** | O braço HTLC é reutilizável integralmente; não existe 2PC cross-chain público para Fabric |
| Artigo | **Inglês** | Viabiliza submissão futura |
| Git | Local por enquanto | — |

O plano B documentado (dois canais numa rede, 8 GB) permanece válido caso a
máquina não aguente as duas redes.

## Riscos conhecidos

O código (chaincode 2PC, scripts de orquestração, injeção de falha) é a parte
tranquila. O risco real é o **setup do Fabric**: material de MSP/certificados,
definição de canais, versão de chaincode, orderer — quebra por detalhe de
versão. Não é inviável, é trabalhoso.

**Mitigação**: *spike de viabilidade* antes de investir no resto — subir as duas
redes do testbed do Weaver e executar um swap HTLC completo com o `go-cli`. Se
isso roda, o resto é caminhável. É o portão da Etapa 1.

Risco secundário: **drift entre documentação e código no monorepo Cacti** (os
READMEs estão desatualizados em relação aos `go.mod`/`package.json`). Mitigação:
pinar o clone em tag/commit e confiar nos manifestos, não nos READMEs.

## Cronograma

| Período | Marco |
|---|---|
| 04.09 – 11.09 | Fundamentos: Fabric no ar, ciclo de vida do contrato |
| 18.09 – 02.10 | **Seminário** (entrega avaliada) — teoria HTLC vs 2PC |
| 09.10 – 23.10 | Implementação do swap atômico |
| 30.10 – 13.11 | **Experimento** (entrega avaliada) — execução e resultados |
| 27.11 | Discussão / reserva |

Avaliação: por grupo **e por indivíduo** — participação, seminário, experimentos.

### Três frentes de trabalho

1. **Plataforma** — rodar a demo oficial do Fabric, dominar o ciclo de vida do
   chaincode (empacotar, instalar, aprovar, efetivar, invocar)
2. **Teoria** — HTLC e 2PC: características, restrições, desafios
3. **Implementações** — levantar contratos publicados pelos autores da área

## Referências

Todas indicadas pelo professor.

**HTLC**
- HERLIHY, M. *Atomic Cross-Chain Swaps*. ACM PODC, 2018. arXiv:1801.09515
  — https://arxiv.org/abs/1801.09515
- HERLIHY, M.; LISKOV, B.; SHRIRA, L. *Cross-chain deals and adversarial
  commerce*. The VLDB Journal, v. 31, n. 6, p. 1291–1309, 2022.

**2PC**
- ZAKHARY, V.; AGRAWAL, D.; EL ABBADI, A. *Atomic Commitment Across
  Blockchains*. PVLDB, v. 13, n. 9, p. 1319–1331, 2020.
  — https://doi.org/10.14778/3397230.3397231
- LU, H.; JAJOO, A.; NAMJOSHI, K. S. *A Two-Phase Protocol for Atomic
  Multi-Chain Transactions*. ACM DIN, 2024, p. 21–27.
  — https://doi.org/10.1145/3694809.3700742

**Plataforma**
- Hyperledger Fabric v2.5 — https://hyperledger-fabric.readthedocs.io
- Hyperledger Cacti / Weaver — https://github.com/hyperledger-cacti/cacti
  (atenção: a organização mudou; `hyperledger-labs/weaver-dlt-interoperability`
  é o repositório legado)

> O "2PC… procurar" deixado em aberto pelo professor foi resolvido: a
> bibliografia completa está em [docs/paper/refs.bib](docs/paper/refs.bib), com
> as fundações clássicas (Gray 1978; Gray & Lamport 2006; Skeen 1981;
> Bernstein/Hadzilacos/Goodman 1987), o 2PC cross-chain publicado
> (Falazi et al. 2025 — 2PC4BC; Ezhilchelvan et al. 2018) e o padrão IETF SATP, cujo
> Stage 3 é literalmente um 2PC entre gateways. O posicionamento está em
> [docs/paper/related-work.md](docs/paper/related-work.md).

## Convenções

- Documentação e comentários em **português**.
- Código, nomes de variáveis e mensagens de commit em **inglês**.
- A sigla é **HTLC** (não HTCL).
- Nas apresentações, dizer o nome por extenso na primeira menção antes da sigla.
