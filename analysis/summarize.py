#!/usr/bin/env python3
"""
Resumo rápido do agregado de execuções, para ler no terminal ao fim do runner.

A análise completa (tabelas e figuras do artigo) fica em analyze.py; aqui só
queremos saber, imediatamente, se os cenários produziram os desfechos esperados.

Uso: summarize.py [experiments/results/aggregated.csv]
"""

import csv
import statistics
import sys
from collections import Counter, defaultdict
from pathlib import Path


def load(path: Path) -> list[dict]:
    with path.open(encoding="utf-8") as f:
        return list(csv.DictReader(f))


def num(row: dict, key: str) -> float | None:
    raw = (row.get(key) or "").strip()
    try:
        return float(raw)
    except ValueError:
        return None


def main() -> int:
    path = Path(sys.argv[1] if len(sys.argv) > 1
                else "experiments/results/aggregated.csv")
    if not path.is_file():
        print(f"agregado não encontrado: {path}", file=sys.stderr)
        return 1

    rows = load(path)
    if not rows:
        print("nenhuma execução registrada")
        return 0

    by_scenario: dict[str, list[dict]] = defaultdict(list)
    for r in rows:
        by_scenario[r["scenario"]].append(r)

    header = f"  {'cenário':<7} {'proto':<5} {'n':>3}  {'desfechos':<34} {'esperado':<15} {'lat.':>8} {'bloqueio':>10}"
    print(header)
    print("  " + "-" * (len(header) - 2))

    for scenario in sorted(by_scenario):
        runs = by_scenario[scenario]
        outcomes = Counter(r["outcome"] for r in runs)
        expected = runs[0].get("expected", "")

        # Desfechos em ordem de frequência, com a contagem.
        summary = " ".join(f"{o}×{c}" for o, c in outcomes.most_common())

        lat = [v for v in (num(r, "protocol_latency_ms") for r in runs) if v]
        lock = [v for v in (num(r, "lock_duration_ms") for r in runs) if v]

        lat_s = f"{statistics.median(lat):.0f}ms" if lat else "-"
        lock_s = f"{statistics.median(lock)/1000:.1f}s" if lock else "-"

        matched = sum(1 for r in runs if r.get("matched") == "sim")
        flag = " " if matched == len(runs) else "!"

        print(f" {flag}{scenario:<7} {runs[0]['protocol']:<5} {len(runs):>3}  "
              f"{summary:<34} {expected:<15} {lat_s:>8} {lock_s:>10}")

    # O que interessa ao artigo: com que frequência cada protocolo falhou, e como.
    print()
    for proto in sorted({r["protocol"] for r in rows}):
        runs = [r for r in rows if r["protocol"] == proto]
        violated = sum(1 for r in runs if r["outcome"] == "VIOLATED")
        blocked = sum(1 for r in runs if r["outcome"] == "BLOCKED")
        total = len(runs)
        print(f"  {proto:<5} {total:>3} execuções   "
              f"violações de atomicidade: {violated} ({violated/total:.0%})   "
              f"bloqueios: {blocked} ({blocked/total:.0%})")

    divergent = [r for r in rows if r.get("matched") == "nao"]
    if divergent:
        print(f"\n  ! {len(divergent)} execução(ões) divergiram do esperado:")
        for r in divergent[:10]:
            print(f"      {r['run_id']}: obteve {r['outcome']}, "
                  f"esperava {r['expected']}")
        if len(divergent) > 10:
            print(f"      ... e mais {len(divergent) - 10}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
