# HTLC — Hashed Timelock Contract

*Hashed Timelock Contract* (contrato com trava de hash e de tempo). Primeira
menção sempre por extenso; a sigla é **HTLC**, nunca HTCL.

## A ideia

O 2PC precisa de um coordenador confiável. O HTLC pergunta: e se trocássemos
confiança por **criptografia + tempo**?

Duas travas sobre o mesmo ativo:

- **Hashlock** — o ativo só é liberado a quem apresentar a pré-imagem `s` de um
  hash `h = H(s)` publicado no contrato. O ponto crucial: ao resgatar, o segredo
  vira **público no ledger**, e é isso que habilita a contraparte na outra
  cadeia. O segredo é o mecanismo de propagação da decisão.
- **Timelock** — se a troca não completa até um prazo `t`, o ativo volta ao dono
  original. É o que evita que um ativo fique travado para sempre.

Nenhuma das duas cadeias precisa saber que a outra existe. A ligação entre elas
é o segredo, carregado por quem tem interesse em completar a troca.

## O protocolo (Nolan 2013, formalizado por Herlihy 2018)

Alice tem um ativo na cadeia A, Bob tem outro na cadeia B, querem trocar.

| # | Quem | Onde | Ação |
|---|---|---|---|
| 1 | Alice | — | sorteia `s`, calcula `h = H(s)` |
| 2 | Alice | cadeia A | trava seu ativo com `h`, prazo `t₁`, resgatável por Bob |
| 3 | Bob | cadeia B | vê o lock de Alice, trava o dele com o **mesmo `h`**, prazo `t₂`, resgatável por Alice |
| 4 | Alice | cadeia B | resgata com `s` — **o segredo vaza para o ledger B** |
| 5 | Bob | cadeia A | lê `s` do ledger B e resgata na cadeia A |

Se algo falha, os timelocks expiram e cada um recupera o seu.

## Por que os dois prazos precisam ser diferentes

**Este é o ponto delicado que mais aparece em prova.**

Quem revela o segredo primeiro (Alice, no passo 4) fica em desvantagem: a partir
dali Bob sabe `s` e pode resgatar quando quiser, enquanto Alice já se
comprometeu. Se `t₁ = t₂`, existe uma janela em que Alice resgata na cadeia B
faltando pouco para os dois prazos vencerem, e Bob não consegue mais resgatar na
cadeia A a tempo. Resultado: **efetivação parcial** — Alice fica com os dois
ativos.

A regra: **`t₁ > t₂`**, com folga. Quem inicia (Alice, que revela primeiro) tem
o prazo **maior**; quem responde tem o prazo **menor**. A diferença `t₁ − t₂`
precisa cobrir o pior caso de: Alice resgatar em B, a transação ser incluída em
bloco, Bob observar, e Bob submeter e ter incluída a transação de resgate em A.

Convenção usual: `t₁ = 2·t₂`. No nosso experimento, os valores são parâmetros
dos cenários (`--timeout1`, `--timeout2`) justamente para explorar a margem.

## A hipótese de sincronia (o calcanhar de Aquiles)

O timelock só é seguro se existir **limite conhecido** para atraso de mensagens
e produção de blocos. Se um participante honesto não consegue executar a tempo —
rede congestionada, peer lento, relógio adiantado — o prazo expira e a
atomicidade **pode ser violada**, mesmo sem ninguém agir de má-fé.

É exatamente esse o ataque de Zakhary (2020) ao HTLC, e é o que o **cenário H3**
testa empiricamente: injetar atraso maior que a margem `t₁ − t₂` e verificar se
aparece efetivação parcial.

Ou seja: o HTLC não elimina a confiança, ele a **desloca** — de um coordenador
para uma hipótese temporal sobre a rede.

## No Hyperledger Fabric

O Fabric **não tem timelock nativo**. Não há relógio confiável no chaincode:
`time.Now()` é não-determinístico e quebra o endosso (peers diferentes
produziriam write sets diferentes). O timelock é implementado comparando
`stub.GetTxTimestamp()` — o timestamp que o cliente colocou na proposta,
validado dentro de uma janela pelos peers — contra o prazo gravado no estado.

Consequências:

1. A segurança do timelock depende de **sincronia aproximada de relógios entre
   peers** — uma hipótese adicional que o Bitcoin/Ethereum não precisam fazer da
   mesma forma. Declarar explicitamente no artigo.
2. Não existe temporizador que dispare sozinho. O resgate por expiração
   (`unlock`) precisa ser invocado por **um cliente externo** — o "watchdog" da
   vítima. Se ninguém invoca, o ativo fica travado indefinidamente, mesmo com o
   prazo vencido.
3. **Clock skew vira variável injetável** — cenário X1, o ângulo original do
   trabalho.

## Ataques e limites (contexto permissionado)

- **MAD-HTLC** (Tsabary et al. 2021): mineradores podem ser subornados para
  censurar a transação de resgate até o prazo expirar. **Em Fabric permissionado
  o vetor praticamente desaparece** — não há mempool aberta nem competição por
  taxa. Vale declarar como achado: parte da crítica ao HTLC é específica do
  ambiente permissionless.
- **Sore loser / griefing** (Xue & Herlihy 2021): mesmo sem ganhar nada, um
  participante pode abandonar a troca depois do lock, forçando a contraparte a
  ficar com capital imobilizado até a expiração. Não viola atomicidade, mas tem
  custo real — é a nossa métrica de **tempo de bloqueio**.

## Implementação que vamos usar

Biblioteca `assetexchange` do Hyperledger Cacti/Weaver (Apache-2.0), em Go:

- `LockAsset` / `LockFungibleAsset` — trava com hash e prazo
- `ClaimAssetUsingContractId` — resgata apresentando a pré-imagem
- `UnlockAssetUsingContractId` — devolve após expiração
- `IsAssetLockedQueryUsingContractId` — consulta de estado
- `GetHTLCHashPreImage` — recupera o segredo já revelado (é assim que Bob
  descobre `s` sem falar com Alice)

Detalhe de projeto importante: **a biblioteca só gerencia os registros de trava**
— quem muda a titularidade do ativo é o chaincode da aplicação. Daí a existência
do `simpleasset` como camada de integração.

## Referências

Nolan (2013) · Herlihy PODC'18 · Herlihy/Liskov/Shrira VLDBJ'22 ·
Tsabary et al. S&P'21 (MAD-HTLC) · Xue & Herlihy PODC'21 (sore loser) ·
Narayanam et al. AFT'22 (MPHTLC em Weaver). Entradas completas em
[`docs/paper/refs.bib`](../paper/refs.bib).
