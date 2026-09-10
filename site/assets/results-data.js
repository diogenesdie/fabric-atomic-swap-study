// Gerado por site/tools/export_results.py a partir de experiments/results/aggregated.csv. Não editar à mão.
window.RESULTS = {
  "generated_from": "experiments/results/aggregated.csv",
  "runs": 130,
  "scenarios": [
    {
      "code": "B1",
      "protocol": "htlc",
      "title": "HTLC sem falha",
      "story": "Troca completa, ninguém falha. É a referência do HTLC.",
      "n": 10,
      "expected": "COMMITTED_BOTH",
      "outcome": "COMMITTED_BOTH",
      "outcomes": {
        "COMMITTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 6.1,
      "latency_ms": 8150,
      "latency_sd_ms": 20,
      "writes": 4
    },
    {
      "code": "B2",
      "protocol": "2pc",
      "title": "2PC sem falha, fases em paralelo",
      "story": "O coordenador fala com as duas redes ao mesmo tempo.",
      "n": 10,
      "expected": "COMMITTED_BOTH",
      "outcome": "COMMITTED_BOTH",
      "outcomes": {
        "COMMITTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 2.0,
      "latency_ms": 4070,
      "latency_sd_ms": 13,
      "writes": 4
    },
    {
      "code": "B3",
      "protocol": "2pc",
      "title": "2PC sem falha, fases em sequência",
      "story": "Controle: o mesmo 2PC, mas uma rede por vez. Isola o efeito do paralelismo.",
      "n": 10,
      "expected": "COMMITTED_BOTH",
      "outcome": "COMMITTED_BOTH",
      "outcomes": {
        "COMMITTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 4.1,
      "latency_ms": 8134,
      "latency_sd_ms": 11,
      "writes": 4
    },
    {
      "code": "H1",
      "protocol": "htlc",
      "title": "HTLC: Bob nunca tranca",
      "story": "Alice tranca o título, Bob não aparece. O prazo de Alice vence e ela recupera o ativo.",
      "n": 10,
      "expected": "ABORTED_BOTH",
      "outcome": "ABORTED_BOTH",
      "outcomes": {
        "ABORTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 30.1,
      "latency_ms": 32079,
      "latency_sd_ms": 7,
      "writes": 2
    },
    {
      "code": "H2",
      "protocol": "htlc",
      "title": "HTLC: segredo revelado, Bob não resgata",
      "story": "Alice já pegou os tokens e revelou o segredo; Bob cai antes de usá-lo. O prazo vence e Alice recupera o título também.",
      "n": 10,
      "expected": "VIOLATED",
      "outcome": "VIOLATED",
      "outcomes": {
        "VIOLATED": 10
      },
      "matched": 10,
      "lock_s": 34.1,
      "latency_ms": 36178,
      "latency_sd_ms": 16,
      "writes": 4
    },
    {
      "code": "H3",
      "protocol": "htlc",
      "title": "HTLC: atraso de rede consome a margem",
      "story": "800 ms de atraso em cada mensagem. A folga entre os dois prazos fica pequena e o cliente de Bob se recusa a trancar.",
      "n": 10,
      "expected": "ABORTED_BOTH",
      "outcome": "ABORTED_BOTH",
      "outcomes": {
        "ABORTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 35.1,
      "latency_ms": 44937,
      "latency_sd_ms": 306,
      "writes": 2
    },
    {
      "code": "X1",
      "protocol": "htlc",
      "title": "HTLC: relógio de Bob adiantado",
      "story": "Bob acha que o prazo está mais perto do que está e recusa a troca. Cauteloso demais.",
      "n": 10,
      "expected": "ABORTED_BOTH",
      "outcome": "ABORTED_BOTH",
      "outcomes": {
        "ABORTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 65.1,
      "latency_ms": 67090,
      "latency_sd_ms": 12,
      "writes": 2
    },
    {
      "code": "X2",
      "protocol": "htlc",
      "title": "HTLC: relógio de Bob atrasado",
      "story": "Bob acha que tem mais tempo do que tem — e mesmo assim a troca completa.",
      "n": 10,
      "expected": "COMMITTED_BOTH",
      "outcome": "COMMITTED_BOTH",
      "outcomes": {
        "COMMITTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 6.1,
      "latency_ms": 8156,
      "latency_sd_ms": 15,
      "writes": 4
    },
    {
      "code": "T1",
      "protocol": "2pc",
      "title": "2PC: um participante vota NÃO",
      "story": "Uma das redes recusa na fase de preparo. O coordenador aborta nas duas.",
      "n": 10,
      "expected": "ABORTED_BOTH",
      "outcome": "ABORTED_BOTH",
      "outcomes": {
        "ABORTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 2.0,
      "latency_ms": 4056,
      "latency_sd_ms": 15,
      "writes": 2
    },
    {
      "code": "T2",
      "protocol": "2pc",
      "title": "2PC: coordenador cai entre as fases",
      "story": "Votos coletados, decisão não comunicada. As duas redes ficam com o ativo em custódia, esperando.",
      "n": 10,
      "expected": "BLOCKED",
      "outcome": "BLOCKED",
      "outcomes": {
        "BLOCKED": 10
      },
      "matched": 10,
      "lock_s": null,
      "latency_ms": 2042,
      "latency_sd_ms": 4,
      "writes": 2
    },
    {
      "code": "T2r",
      "protocol": "2pc",
      "title": "2PC: o mesmo, recuperado pelo diário",
      "story": "O coordenador reinicia, lê a decisão que gravou antes de cair e termina o que começou.",
      "n": 10,
      "expected": "COMMITTED_BOTH",
      "outcome": "COMMITTED_BOTH",
      "outcomes": {
        "COMMITTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 24.6,
      "latency_ms": 2025,
      "latency_sd_ms": 5,
      "writes": 2
    },
    {
      "code": "T3",
      "protocol": "2pc",
      "title": "2PC: atraso de rede nas duas fases",
      "story": "Os mesmos 800 ms do H3. O 2PC só fica mais lento.",
      "n": 10,
      "expected": "COMMITTED_BOTH",
      "outcome": "COMMITTED_BOTH",
      "outcomes": {
        "COMMITTED_BOTH": 10
      },
      "matched": 10,
      "lock_s": 6.9,
      "latency_ms": 19354,
      "latency_sd_ms": 14,
      "writes": 4
    },
    {
      "code": "T4",
      "protocol": "2pc",
      "title": "2PC: prazo dispara em um só participante",
      "story": "O coordenador cai; uma rede espera, a outra desiste sozinha pelo prazo. Ao voltar, o coordenador efetiva onde ainda pode.",
      "n": 10,
      "expected": "VIOLATED",
      "outcome": "VIOLATED",
      "outcomes": {
        "VIOLATED": 10
      },
      "matched": 10,
      "lock_s": 30.8,
      "latency_ms": 2026,
      "latency_sd_ms": 6,
      "writes": 1
    }
  ]
};
