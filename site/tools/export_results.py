#!/usr/bin/env python3
"""
Exporta o agregado dos experimentos para o site do seminário.

Lê experiments/results/aggregated.csv e grava site/assets/results-data.js com a
mediana por cenário (latência, bloqueio, escritas) e a contagem de desfechos.
Assim o site mostra sempre os mesmos números do artigo, sem cópia manual.

Uso:
    python3 site/tools/export_results.py
"""

import csv
import json
import statistics
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SRC = ROOT / "experiments" / "results" / "aggregated.csv"
DST = ROOT / "site" / "assets" / "results-data.js"

# Ordem de apresentação e descrição curta de cada cenário (em português, para
# leigos). O que se mede vem do CSV; o que se explica vive aqui.
SCENARIOS = [
    ("B1", "HTLC sem falha", "Troca completa, ninguém falha. É a referência do HTLC."),
    ("B2", "2PC sem falha, fases em paralelo", "O coordenador fala com as duas redes ao mesmo tempo."),
    ("B3", "2PC sem falha, fases em sequência", "Controle: o mesmo 2PC, mas uma rede por vez. Isola o efeito do paralelismo."),
    ("H1", "HTLC: Bob nunca tranca", "Alice tranca o título, Bob não aparece. O prazo de Alice vence e ela recupera o ativo."),
    ("H2", "HTLC: segredo revelado, Bob não resgata", "Alice já pegou os tokens e revelou o segredo; Bob cai antes de usá-lo. O prazo vence e Alice recupera o título também."),
    ("H3", "HTLC: atraso de rede consome a margem", "800 ms de atraso em cada mensagem. A folga entre os dois prazos fica pequena e o cliente de Bob se recusa a trancar."),
    ("X1", "HTLC: relógio de Bob adiantado", "Bob acha que o prazo está mais perto do que está e recusa a troca. Cauteloso demais."),
    ("X2", "HTLC: relógio de Bob atrasado", "Bob acha que tem mais tempo do que tem — e mesmo assim a troca completa."),
    ("T1", "2PC: um participante vota NÃO", "Uma das redes recusa na fase de preparo. O coordenador aborta nas duas."),
    ("T2", "2PC: coordenador cai entre as fases", "Votos coletados, decisão não comunicada. As duas redes ficam com o ativo em custódia, esperando."),
    ("T2r", "2PC: o mesmo, recuperado pelo diário", "O coordenador reinicia, lê a decisão que gravou antes de cair e termina o que começou."),
    ("T3", "2PC: atraso de rede nas duas fases", "Os mesmos 800 ms do H3. O 2PC só fica mais lento."),
    ("T4", "2PC: prazo dispara em um só participante", "O coordenador cai; uma rede espera, a outra desiste sozinha pelo prazo. Ao voltar, o coordenador efetiva onde ainda pode."),
]


def median(values):
    return statistics.median(values) if values else None


def main() -> int:
    if not SRC.is_file():
        print(f"agregado não encontrado: {SRC}")
        return 1

    with SRC.open(encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    by_scenario = defaultdict(list)
    for r in rows:
        by_scenario[r["scenario"]].append(r)

    out = []
    for code, title, story in SCENARIOS:
        runs = by_scenario.get(code, [])
        if not runs:
            continue
        outcomes = Counter(r["outcome"] for r in runs)
        lat = [float(r["protocol_latency_ms"]) for r in runs if r["protocol_latency_ms"]]
        lock = [float(r["lock_duration_ms"]) for r in runs if r["lock_duration_ms"]]
        writes = [int(r["transactions"]) for r in runs if r["transactions"]]
        outcome, _ = outcomes.most_common(1)[0]
        out.append({
            "code": code,
            "protocol": runs[0]["protocol"],
            "title": title,
            "story": story,
            "n": len(runs),
            "expected": runs[0]["expected"],
            "outcome": outcome,
            "outcomes": dict(outcomes),
            "matched": sum(1 for r in runs if r["matched"] == "sim"),
            # BLOCKED não tem fim: o bloqueio dura enquanto ninguém decidir.
            "lock_s": None if outcome == "BLOCKED" else round(median(lock) / 1000, 1),
            "latency_ms": round(median(lat)),
            "latency_sd_ms": round(statistics.pstdev(lat)) if len(lat) > 1 else 0,
            "writes": int(median(writes)),
        })

    payload = {
        "generated_from": "experiments/results/aggregated.csv",
        "runs": len(rows),
        "scenarios": out,
    }
    DST.write_text(
        "// Gerado por site/tools/export_results.py a partir de "
        "experiments/results/aggregated.csv. Não editar à mão.\n"
        "window.RESULTS = " + json.dumps(payload, ensure_ascii=False, indent=2) + ";\n",
        encoding="utf-8",
    )
    print(f"  ✓ {DST.relative_to(ROOT)}: {len(out)} cenários, {len(rows)} execuções")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
