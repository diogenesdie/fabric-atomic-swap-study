#!/usr/bin/env python3
"""
Evolui a apresentação do seminário, respondendo às sete perguntas do professor.

Parte da apresentação existente (que já traz o tema e a identidade visual) e
acrescenta os blocos de seminário e experimentos. O conteúdo vive aqui, não no
arquivo binário: corrigir um número é editar uma linha e regerar.

Uso:
    ./.venv/bin/python docs/seminario/build_deck.py

Lê  seminario-cross-chain-final.pptx
Grava seminario-cross-chain-v2.pptx
"""

import copy
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from pptx import Presentation
from pptx.util import Inches, Pt

from deck_style import (ACCENT, BAD, BLOCKED, BODY, CONTENT_W, FAINT, INK, LINE,
                        MARGIN, MIDNAVY, MUTED, NEUTRAL, OK, WASH, WHITE,
                        badge, bullets, footnote, item, label, panel, style,
                        textbox, title)

ROOT = Path(__file__).resolve().parents[2]
SRC = ROOT / "seminario-cross-chain-final.pptx"
DST = ROOT / "seminario-cross-chain-v2.pptx"


def blank(prs):
    return prs.slides.add_slide(prs.slide_layouts[0])


def cite(slide, left, top, width, texto, fonte):
    """Caixa de citação: a referência em destaque, com o veículo abaixo."""
    h = Inches(1.05)
    panel(slide, left, top, width, h, fill=WASH)
    _, tf = textbox(slide, left + Inches(0.28), top + Inches(0.18),
                    width - Inches(0.56), Inches(0.5))
    p = tf.paragraphs[0]
    p.line_spacing = 1.15
    r = p.add_run(); r.text = texto
    style(r, size=14, color=INK, bold=True)
    _, sf = textbox(slide, left + Inches(0.28), top + Inches(0.68),
                    width - Inches(0.56), Inches(0.3))
    r = sf.paragraphs[0].add_run(); r.text = fonte
    style(r, size=11.5, color=MUTED)


def table(slide, left, top, width, headers, rows, widths=None, sizes=(10.5, 10)):
    """Tabela enxuta: sem grade pesada, alinhada ao resto do deck."""
    nrows, ncols = len(rows) + 1, len(headers)
    height = Inches(0.34) * nrows
    shape = slide.shapes.add_table(nrows, ncols, left, top, width, height)
    tbl = shape.table
    tbl.first_row = True
    tbl.horz_banding = False

    if widths:
        total = sum(widths)
        for i, w in enumerate(widths):
            tbl.columns[i].width = Inches(width.inches * w / total)

    for c, h in enumerate(headers):
        cell = tbl.cell(0, c)
        cell.text = ""
        cell.fill.solid(); cell.fill.fore_color.rgb = INK
        cell.margin_left = cell.margin_right = Inches(0.1)
        cell.margin_top = cell.margin_bottom = Inches(0.04)
        r = cell.text_frame.paragraphs[0].add_run(); r.text = h
        style(r, size=sizes[0], color=WHITE, bold=True)

    for i, row in enumerate(rows, 1):
        for c, val in enumerate(row):
            cell = tbl.cell(i, c)
            cell.text = ""
            cell.fill.solid()
            cell.fill.fore_color.rgb = WHITE if i % 2 else WASH
            cell.margin_left = cell.margin_right = Inches(0.1)
            cell.margin_top = cell.margin_bottom = Inches(0.03)
            p = cell.text_frame.paragraphs[0]
            p.line_spacing = 1.05
            colour = BODY
            bold = False
            txt = val
            # marcação: |cor| no início pinta a célula (desfechos)
            if val.startswith("|"):
                tag, txt = val[1:].split("|", 1)
                colour = {"ok": OK, "bad": BAD, "blk": BLOCKED,
                          "ink": INK, "mut": MUTED}[tag]
                bold = tag in ("ok", "bad", "blk")
            for j, part in enumerate(txt.split("**")):
                if not part:
                    continue
                r = p.add_run(); r.text = part
                style(r, size=sizes[1], color=INK if j % 2 else colour,
                      bold=bold or bool(j % 2))
    return shape


# ═══════════════════════════════════════════════════════ SEMINÁRIO

def slide_ref_principal(prs):
    s = blank(prs)
    title(s, "Seminário · referência principal",
          "A que estrutura a apresentação e da qual saem as perguntas de pesquisa")

    cite(s, MARGIN, Inches(1.75), CONTENT_W,
         "Atomic Commitment Across Blockchains",
         "ZAKHARY, V.; AGRAWAL, D.; EL ABBADI, A.  ·  Proceedings of the VLDB "
         "Endowment, v. 13, n. 9, p. 1319–1331, 2020  ·  doi:10.14778/3397230.3397231")

    label(s, MARGIN, Inches(3.05), "por que esta")
    y = bullets(s, MARGIN, Inches(3.35), CONTENT_W, [
        "**Cobre as duas famílias.** Propõe um protocolo de commit atômico (AC3WN) e, "
        "para justificá-lo, disseca o HTLC — dá o seminário inteiro em um só texto.",
        "**Ancora o tema na tradição de bancos de dados.** Publicada em PVLDB, liga o "
        "problema cross-chain à linhagem de Gray e Skeen, que é de onde o commit atômico vem.",
        "**Explicita o modelo de falha,** com o contraexemplo do timelock expirado.",
        "**Sua tese central é falsificável** — e é exatamente o que os experimentos testam.",
    ])

    panel(s, MARGIN, y + Inches(0.15), CONTENT_W, Inches(0.95),
          fill=WHITE, border=LINE)
    _, tf = textbox(s, MARGIN + Inches(0.28), y + Inches(0.32),
                    CONTENT_W - Inches(0.56), Inches(0.7))
    p = tf.paragraphs[0]; p.line_spacing = 1.2
    for i, part in enumerate(
            "Alternativas consideradas: **Herlihy (PODC 2018)** fundamenta o HTLC, mas "
            "cobre só uma família; **Engel, Herlihy & Xue (SSS 2021)** faz precisamente a "
            "nossa comparação, porém de forma analítica e como artigo convidado — entra "
            "como referência de posicionamento, não como eixo.".split("**")):
        if part:
            r = p.add_run(); r.text = part
            style(r, size=12, color=INK if i % 2 else BODY, bold=bool(i % 2))


def slide_refs_adicionais(prs):
    s = blank(prs)
    title(s, "Seminário · referências adicionais",
          "Organizadas pelo papel que cumprem na apresentação")

    grupos = [
        ("Fundamentos do commit atômico", [
            "GRAY (1978) — origem do 2PC e do problema de bloqueio",
            "SKEEN (1981) — 3PC e a impossibilidade sob particionamento",
            "BERNSTEIN, HADZILACOS & GOODMAN (1978), cap. 7 — propriedades AC1–AC5",
            "GRAY & LAMPORT (TODS, 2006) — Paxos Commit: coordenador replicado",
        ]),
        ("HTLC: origem e formalização", [
            "NOLAN (2013) — a construção original, em fórum",
            "HERLIHY (PODC, 2018) — formalização como grafo de dependências",
            "HERLIHY, LISKOV & SHRIRA (VLDB J., 2022) — generalização para deals",
        ]),
        ("Limites e ataques", [
            "XUE & HERLIHY (PODC, 2021) — custo de lock-up (sore loser)",
            "TSABARY et al. (S&P, 2021) — MAD-HTLC: ataque por suborno",
            "ZAMYATIN et al. (FC, 2021) — SoK: impossibilidade sem terceiro confiável",
        ]),
        ("Comparação e posicionamento", [
            "ENGEL, HERLIHY & XUE (SSS, 2021) — do 2PC ao HTLC, analiticamente",
            "TAO, LI & LI (TKDE, 2024) — atomicidade permissionada sob falhas",
            "LU, JAJOO & NAMJOSHI (ACM DIN, 2024) — protocolo de duas fases multi-cadeia",
        ]),
        ("Padrão e implementação", [
            "IETF — draft-ietf-satp-core: Stage 3 é um 2PC entre gateways",
            "HYPERLEDGER CACTI / WEAVER — HTLC de referência para Fabric",
        ]),
        ("Plataforma e metodologia", [
            "ANDROULAKI et al. (EuroSys, 2018) — arquitetura do Fabric",
            "THAKKAR et al. (MASCOTS, 2018) — metodologia de desempenho",
            "HAJDU et al. (IEEE Access, 2020) — injeção de falhas em Fabric",
        ]),
    ]

    col_w = Inches(5.85)
    for i, (titulo, itens) in enumerate(grupos):
        col = i % 2
        row = i // 2
        left = MARGIN + col * Inches(6.18)
        top = Inches(1.72) + row * Inches(1.78)
        label(s, left, top, titulo, color=ACCENT, size=10)
        y = top + Inches(0.28)
        for it in itens:
            _, tf = textbox(s, left, y, col_w, Inches(0.28))
            p = tf.paragraphs[0]; p.line_spacing = 1.1
            autor, _, resto = it.partition(" — ")
            r = p.add_run(); r.text = autor
            style(r, size=10.5, color=INK, bold=True)
            if resto:
                r = p.add_run(); r.text = " — " + resto
                style(r, size=10.5, color=BODY)
            y += Inches(0.33)

    footnote(s, "Entradas completas, com DOI, em docs/paper/refs.bib — 42 referências "
                "conferidas contra Crossref, DBLP e arXiv.")


def slide_estrutura_a(prs):
    s = blank(prs)
    title(s, "Seminário · estrutura (1/2)",
          "Parte A — conhecimentos da área  ·  ~40% do tempo")

    y = Inches(1.85)
    for n, (h, b) in enumerate([
        ("Transações distribuídas e commit atômico",
         "Propriedades AC1–AC5. A distinção que organiza tudo: commit atômico não é "
         "consenso — consenso decide por quórum, commit exige unanimidade, e uma única "
         "recusa aborta tudo."),
        ("2PC clássico e o problema do bloqueio",
         "Fases de preparo e efetivação, o log durável, e a incerteza do participante "
         "quando o coordenador cai entre as fases. Skeen (1981): nenhum protocolo de "
         "commit é não-bloqueante sob particionamento."),
        ("O salto para o cenário cross-chain",
         "Por que não existe coordenador confiável entre domínios sem confiança mútua, e "
         "por que a impossibilidade de comunicação sem terceiro confiável (Zamyatin, 2021) "
         "força cada família a uma escolha diferente."),
        ("A plataforma: Hyperledger Fabric",
         "Execute-order-validate, canais e redes, e três restrições que moldam qualquer "
         "implementação: invocação entre canais é somente leitura, não há temporizador em "
         "chaincode, e a latência é dominada pelo corte de bloco."),
    ], start=1):
        y = item(s, y, str(n), h, b) + Inches(0.24)

    footnote(s, "Objetivo da parte A: que a turma saia sabendo por que o problema é "
                "difícil antes de ver as soluções.")


def slide_estrutura_b(prs):
    s = blank(prs)
    title(s, "Seminário · estrutura (2/2)",
          "Parte B — aprofundamento do tema  ·  ~60% do tempo")

    y = Inches(1.85)
    for n, (h, b) in enumerate([
        ("HTLC em detalhe",
         "Hashlock e timelock, o protocolo de quatro passos, e por que os dois prazos "
         "precisam ser diferentes: quem revela o segredo primeiro fica em desvantagem e "
         "precisa de folga para a contraparte agir."),
        ("A crítica de Zakhary ao HTLC",
         "Um timelock expirado pode violar a atomicidade mesmo sem má-fé, quando um "
         "participante honesto não consegue executar a tempo. É a motivação declarada "
         "do AC3WN."),
        ("2PC cross-chain",
         "Escrow no lugar do lock de banco de dados, coordenador necessariamente externo, "
         "e o padrão IETF SATP, cujo Stage 3 é literalmente um 2PC entre gateways."),
    ], start=5):
        y = item(s, y, str(n), h, b) + Inches(0.2)

    # A ponte para os experimentos — o slide inteiro existe por causa disto.
    top = y + Inches(0.12)
    panel(s, MARGIN, top, CONTENT_W, Inches(1.42), fill=INK)
    badge(s, top + Inches(0.24), "8", fill=ACCENT, left=MARGIN + Inches(0.3))
    _, tf = textbox(s, MARGIN + Inches(1.0), top + Inches(0.22),
                    CONTENT_W - Inches(1.3), Inches(0.32))
    r = tf.paragraphs[0].add_run()
    r.text = "Onde a teoria não decide  →  daí os experimentos"
    style(r, size=17, color=WHITE, bold=True, font="Cambria")

    _, bf = textbox(s, MARGIN + Inches(1.0), top + Inches(0.62),
                    CONTENT_W - Inches(1.3), Inches(0.7))
    p = bf.paragraphs[0]; p.line_spacing = 1.2
    for i, part in enumerate(
            "As duas famílias dependem de hipóteses temporais e ambas as comparações "
            "existentes foram feitas **pelos proponentes de uma delas**, dentro do "
            "argumento de motivação do próprio protocolo. Ninguém as colocou lado a lado, "
            "na mesma plataforma, sob as mesmas falhas, **medindo em vez de argumentar**. "
            "É essa a lacuna — e é ela que define o que os experimentos precisam "
            "responder.".split("**")):
        if part:
            r = p.add_run(); r.text = part
            style(r, size=12.5, color=WHITE if i % 2 else FAINT, bold=bool(i % 2))


# ═══════════════════════════════════════════════════════ EXPERIMENTOS

def slide_aspectos(prs):
    s = blank(prs)
    title(s, "Experimentos · o que se deseja investigar",
          "Cinco aspectos, cada um derivado de uma afirmação da literatura que não foi medida")

    y = Inches(1.9)
    for n, (h, b) in enumerate([
        ("Preservação da atomicidade sob falha",
         "Sob quais falhas cada protocolo deixa algum lado efetivado e o outro não. "
         "É a propriedade de segurança, e a variável dependente central."),
        ("Bloqueio de recursos",
         "Quanto tempo o capital fica imobilizado em cada protocolo, e em que condições "
         "o bloqueio deixa de ter prazo definido."),
        ("Latência e custo de execução",
         "Latência fim a fim, número de escritas no ledger e quantas rodadas de bloco "
         "cada protocolo consome no caminho crítico."),
        ("Sensibilidade às hipóteses temporais",
         "Atraso de rede e dessincronia de relógio — as premissas de que o HTLC "
         "notoriamente depende, e das quais o 2PC supostamente escapa."),
        ("Recuperabilidade",
         "Se o log durável do coordenador permite retomar a decisão correta, e se o "
         "aborto por prazo devolve os ativos sem quebrar a atomicidade."),
    ], start=1):
        y = item(s, y, str(n), h, b) + Inches(0.16)


def slide_rqs(prs):
    s = blank(prs)
    title(s, "Experimentos · perguntas de pesquisa",
          "Formuladas para admitir resposta empírica — cada uma com um critério objetivo de decisão")

    rqs = [
        ("RQ1", "Sob quais condições de falha cada protocolo deixa de preservar a atomicidade?",
         "Resposta: conjunto de cenários com desfecho VIOLATED, por protocolo."),
        ("RQ2", "Qual o custo de bloqueio que cada protocolo cobra para preservá-la?",
         "Resposta: tempo de capital imobilizado, por cenário e por protocolo."),
        ("RQ3", "No caso sem falha, qual a diferença de latência e custo — e de onde ela vem?",
         "Resposta: latência e nº de escritas, com um braço de controle que isola o paralelismo."),
        ("RQ4", "Como cada protocolo responde à degradação temporal?",
         "Resposta: desfecho e latência sob atraso de rede injetado e sob desvio de relógio."),
        ("RQ5", "A válvula de escape contra o bloqueio do 2PC preserva a atomicidade?",
         "Resposta: desfecho quando o aborto por prazo dispara em um só participante."),
    ]

    y = Inches(1.95)
    for tag, pergunta, criterio in rqs:
        panel(s, MARGIN, y, CONTENT_W, Inches(0.86), fill=WASH)
        _, tf = textbox(s, MARGIN + Inches(0.26), y + Inches(0.14), Inches(0.75), Inches(0.3))
        r = tf.paragraphs[0].add_run(); r.text = tag
        style(r, size=13, color=ACCENT, bold=True)
        _, qf = textbox(s, MARGIN + Inches(1.05), y + Inches(0.12),
                        CONTENT_W - Inches(1.35), Inches(0.32))
        r = qf.paragraphs[0].add_run(); r.text = pergunta
        style(r, size=13.5, color=INK, bold=True)
        _, cf = textbox(s, MARGIN + Inches(1.05), y + Inches(0.47),
                        CONTENT_W - Inches(1.35), Inches(0.28))
        r = cf.paragraphs[0].add_run(); r.text = criterio
        style(r, size=11.5, color=BODY)
        y += Inches(0.98)


def slide_metodologia_sistema(prs):
    s = blank(prs)
    title(s, "Metodologia · sistema e parametrização",
          "O que é fixo, o que varia, e o que precisa ser declarado no relato")

    label(s, MARGIN, Inches(1.85), "tamanho do sistema  ·  fixo")
    bullets(s, MARGIN, Inches(2.15), Inches(5.8), [
        "**Duas redes Fabric 2.5.16 independentes** — não dois canais: canais "
        "compartilham o serviço de ordenação, o que impediria falhar uma cadeia sem afetar a outra.",
        "Por rede: **1 organização, 1 peer, 1 orderer, 1 CA**, LevelDB.",
        "**BatchTimeout = 2 s**, no padrão. Reduzi-lo comprimiria as duas curvas e "
        "esconderia justamente o efeito de interesse.",
        "Ativos: **1 não-fungível** (rede 1) e **1 fungível** (rede 2).",
    ], size=11.5, gap=Inches(0.32))

    label(s, Inches(6.83), Inches(1.85), "parâmetros  ·  variam")
    bullets(s, Inches(6.83), Inches(2.15), Inches(5.85), [
        "**Prazos do HTLC:** T1 (quem inicia) e T2 (quem responde), com T1 > T2.",
        "**Prazo do 2PC:** o limite após o qual o participante pode abortar sozinho.",
        "**Atraso de rede:** 0 ou 800 ms, via tc/netem, com duração controlada.",
        "**Desvio de relógio:** −20 s a +40 s no participante que responde.",
        "**Fases do 2PC:** paralelas ou sequenciais — o braço de controle.",
        "**Ponto de interrupção:** onde o processo é morto no protocolo.",
    ], size=11.5, gap=Inches(0.32))

    footnote(s, "Toda execução recebe um ativo virgem: recriar as redes levaria ~3 min por "
                "repetição, o que inviabilizaria N ≥ 10.")


def slide_metodologia_metricas(prs):
    s = blank(prs)
    title(s, "Metodologia · métricas e classificação",
          "O desfecho é lido dos dois ledgers, nunca reportado pelo cliente")

    label(s, MARGIN, Inches(1.8), "classificação do desfecho")
    table(s, MARGIN, Inches(2.1), Inches(5.9),
          ["Desfecho", "Significado"],
          [["|ok|COMMITTED_BOTH", "os dois lados efetivaram"],
           ["|mut|ABORTED_BOTH", "nenhum efetivou — aborto correto"],
           ["|bad|VIOLATED", "só um lado efetivou — falha de **segurança**"],
           ["|blk|BLOCKED", "ativo preso sem resolução — falha de **vivacidade**"]],
          widths=[1.05, 2.0])

    label(s, Inches(6.83), Inches(1.8), "métricas observadas")
    bullets(s, Inches(6.83), Inches(2.1), Inches(5.85), [
        "**Atomicidade** — contagem de VIOLATED por cenário.",
        "**Tempo de bloqueio** — do travamento até a devolução ou efetivação.",
        "**Latência do protocolo** — janela do primeiro ao último passo.",
        "**Custo** — número de escritas submetidas ao ledger.",
    ], size=11.5, gap=Inches(0.34))

    top = Inches(4.35)
    panel(s, MARGIN, top, CONTENT_W, Inches(1.55), fill=WHITE, border=LINE)
    label(s, MARGIN + Inches(0.3), top + Inches(0.2), "tratamentos necessários", color=BAD)
    bullets(s, MARGIN + Inches(0.3), top + Inches(0.52), CONTENT_W - Inches(0.6), [
        "**Latência é janela, não soma.** Somar as durações contaria duas vezes as fases "
        "paralelas do 2PC e apagaria exatamente a vantagem que se quer medir.",
        "**Conflito MVCC não é aborto de protocolo** — contá-los juntos inflaria a taxa "
        "de aborto do 2PC.",
        "**Mediana e desvio-padrão**, não média: distribuições com cauda por retentativa.",
    ], size=11.5, gap=Inches(0.3))


def slide_execucoes_graficos(prs):
    s = blank(prs)
    title(s, "Metodologia · execuções e gráficos",
          "Quantas vezes, e o que cada figura precisa mostrar")

    label(s, MARGIN, Inches(1.82), "número de execuções")
    bullets(s, MARGIN, Inches(2.12), Inches(5.8), [
        "**N = 10 por cenário**, 13 cenários — 130 execuções por rodada completa.",
        "N = 10 é o mínimo para falar de **reprodutibilidade**; abaixo disso só se "
        "demonstra que um desfecho é possível.",
        "Estado limpo entre execuções, garantido por ativo virgem e verificação do "
        "estado inicial antes de começar.",
        "O harness **confere que a falha está ativa** antes de executar e aborta se não "
        "estiver — um cenário que roda sem a falha injetada é o pior tipo de defeito.",
    ], size=11.5, gap=Inches(0.34))

    label(s, Inches(6.83), Inches(1.82), "gráficos previstos")
    figs = [
        ("Barras empilhadas de desfecho por cenário",
         "a variável dependente central; cor codifica desfecho"),
        ("Boxplot de latência, HTLC vs 2PC",
         "mostra a diferença e a dispersão lado a lado"),
        ("Barras de tempo de bloqueio por cenário",
         "o custo que a atomicidade cobra"),
        ("Tabela desfecho × cenário com mediana e desvio",
         "o dado bruto que sustenta as três figuras"),
    ]
    y = Inches(2.12)
    for i, (t, d) in enumerate(figs, 1):
        badge(s, y, chr(0x40 + i), fill=MIDNAVY, left=Inches(6.83), size=Inches(0.34))
        _, tf = textbox(s, Inches(7.31), y - Inches(0.02), Inches(5.4), Inches(0.28))
        r = tf.paragraphs[0].add_run(); r.text = t
        style(r, size=12, color=INK, bold=True)
        _, df = textbox(s, Inches(7.31), y + Inches(0.26), Inches(5.4), Inches(0.28))
        r = df.paragraphs[0].add_run(); r.text = d
        style(r, size=11, color=BODY)
        y += Inches(0.78)

    footnote(s, "Pipeline automatizado: o runner agrega os CSVs e a análise gera tabelas "
                "(Markdown e LaTeX) e figuras a partir do mesmo agregado.")


def slide_matriz(prs):
    s = blank(prs)
    title(s, "Experimentos · matriz de cenários",
          "Treze cenários. Os pares em destaque são os que sustentam a comparação")

    rows = [
        ["B1", "HTLC", "sem falha (baseline)", "|ok|COMMITTED_BOTH"],
        ["B2", "2PC", "sem falha, fases paralelas", "|ok|COMMITTED_BOTH"],
        ["B3", "2PC", "sem falha, fases sequenciais — **controle**", "|ok|COMMITTED_BOTH"],
        ["H1", "HTLC", "contraparte nunca trava o próprio ativo", "|mut|ABORTED_BOTH"],
        ["H2", "HTLC", "segredo revelado e ninguém resgata", "|bad|VIOLATED"],
        ["H3", "HTLC", "atraso de rede consome a margem", "|mut|ABORTED_BOTH"],
        ["X1", "HTLC", "relógio da contraparte adiantado", "|mut|ABORTED_BOTH"],
        ["X2", "HTLC", "relógio da contraparte atrasado", "|ok|COMMITTED_BOTH"],
        ["T1", "2PC", "um participante vota não", "|mut|ABORTED_BOTH"],
        ["T2", "2PC", "coordenador morto entre as fases", "|blk|BLOCKED"],
        ["T2r", "2PC", "o mesmo, recuperado pelo log durável", "|ok|COMMITTED_BOTH"],
        ["T3", "2PC", "atraso de rede nas duas fases", "|ok|COMMITTED_BOTH"],
        ["T4", "2PC", "prazo dispara em um só participante", "|bad|VIOLATED"],
    ]
    table(s, MARGIN, Inches(1.9), CONTENT_W,
          ["Cen.", "Prot.", "Situação", "Desfecho esperado"],
          rows, widths=[0.5, 0.55, 3.4, 1.5], sizes=(10, 9.5))

    footnote(s, "Pares centrais — H3 × T3: mesma falha, respostas diferentes. "
                "B2 × B3: isola de onde vem a vantagem de latência do 2PC.")


def slide_mapeamento(prs):
    s = blank(prs)
    title(s, "Como os resultados respondem às perguntas",
          "Cada pergunta tem cenários designados e um critério de decisão declarado antes de medir")

    rows = [
        ["RQ1", "H1 H2 H3 · T1 T2 T4",
         "Conjunto de cenários com desfecho VIOLATED em cada braço",
         "**H2** e **T4** violam; os demais preservam"],
        ["RQ2", "H1 H2 H3 · T2 T2r",
         "Tempo de bloqueio por cenário, comparado ao baseline",
         "HTLC bloqueia por prazo definido; **T2 bloqueia sem prazo**"],
        ["RQ3", "B1 · B2 · B3",
         "Latência e nº de escritas; B3 isola o efeito do paralelismo",
         "2PC **2× mais rápido**, e B3 mostra que é paralelismo"],
        ["RQ4", "H3 · T3 · X1 X2",
         "Desfecho e latência sob atraso e sob desvio de relógio",
         "Atraso: HTLC **aborta**, 2PC só **desacelera**"],
        ["RQ5", "T2 · T2r · T4",
         "Desfecho quando o aborto por prazo dispara assimetricamente",
         "**Não preserva** — a válvula custa a segurança"],
    ]
    table(s, MARGIN, Inches(2.0), CONTENT_W,
          ["", "Cenários", "Critério de decisão", "O que o piloto já indica"],
          rows, widths=[0.4, 1.5, 2.9, 2.6], sizes=(10, 9.5))

    footnote(s, "A última coluna traz o resultado do piloto (N = 10). O critério de decisão "
                "foi fixado antes das medições.")


def slide_resultados(prs):
    s = blank(prs)
    title(s, "Resultados preliminares do piloto",
          "N = 10 por cenário · 130 execuções · medianas")

    rows = [
        ["B1", "HTLC", "|ok|COMMITTED_BOTH", "8.150", "6,1", "4"],
        ["B2", "2PC", "|ok|COMMITTED_BOTH", "4.070", "2,0", "4"],
        ["B3", "2PC", "|ok|COMMITTED_BOTH", "8.134", "4,1", "4"],
        ["H2", "HTLC", "|bad|VIOLATED", "36.178", "34,1", "4"],
        ["H3", "HTLC", "|mut|ABORTED_BOTH", "44.937", "35,1", "2"],
        ["T2", "2PC", "|blk|BLOCKED", "2.042", "sem prazo", "2"],
        ["T2r", "2PC", "|ok|COMMITTED_BOTH", "2.025", "24,6", "2"],
        ["T3", "2PC", "|ok|COMMITTED_BOTH", "19.354", "6,9", "4"],
        ["T4", "2PC", "|bad|VIOLATED", "2.026", "30,8", "1"],
    ]
    table(s, MARGIN, Inches(1.95), Inches(7.4),
          ["Cen.", "Prot.", "Desfecho", "Lat. (ms)", "Bloq. (s)", "Escr."],
          rows, widths=[0.5, 0.55, 1.6, 0.8, 0.85, 0.5], sizes=(9.5, 9))

    left = Inches(8.35)
    panel(s, left, Inches(1.95), Inches(4.33), Inches(2.55), fill=INK)
    label(s, left + Inches(0.26), Inches(2.16), "o achado que muda a hipótese",
          color=ACCENT, size=9.5)
    _, tf = textbox(s, left + Inches(0.26), Inches(2.5), Inches(3.8), Inches(1.9))
    p = tf.paragraphs[0]; p.line_spacing = 1.25
    for i, part in enumerate(
            "Esperávamos que o HTLC violasse a atomicidade e o 2PC apenas bloqueasse. "
            "A primeira metade se confirmou; **a segunda não**. Em T4, o mecanismo que o "
            "2PC usa para escapar do bloqueio faz com que ele viole exatamente como o "
            "HTLC. **Os dois quebram do mesmo jeito** — o que compra vivacidade é o que "
            "custa a segurança.".split("**")):
        if part:
            r = p.add_run(); r.text = part
            style(r, size=12, color=WHITE if i % 2 else FAINT, bold=bool(i % 2))

    panel(s, left, Inches(4.68), Inches(4.33), Inches(1.75), fill=WHITE, border=LINE)
    label(s, left + Inches(0.26), Inches(4.88), "ressalva sobre o N", color=BAD, size=9.5)
    _, tf = textbox(s, left + Inches(0.26), Inches(5.2), Inches(3.8), Inches(1.3))
    p = tf.paragraphs[0]; p.line_spacing = 1.25
    for i, part in enumerate(
            "N = 10 mostra que cada resultado é **reprodutível**, mas as proporções não "
            "são taxas naturais de falha: refletem a matriz que escolhemos. O estudo mede "
            "**sob que condições** cada protocolo quebra, não com que frequência isso "
            "ocorre em produção.".split("**")):
        if part:
            r = p.add_run(); r.text = part
            style(r, size=11.5, color=INK if i % 2 else BODY, bold=bool(i % 2))

    footnote(s, "Desvio-padrão da latência entre 11 e 22 ms nos cenários sem falha: "
                "a diferença de 2× entre B1 e B2 está muito acima do ruído da plataforma.")


# ═══════════════════════════════════════════════════ edição do existente

def fix_existing(prs):
    """Corrige fatos que mudaram desde a versão de planejamento."""
    trocas = {
        "Dois": "Duas",
        "canais": "redes",
        "/ledgers no Hyperledger Fabric": "/ledgers independentes no Hyperledger Fabric",
        "Hyperledger Fabric — blockchains permissionadas, com canais que permitem "
        "instanciar múltiplas cadeias no mesmo ambiente.":
            "Hyperledger Fabric — duas redes independentes, cada uma com peer, orderer "
            "e CA próprios, para que falhas possam ser injetadas em uma sem afetar a outra.",
    }
    for slide in prs.slides:
        for shape in slide.shapes:
            if not shape.has_text_frame:
                continue
            for para in shape.text_frame.paragraphs:
                for run in para.runs:
                    if run.text in trocas:
                        run.text = trocas[run.text]


def reorder(prs, order):
    """Reordena os slides manipulando a lista de ids da apresentação."""
    lst = prs.slides._sldIdLst
    ids = list(lst)
    for i in ids:
        lst.remove(i)
    for idx in order:
        lst.append(ids[idx])


def main():
    prs = Presentation(str(SRC))
    n_orig = len(prs.slides._sldIdLst)

    fix_existing(prs)

    for build in (slide_ref_principal, slide_refs_adicionais,
                  slide_estrutura_a, slide_estrutura_b,
                  slide_aspectos, slide_rqs,
                  slide_metodologia_sistema, slide_metodologia_metricas,
                  slide_execucoes_graficos, slide_matriz,
                  slide_mapeamento, slide_resultados):
        build(prs)

    # Original: 0 capa, 1 problema, 2 justificativa, 3 famílias 1/2,
    #           4 famílias 2/2, 5 nosso experimento, 6 plano, 7 referências
    novos = list(range(n_orig, n_orig + 12))
    reorder(prs, [0, 1, 2, 3, 4, 5] + novos + [6, 7])

    prs.save(str(DST))
    print(f"  ✓ {DST.name}: {len(prs.slides._sldIdLst)} slides "
          f"({n_orig} originais + 12 novos)")


if __name__ == "__main__":
    main()
