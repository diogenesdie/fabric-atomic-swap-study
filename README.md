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
make deploy-2pc   # instala o chaincode twopc nas duas redes
make experiments  # roda a matriz de cenários com injeção de falha
make analyze      # gera tabelas e gráficos a partir dos CSVs
make networks-down
```

## Mapa do repositório

```
docs/notas/        anotações teóricas (PT) — HTLC, 2PC, Fabric, modelos de falha
docs/paper/        artigo (EN) — outline, related work, refs.bib, figuras
docs/decisoes.md   registro de decisões de projeto
scripts/           setup, subida das redes, deploy de chaincode
chaincode/twopc/   chaincode 2PC (Prepare/Commit/Abort/TimeoutAbort)
apps/coordinator/  coordenador 2PC com write-ahead log e recuperação
apps/htlc-orchestrator/  cliente HTLC instrumentado
experiments/       cenários, injeção de falha, runner, resultados
analysis/          agregação e gráficos (Python)
```

## Requisitos

Docker + Compose v2, Go 1.21+, `make`, `git`, `curl`, `jq`, Python 3 com pandas
e matplotlib. **16 GB de RAM** — são duas redes Fabric completas na mesma
máquina. Linux ou macOS; Windows via WSL2.

## Documentos de referência

- [docs/notas/](docs/notas/) — fundamentação teórica e pegadinhas da plataforma
- [docs/paper/related-work.md](docs/paper/related-work.md) — posicionamento e lacuna
- [CLAUDE.md](CLAUDE.md) — contexto completo do trabalho
