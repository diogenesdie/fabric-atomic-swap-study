# Site do seminário

Página única, estática, que conta o seminário do zero: blockchain → contratos
inteligentes → o problema cross-chain → atomicidade → 2PC → HTLC → comparação →
Hyperledger Fabric → experimento → resultados → conclusão. Feita para leigos:
cada conceito tem uma figura animada ou um simulador com **▶ Reproduzir** e
**Passo →**.

Não há build. É HTML, CSS e JavaScript puros, sem dependências além das fontes
do Google Fonts.

## Estrutura

```
site/
  index.html              o seminário inteiro, em 12 capítulos
  assets/styles.css       tokens de tema (claro e escuro), tipografia, layout
  assets/app.js           trilho de capítulos, teclado, tema, capa animada
  assets/sims.js          simuladores (blockchain, contrato, 2PC, HTLC, Fabric)
  assets/charts.js        matriz de cenários, gráficos e tabela de resultados
  assets/results-data.js  GERADO — medianas por cenário, a partir do CSV
  tools/export_results.py gera results-data.js de experiments/results/aggregated.csv
  .nojekyll               evita o processamento Jekyll do GitHub Pages
```

## Ver localmente

```bash
cd site
python3 -m http.server 8000
# abra http://localhost:8000
```

Abrir o `index.html` direto do disco também funciona.

## Atualizar os números

Os resultados exibidos vêm de `experiments/results/aggregated.csv`. Depois de
uma nova rodada de experimentos:

```bash
python3 site/tools/export_results.py
```

Isso regrava `assets/results-data.js`. As descrições de cada cenário (título,
história, por que termina como termina) vivem em `tools/export_results.py` e em
`assets/charts.js`; os números, só no CSV.

## Publicar no GitHub Pages

**Opção A — a partir deste repositório (já configurado).** O workflow
`.github/workflows/pages.yml` publica a pasta `site/` a cada push na `main` que
a altere. Uma vez só, no GitHub: *Settings → Pages → Build and deployment →
Source: GitHub Actions*. O endereço fica em
`https://<usuário>.github.io/<repositório>/`.

**Opção B — repositório dedicado só para o site.** Copie o conteúdo de `site/`
para a raiz de um repositório novo e, em *Settings → Pages*, escolha *Deploy
from a branch* → `main` → `/ (root)`. Nada precisa mudar nos arquivos: todos os
caminhos são relativos.

## Navegação

- `→` / `←` (ou `PageDown` / `PageUp`, espaço) pulam entre capítulos.
- **⤢ Ampliar**, em cada figura animada, ocupa a tela toda para projeção.
  Ali, `→` ou espaço avançam um passo e `Esc` fecha.
- O trilho à esquerda mostra o capítulo atual; no celular ele vira o botão ☰.
- O botão no canto superior direito alterna tema claro/escuro (fica salvo no
  navegador).
- `prefers-reduced-motion` é respeitado: as animações viram estados finais e os
  simuladores continuam funcionando passo a passo.

## Convenções

Texto em português, para a turma. Código e nomes de identificadores em inglês.
As cores de desfecho são as mesmas do deck e do artigo: verde `COMMITTED_BOTH`,
cinza `ABORTED_BOTH`, âmbar `BLOCKED`, vermelho `VIOLATED`. A paleta dos dois
protocolos (índigo HTLC, verde-água 2PC) foi validada para daltonismo nos dois
temas.
