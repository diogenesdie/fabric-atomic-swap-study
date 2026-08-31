# HTLC vs 2PC — Atomicidade em transações cross-chain

Estudo comparativo empírico entre **HTLC** (Hashed Timelock Contract) e **2PC**
(Two-Phase Commit) para atomicidade entre múltiplas blockchains, implementado
sobre **Hyperledger Fabric 2.5** e avaliado sob os mesmos cenários de falha.

Trabalho da disciplina *Plataformas Computacionais para Sistemas Distribuídos*
(PPGCC/PUCRS, 2026/2), Prof. Fernando Luís Dotti.

**Equipe:** Diógenes Dietrich de Morais · José Francisco Dias Venturini ·
Pedro Augusto Pereira

## A pergunta

HTLC e 2PC resolvem a mesma propriedade de segurança — atomicidade: ou todos os
lados efetivam, ou nenhum — sob premissas diferentes de confiança, falha e
tempo. A literatura compara as duas famílias *argumentando*; ninguém as colocou
lado a lado, na mesma plataforma, sob os mesmos cenários de falha, *medindo*.

**Hipótese.** HTLC preserva atomicidade sem coordenador, ao custo de bloqueio
mais longo e dependência de hipóteses de sincronia. A variante 2PC conclui mais
rápido no caso sem falhas, mas fica exposta ao bloqueio quando o coordenador
falha.

## Como está montado

| Braço | Origem do código |
|---|---|
| **HTLC** | Reutilizado do [Hyperledger Cacti/Weaver](https://github.com/hyperledger-cacti/cacti) (Apache-2.0) — biblioteca `assetexchange`, chaincode `simpleasset` e testbed de duas redes Fabric 2.5.16 |
| **2PC** | Implementação própria — não existe 2PC cross-chain público para Fabric (ver [docs/notas/2pc.md](docs/notas/2pc.md)) |

Duas **redes Fabric independentes** (não dois canais), com orderers próprios,
para que falhas possam ser injetadas em cada cadeia de forma independente.

## Quickstart

```bash
make prereqs      # verifica Docker, Go, make, RAM
make setup        # clona o Cacti/Weaver pinado em versão
make networks-up  # sobe as duas redes Fabric + chaincodes
make spike        # executa um swap HTLC completo (gate de viabilidade)
```

Depois de validado o spike:

```bash
make setup-htlc   # corrige os perfis, registra usuários, popula ativos
make deploy-2pc   # instala o chaincode twopc nas duas redes
make venv         # ambiente Python da análise (uma vez)
make scenarios    # lista os 13 cenários
make experiments  # roda a matriz completa (N=10 por padrão; ~2 h)
make analyze      # gera as tabelas e figuras do artigo
make networks-down
```

Para um cenário só, ou menos repetições:

```bash
./experiments/runner.sh --scenario=H2 --repeat=3
make experiments N=3
```

## Resultados

O contraste que o estudo mede, com a mesma falha aplicada aos dois protocolos:

| | HTLC | 2PC |
|---|---|---|
| Sem falha | 8,1 s | 4,1 s (fases paralelas) · 8,1 s (sequenciais) |
| Sob atraso de rede | aborta com segurança | efetiva, mais lento |
| Coordenador/contraparte cai | recupera por prazo | bloqueia até recuperação |
| Viola atomicidade quando | o prazo vence com o segredo revelado | o prazo dispara num participante só |

A conclusão não é a que esperávamos: **os dois protocolos quebram do mesmo
jeito**. O mecanismo que cada um usa para comprar vivacidade — timelock no
HTLC, aborto unilateral no 2PC — é o que custa a segurança.

Números atualizados em `docs/paper/figures/results.md` após `make analyze`.

## Mapa do repositório

```
docs/progresso.html  relatório de andamento, abre no navegador
docs/notas/        anotações teóricas (PT) — HTLC, 2PC, Fabric, modelos de falha
docs/paper/        artigo (EN) — outline, related work, refs.bib, figuras
docs/decisoes.md   registro de decisões de projeto
scripts/           setup, subida das redes, deploy de chaincode
apps/coordinator/  coordenador 2PC com write-ahead log e recuperação
apps/htlc-orchestrator/  cliente HTLC instrumentado
chaincode/twopc/   chaincode 2PC (Prepare/Commit/Abort/TimeoutAbort)
internal/          conexão e instrumentação compartilhadas pelos dois braços
experiments/       cenários, injeção de falha, runner, resultados
analysis/          agregação, resumo e figuras (Python)
```

## Requisitos

Docker + Compose v2, Go 1.21+, `make`, `git`, `curl`, `jq` e Python 3
(matplotlib para as figuras, instalado por `make venv`).

**Memória: ~8 GB alocados ao Docker bastam.** O plano original supunha 16 GB,
mas as duas redes juntas consomem cerca de 280 MiB — o testbed usa um peer por
rede e LevelDB. Em Apple Silicon roda nativo: o Fabric 2.5.16 publica imagens
`arm64`.

Linux ou macOS; Windows via WSL2.

## Documentos de referência

- [docs/notas/](docs/notas/) — fundamentação teórica e pegadinhas da plataforma
- [docs/paper/related-work.md](docs/paper/related-work.md) — posicionamento e lacuna
- [CLAUDE.md](CLAUDE.md) — contexto completo do trabalho
