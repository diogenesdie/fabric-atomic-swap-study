# Registro de decisões

Formato leve de ADR: contexto, decisão, consequências. Ordem cronológica
inversa. Decisão revista ganha entrada nova, a antiga fica marcada como
superada — o histórico do raciocínio importa para o artigo.

---

## D-005 — Vocabulário de mensagens do 2PC alinhado ao IETF SATP

**Data:** 30.08.2026 · **Status:** aceita

**Contexto.** O braço 2PC é implementação nossa, o que abre a objeção de estarmos
comparando o HTLC contra um 2PC de fabricação própria, escolhido para perder.
O Stage 3 do padrão IETF SATP (*Secure Asset Transfer Protocol*) é literalmente
um 2PC entre gateways, com mensagens nomeadas.

**Decisão.** Adotar a nomenclatura do SATP nas funções e eventos do chaincode
`twopc`: `LockAssertion` (voto), `CommitPreparation`, `CommitReady`,
`CommitFinal`, `Rollback`.

**Consequências.** Custo zero de implementação. O protocolo passa a ser
reconhecível como alinhado a um padrão em vias de padronização, não uma
invenção nossa. O modelo de falhas documentado do SATP-Hermes também serve de
referência para os cenários.

---

## D-004 — Artigo em inglês, documentação interna em português

**Data:** 30.08.2026 · **Status:** aceita

**Contexto.** A disciplina é em português; o artigo pode ser submetido depois a
workshop ou conferência.

**Decisão.** `docs/paper/` em inglês; `docs/notas/`, `docs/decisoes.md`,
`README.md` e comentários de código em português. Nomes de identificadores e
mensagens de commit em inglês, conforme as convenções do projeto.

---

## D-003 — Go em todo o código próprio

**Data:** 30.08.2026 · **Status:** aceita

**Contexto.** A decisão Go vs Node.js estava em aberto e trocar no meio custa
caro. O levantamento mostrou que o Weaver é Go, o esqueleto natural do chaincode
2PC (`asset-transfer-basic`) é Go, e existe uma CLI em Go (`go-cli`) equivalente
à de TypeScript.

**Decisão.** Go para chaincode `twopc`, coordenador, orquestrador HTLC e
ferramentas. Python apenas na análise dos resultados.

**Consequências.** Evita-se o build Node/yarn do monorepo Cacti, apontado como a
maior fonte de dor de setup. Perda: a documentação oficial do Weaver traz os
exemplos com o `fabric-cli` (TypeScript), então a tradução para o `go-cli` fica
por nossa conta.

---

## D-002 — Reutilizar o Weaver para HTLC; implementar apenas o 2PC

**Data:** 30.08.2026 · **Status:** aceita

**Contexto.** Levantamento de contratos públicos (agosto/2026):

- **HTLC:** o Hyperledger Cacti/Weaver tem implementação madura, Apache-2.0,
  Fabric 2.5.16, ativa — e o modo *asset exchange* **não exige relays, drivers
  nem IIN agents**, o que remove a parte mais pesada da stack. Os demais
  repositórios encontrados estão abandonados (era Fabric 1.x) ou **sem
  licença**, o que impede reuso em trabalho acadêmico.
- **2PC:** não existe implementação pública utilizável. AC3WN e o protocolo de
  Lu/Jajoo/Namjoshi não publicaram código; o SATP-Hermes do Cacti é 2PC de
  verdade, mas em TypeScript, com caminho testado Fabric↔Besu e histórico de
  instabilidade.

**Decisão.** Braço HTLC reutilizado do Weaver (biblioteca `assetexchange` +
chaincode `simpleasset` + `go-cli`). Braço 2PC implementado por nós, partindo do
`asset-transfer-basic` do fabric-samples (Apache-2.0, 194 linhas).

**Consequências.** Esforço estimado ~700–1.050 linhas de Go, compatível com o
prazo. A assimetria de esforço entre os braços é declarada no artigo — ela
própria é um achado sobre o estado do ecossistema. Risco: dependência de um
monorepo grande, mitigada por pinar o clone em versão.

---

## D-001 — Duas redes Fabric separadas, não dois canais

**Data:** 30.08.2026 · **Status:** aceita · **Supera:** decisão preliminar por
dois canais

**Contexto.** A escolha inicial foi por dois canais numa rede, por serem mais
leves (8 GB de RAM). O levantamento revelou que o testbed oficial do Weaver já
entrega **duas redes completas** com um comando — adaptá-lo para dois canais
seria trabalho *adicional*, não economia.

**Decisão.** Duas redes Fabric independentes, via
`weaver/tests/network-setups/fabric/dev`.

**Consequências.** Cada cadeia tem peers, CA e orderer próprios, o que permite
injetar falhas de forma independente e sustenta melhor a analogia com cadeias
distintas — importante porque o argumento do HTLC é justamente a ausência de
terceiro confiável compartilhado. Custo: ~16 GB de RAM.

**Plano B.** Se a máquina não aguentar, voltar a dois canais numa rede e
declarar a limitação (orderer compartilhado) na seção de ameaças à validade.
