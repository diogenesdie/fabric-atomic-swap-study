#!/usr/bin/env python3
"""
Análise dos experimentos: tabelas e figuras do artigo.

Lê experiments/results/aggregated.csv e escreve em docs/paper/figures/:

  outcomes.pdf   desfecho por cenário (a variável dependente central)
  latency.pdf    latência do protocolo, HTLC vs 2PC
  blocking.pdf   tempo de capital imobilizado por cenário
  results.tex    tabela pronta para o artigo
  results.md     a mesma tabela, para conferir no terminal

Uso: analyze.py [caminho/do/aggregated.csv]
"""

import csv
import statistics
import sys
from collections import Counter, defaultdict
from pathlib import Path

try:
    import matplotlib
    matplotlib.use("Agg")  # sem display: rodamos em terminal
    import matplotlib.pyplot as plt
except ImportError:
    plt = None

# Ordem dos cenários: baselines, depois HTLC, depois 2PC, depois clock skew.
# Não é alfabética — segue o argumento do artigo.
ORDER = ["B1", "B2", "B3", "H1", "H2", "H3", "X1", "X2",
         "T1", "T2", "T2r", "T3", "T4"]

# Cor por desfecho. É informação, não decoração: verde efetiva, cinza aborta,
# âmbar bloqueia, vermelho viola.
OUTCOME_COLOR = {
    "COMMITTED_BOTH": "#2E7D4F",
    "ABORTED_BOTH":   "#7A8894",
    "BLOCKED":        "#B5811F",
    "VIOLATED":       "#AE382C",
    "ERROR":          "#4A4A4A",
    "NO_CSV":         "#4A4A4A",
}

LABEL = {
    "B1": "B1 HTLC base",       "B2": "B2 2PC base",
    "B3": "B3 2PC sequencial",  "H1": "H1 ausente",
    "H2": "H2 segredo vazou",   "H3": "H3 atraso",
    "X1": "X1 relógio +",       "X2": "X2 relógio −",
    "T1": "T1 voto não",        "T2": "T2 coord. morto",
    "T2r": "T2r recuperado",    "T3": "T3 atraso",
    "T4": "T4 prazo assimétrico",
}


def load(path: Path) -> list[dict]:
    with path.open(encoding="utf-8") as f:
        return list(csv.DictReader(f))


def num(row: dict, key: str):
    try:
        return float((row.get(key) or "").strip())
    except ValueError:
        return None


def by_scenario(rows: list[dict]) -> dict[str, list[dict]]:
    groups: dict[str, list[dict]] = defaultdict(list)
    for r in rows:
        groups[r["scenario"]].append(r)
    return groups


def ordered(groups: dict[str, list[dict]]) -> list[str]:
    known = [s for s in ORDER if s in groups]
    return known + sorted(set(groups) - set(known))


def series(runs: list[dict], key: str) -> list[float]:
    return [v for v in (num(r, key) for r in runs) if v is not None]


# ------------------------------------------------------------------- tabelas

def build_table(groups) -> list[dict]:
    table = []
    for s in ordered(groups):
        runs = groups[s]
        outcomes = Counter(r["outcome"] for r in runs)
        lat = series(runs, "protocol_latency_ms")
        lock = series(runs, "lock_duration_ms")
        txs = series(runs, "transactions")
        table.append({
            "scenario": s,
            "protocol": runs[0]["protocol"].upper(),
            "n": len(runs),
            "outcome": outcomes.most_common(1)[0][0],
            "consistent": len(outcomes) == 1,
            "outcomes": outcomes,
            "latency": statistics.median(lat) if lat else None,
            "latency_sd": statistics.stdev(lat) if len(lat) > 1 else 0.0,
            "lock": statistics.median(lock) if lock else None,
            "txs": statistics.median(txs) if txs else None,
        })
    return table


def write_markdown(table, path: Path):
    lines = [
        "# Resultados",
        "",
        "Mediana de N execuções por cenário. A coluna *desfecho* traz o resultado",
        "observado; quando houve mais de um, todos são listados com a contagem.",
        "",
        "| Cenário | Protocolo | N | Desfecho | Latência (ms) | Bloqueio (s) | Escritas |",
        "|---|---|---:|---|---:|---:|---:|",
    ]
    for r in table:
        out = (r["outcome"] if r["consistent"]
               else " ".join(f"{o}×{c}" for o, c in r["outcomes"].most_common()))
        lat = f"{r['latency']:.0f}" if r["latency"] is not None else "—"
        lock = f"{r['lock']/1000:.1f}" if r["lock"] is not None else "—"
        txs = f"{r['txs']:.0f}" if r["txs"] is not None else "—"
        lines.append(f"| {r['scenario']} | {r['protocol']} | {r['n']} | "
                     f"{out} | {lat} | {lock} | {txs} |")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_latex(table, path: Path):
    rows = []
    for r in table:
        out = (r["outcome"] if r["consistent"]
               else "/".join(o for o in r["outcomes"]))
        lat = f"{r['latency']:.0f}" if r["latency"] is not None else "--"
        lock = f"{r['lock']/1000:.1f}" if r["lock"] is not None else "--"
        txs = f"{r['txs']:.0f}" if r["txs"] is not None else "--"
        rows.append(f"  {r['scenario']} & {r['protocol']} & {r['n']} & "
                    f"\\textsc{{{out.replace('_', '\\_').lower()}}} & "
                    f"{lat} & {lock} & {txs} \\\\")

    path.write_text(
        "% Gerado por analysis/analyze.py — não editar à mão.\n"
        "\\begin{table}[t]\n  \\centering\n"
        "  \\caption{Desfecho e custo por cenário. Medianas de $N$ execuções.}\n"
        "  \\label{tab:results}\n"
        "  \\begin{tabular}{llrlrrr}\n    \\toprule\n"
        "    Cen. & Prot. & $N$ & Desfecho & Lat. (ms) & Bloq. (s) & Escritas \\\\\n"
        "    \\midrule\n" + "\n".join(rows) +
        "\n    \\bottomrule\n  \\end{tabular}\n\\end{table}\n",
        encoding="utf-8")


# ------------------------------------------------------------------- figuras

def fig_outcomes(table, path: Path):
    """Desfecho por cenário — a variável dependente central do estudo."""
    fig, ax = plt.subplots(figsize=(9, 4.2))

    labels = [LABEL.get(r["scenario"], r["scenario"]) for r in table]
    ys = range(len(table))

    seen = set()
    for y, r in zip(ys, table):
        left = 0
        total = sum(r["outcomes"].values())
        for outcome, count in r["outcomes"].most_common():
            frac = count / total
            ax.barh(y, frac, left=left, height=.62,
                    color=OUTCOME_COLOR.get(outcome, "#888"),
                    label=outcome if outcome not in seen else None)
            seen.add(outcome)
            if frac > .18:
                ax.text(left + frac / 2, y, f"{count}", ha="center", va="center",
                        color="white", fontsize=8, fontweight="bold")
            left += frac

    ax.set_yticks(list(ys))
    ax.set_yticklabels(labels, fontsize=9)
    ax.invert_yaxis()
    ax.set_xlim(0, 1)
    ax.set_xlabel("proporção das execuções")
    ax.set_title("Desfecho por cenário", fontsize=11, loc="left", pad=12)
    ax.spines[["top", "right"]].set_visible(False)
    ax.legend(loc="upper center", bbox_to_anchor=(.5, -.16),
              ncol=4, frameon=False, fontsize=8)
    fig.tight_layout()
    fig.savefig(path, bbox_inches="tight")
    plt.close(fig)


def fig_latency(groups, path: Path):
    """Latência do protocolo. Separa os braços para a comparação ficar direta."""
    fig, ax = plt.subplots(figsize=(8, 4))

    scenarios = [s for s in ordered(groups) if series(groups[s], "protocol_latency_ms")]
    data = [series(groups[s], "protocol_latency_ms") for s in scenarios]
    colors = ["#0F5F58" if groups[s][0]["protocol"] == "htlc" else "#A9762F"
              for s in scenarios]

    bp = ax.boxplot(data, patch_artist=True, widths=.6, medianprops=dict(color="white"))
    for patch, c in zip(bp["boxes"], colors):
        patch.set_facecolor(c)
        patch.set_alpha(.85)
        patch.set_edgecolor("none")

    ax.set_xticklabels([LABEL.get(s, s) for s in scenarios],
                       rotation=35, ha="right", fontsize=8)
    ax.set_ylabel("latência do protocolo (ms)")
    ax.set_title("Latência por cenário — HTLC (teal) vs 2PC (ocre)",
                 fontsize=11, loc="left", pad=12)
    ax.spines[["top", "right"]].set_visible(False)
    ax.grid(axis="y", alpha=.25, linewidth=.6)
    ax.set_axisbelow(True)
    fig.tight_layout()
    fig.savefig(path, bbox_inches="tight")
    plt.close(fig)


def fig_blocking(groups, path: Path):
    """Capital imobilizado — o custo que a atomicidade cobra."""
    fig, ax = plt.subplots(figsize=(8, 4))

    scenarios = [s for s in ordered(groups) if series(groups[s], "lock_duration_ms")]
    medians = [statistics.median(series(groups[s], "lock_duration_ms")) / 1000
               for s in scenarios]
    colors = ["#0F5F58" if groups[s][0]["protocol"] == "htlc" else "#A9762F"
              for s in scenarios]

    ax.bar(range(len(scenarios)), medians, color=colors, alpha=.85, width=.65)
    ax.set_xticks(range(len(scenarios)))
    ax.set_xticklabels([LABEL.get(s, s) for s in scenarios],
                       rotation=35, ha="right", fontsize=8)
    ax.set_ylabel("tempo de bloqueio (s)")
    ax.set_title("Capital imobilizado por cenário (mediana)",
                 fontsize=11, loc="left", pad=12)
    ax.spines[["top", "right"]].set_visible(False)
    ax.grid(axis="y", alpha=.25, linewidth=.6)
    ax.set_axisbelow(True)
    fig.tight_layout()
    fig.savefig(path, bbox_inches="tight")
    plt.close(fig)


# ------------------------------------------------------------------ principal

def main() -> int:
    root = Path(__file__).resolve().parent.parent
    path = Path(sys.argv[1]) if len(sys.argv) > 1 \
        else root / "experiments/results/aggregated.csv"
    if not path.is_file():
        print(f"agregado não encontrado: {path}", file=sys.stderr)
        print("rode primeiro: ./experiments/runner.sh --all --repeat=10",
              file=sys.stderr)
        return 1

    rows = load(path)
    if not rows:
        print("nenhuma execução registrada", file=sys.stderr)
        return 1

    groups = by_scenario(rows)
    table = build_table(groups)

    outdir = root / "docs/paper/figures"
    outdir.mkdir(parents=True, exist_ok=True)

    write_markdown(table, outdir / "results.md")
    write_latex(table, outdir / "results.tex")
    print(f"  ✓ tabelas: {outdir/'results.md'}, {outdir/'results.tex'}")

    if plt is None:
        print("  ! matplotlib ausente: figuras não geradas "
              "(pip install -r analysis/requirements.txt)")
    else:
        fig_outcomes(table, outdir / "outcomes.pdf")
        fig_latency(groups, outdir / "latency.pdf")
        fig_blocking(groups, outdir / "blocking.pdf")
        print(f"  ✓ figuras: outcomes.pdf, latency.pdf, blocking.pdf em {outdir}")

    # Os números que o artigo cita no texto.
    print("\n  Números para o texto:")
    med = {r["scenario"]: r for r in table}
    if "B1" in med and "B2" in med and med["B1"]["latency"] and med["B2"]["latency"]:
        print(f"    HTLC sem falha:            {med['B1']['latency']:.0f} ms")
        print(f"    2PC sem falha (paralelo):  {med['B2']['latency']:.0f} ms "
              f"({med['B1']['latency']/med['B2']['latency']:.1f}× mais rápido)")
    if "B3" in med and med["B3"]["latency"]:
        print(f"    2PC sequencial:            {med['B3']['latency']:.0f} ms "
              f"(o controle: sem paralelismo, a vantagem some)")

    for proto in ("htlc", "2pc"):
        runs = [r for r in rows if r["protocol"] == proto]
        if not runs:
            continue
        v = sum(1 for r in runs if r["outcome"] == "VIOLATED")
        b = sum(1 for r in runs if r["outcome"] == "BLOCKED")
        print(f"    {proto.upper():4} — {len(runs)} execuções, "
              f"{v} violações, {b} bloqueios")

    divergent = [r for r in rows if r.get("matched") == "nao"]
    if divergent:
        print(f"\n  ! {len(divergent)} execução(ões) divergiram do esperado")
        for r in divergent[:8]:
            print(f"      {r['run_id']}: {r['outcome']} (esperado {r['expected']})")

    return 0


if __name__ == "__main__":
    sys.exit(main())
