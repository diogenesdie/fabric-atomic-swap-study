/* Capítulos 9 e 10: matriz de cenários, gráficos e tabela, a partir de
   window.RESULTS (gerado de experiments/results/aggregated.csv). */
(function () {
  "use strict";
  const R = window.RESULTS;
  if (!R || !R.scenarios) return;
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  const OUTCOME = {
    COMMITTED_BOTH: { cls: "ok",      plain: "os dois efetivaram" },
    ABORTED_BOTH:   { cls: "neutral", plain: "nenhum efetivou" },
    BLOCKED:        { cls: "blocked", plain: "preso, sem decisão" },
    VIOLATED:       { cls: "bad",     plain: "só um lado" },
  };
  const PROTO = { htlc: { tag: "tag-htlc", name: "HTLC", bar: "htlc" }, "2pc": { tag: "tag-2pc", name: "2PC", bar: "twopc" } };

  // Por que cada cenário termina como termina, em uma ou duas frases.
  const WHY = {
    B1: "Quatro escritas, uma após a outra: trancar, trancar, abrir, abrir. A latência é quatro rodadas de bloco.",
    B2: "As mesmas quatro escritas, mas PREPARAR vai às duas redes ao mesmo tempo, e EFETIVAR também. Duas rodadas de bloco.",
    B3: "Forçamos o coordenador a esperar uma rede antes de falar com a outra. Volta a custar quatro rodadas, igual ao HTLC. Prova que a vantagem do 2PC é o paralelismo.",
    H1: "Alice tranca, Bob nunca aparece. Nada vaza, nada se perde, mas o título fica preso até T1 vencer.",
    H2: "Alice já levou os tokens e revelou o segredo. Bob cai antes de usá-lo; T1 vence; Alice recupera o título também. Só um lado da troca aconteceu.",
    H3: "Com 800 ms a mais em cada mensagem, a folga entre T2 e T1 fica menor que o seguro. O cliente de Bob se recusa a trancar; T1 vence; Alice recupera. Aborto correto, longo bloqueio.",
    X1: "Bob, com o relógio 20 s adiantado, acha que T2 está mais perto do que está e recusa uma troca que seria segura. Custou vivacidade (65 s de bloqueio), não segurança.",
    X2: "Bob, com o relógio 40 s atrasado, acha que tem mais tempo do que tem. Segue com uma folga menor do que imagina, e a troca completa mesmo assim.",
    T1: "A Rede B responde NÃO na fase de preparo. O coordenador aborta nas duas. Duas escritas, nada em custódia.",
    T2: "Votos coletados, coordenador morto antes de decidir. As duas redes ficam em custódia sem saber se devem efetivar ou desistir. Ao fim da execução, tudo continua preso.",
    T2r: "Mesma queda, mas a decisão já estava gravada no diário. O coordenador reinicia, lê EFETIVAR e termina. O bloqueio dura o tempo até ele voltar.",
    T3: "Os mesmos 800 ms do H3, nas duas fases. O 2PC não tem folga a perder: fica quase 5× mais lento e efetiva.",
    T4: "Coordenador cai depois de gravar EFETIVAR. A Rede B desiste sozinha pelo prazo; a Rede A espera. O coordenador volta, efetiva a Rede A; a Rede B recusa. Só um lado.",
  };

  const byCode = Object.fromEntries(R.scenarios.map((s) => [s.code, s]));
  const fmtS = (ms) => (ms / 1000).toFixed(1).replace(".", ",");
  const fmt1 = (v) => v.toFixed(1).replace(".", ",");

  /* ---------------------------------------------------- cap. 9: matriz */
  const mbody = document.getElementById("matrix-body");
  if (mbody) {
    R.scenarios.forEach((s) => {
      const o = OUTCOME[s.outcome];
      const e = OUTCOME[s.expected];
      const pr = PROTO[s.protocol];
      const obs = Object.entries(s.outcomes).map(([k, n]) => `${n}× ${k}`).join(", ");
      const tr = document.createElement("tr");
      tr.setAttribute("tabindex", "0");
      tr.setAttribute("aria-expanded", "false");
      tr.innerHTML = `
        <td><span class="caret">▸</span></td>
        <td class="code">${s.code}</td>
        <td><span class="tag ${pr.tag}">${pr.name}</span></td>
        <td>${s.title.replace(/^(HTLC|2PC):?\s*/, "")}</td>
        <td><span class="outcome ${e.cls}">${s.expected}</span></td>
        <td><span class="outcome ${o.cls}">${s.outcome}</span><small style="display:block;margin-top:.3rem;color:var(--muted)">${s.matched}/${s.n} conforme o esperado</small></td>`;
      const detail = document.createElement("tr");
      detail.className = "detail";
      detail.hidden = true;
      detail.innerHTML = `<td colspan="6"><strong>${s.story}</strong><br>${WHY[s.code] || ""}<br>
        <small style="color:var(--muted)">Mediana de ${s.n} execuções: latência ${fmtS(s.latency_ms)} s · ativo preso ${s.lock_s === null ? "sem prazo" : fmt1(s.lock_s) + " s"} · ${s.writes} escrita${s.writes > 1 ? "s" : ""} · ${obs}</small></td>`;
      const toggle = () => { const open = detail.hidden; detail.hidden = !open; tr.classList.toggle("open", open); tr.setAttribute("aria-expanded", String(open)); };
      tr.addEventListener("click", toggle);
      tr.addEventListener("keydown", (ev) => { if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); toggle(); } });
      mbody.append(tr, detail);
    });
  }

  /* ------------------------------------------------------ barras */
  function bars(container, axisEl, rows, max, ticks, unit, fmt = fmt1) {
    container.innerHTML = "";
    rows.forEach((r) => {
      const row = document.createElement("div");
      row.className = "bar-row" + (r.highlight ? " highlight" : "");
      const pct = r.inf ? 100 : Math.min(100, (r.value / max) * 100);
      row.innerHTML = `
        <div class="lbl"><span>${r.label}</span>${r.sub ? `<small>${r.sub}</small>` : ""}</div>
        <div class="track"><div class="bar ${r.cls}${r.inf ? " inf" : ""}${r.dim ? " dim" : ""}" data-w="${pct}" title="${r.title || ""}"></div></div>
        <div class="val">${r.inf ? "sem fim" : fmt(r.value)}<small>${r.inf ? "" : unit}</small></div>`;
      container.appendChild(row);
    });
    if (axisEl) axisEl.innerHTML = ticks.map((t) => `<span>${t}${unit}</span>`).join("");
    const reveal = () => container.querySelectorAll(".bar").forEach((b) => { b.style.width = b.dataset.w + "%"; });
    if (reducedMotion || !("IntersectionObserver" in window)) { reveal(); return; }
    const io = new IntersectionObserver((en) => { if (en.some((e) => e.isIntersecting)) { reveal(); io.disconnect(); } }, { threshold: 0.2 });
    io.observe(container);
  }

  /* ----------------------------------------- cap. 10: latência sem falha */
  const latEl = document.getElementById("bars-latency");
  if (latEl) {
    const rows = [
      ["B1", "HTLC", "4 escritas em sequência", "htlc"],
      ["B2", "2PC · paralelo", "2 rodadas de bloco", "twopc"],
      ["B3", "2PC · sequencial", "controle: 4 rodadas", "twopc"],
    ].map(([code, label, sub, cls]) => ({ label: `${code} · ${label}`, sub, cls, value: byCode[code].latency_ms / 1000, highlight: code !== "B3" }));
    // Duas casas: 8150 ms é 8,15 s, e arredondar para uma casa escondia a
    // diferença entre B1 e B3, que é justamente o que o controle mostra.
    bars(latEl, document.getElementById("axis-latency"), rows, 10, [0, 2, 4, 6, 8, 10], " s",
      (v) => v.toFixed(2).replace(".", ","));
  }

  /* ----------------------------------------------- cap. 10: desfechos */
  const tiles = document.getElementById("tiles-outcomes");
  if (tiles) {
    R.scenarios.forEach((s) => {
      const o = OUTCOME[s.outcome];
      const pr = PROTO[s.protocol];
      const d = document.createElement("div");
      d.className = "tile " + o.cls;
      d.innerHTML = `<div class="code"><span>${s.code}</span><span class="tag ${pr.tag}">${pr.name}</span></div>
        <div class="what">${s.title.replace(/^(HTLC|2PC):?\s*/, "")}</div>
        <div class="res">${o.plain}</div>`;
      tiles.appendChild(d);
    });
  }

  /* ---------------------------------------------- cap. 10: bloqueio */
  const blkEl = document.getElementById("bars-blocking");
  if (blkEl) {
    const rows = R.scenarios.map((s) => ({
      label: s.code,
      sub: s.title.replace(/^(HTLC|2PC):?\s*/, ""),
      cls: PROTO[s.protocol].bar,
      value: s.lock_s === null ? 0 : s.lock_s,
      inf: s.lock_s === null,
      highlight: ["H2", "T4", "T2", "X1"].includes(s.code),
      title: `${s.code}: ${s.lock_s === null ? "sem prazo" : fmt1(s.lock_s) + " s"}`,
    }));
    bars(blkEl, document.getElementById("axis-blocking"), rows, 70, [0, 10, 20, 30, 40, 50, 60, 70], " s");
  }

  /* --------------------------------------------- cap. 10: tabela */
  const tbody = document.querySelector("#results-table tbody");
  if (tbody) {
    R.scenarios.forEach((s) => {
      const o = OUTCOME[s.outcome];
      const tr = document.createElement("tr");
      tr.innerHTML = `<td class="code mono"><strong>${s.code}</strong></td><td>${PROTO[s.protocol].name}</td>
        <td><span class="outcome ${o.cls}">${s.outcome}</span></td>
        <td class="num">${s.latency_ms.toLocaleString("pt-BR")}</td>
        <td class="num">${s.lock_s === null ? "sem prazo" : fmt1(s.lock_s)}</td>
        <td class="num">${s.writes}</td><td class="num">${s.n}</td>`;
      tbody.appendChild(tr);
    });
  }
  const runs = document.getElementById("stat-runs");
  if (runs) runs.textContent = String(R.runs);
})();
