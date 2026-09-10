/* Simuladores e demonstrações interativas.
   Cada figura é um "palco" SVG desenhado aqui e um roteiro de passos.
   Os textos ficam nos roteiros: corrigir uma explicação é editar uma linha. */
(function () {
  "use strict";

  const { tween, wait, ease, cancelToken, reducedMotion } = window.Anim;
  const NS = "http://www.w3.org/2000/svg";

  /* ----------------------------------------------------- utilidades */
  function el(name, attrs = {}, parent = null) {
    const node = document.createElementNS(NS, name);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === "text") node.textContent = v;
      else node.setAttribute(k, v);
    }
    if (parent) parent.appendChild(node);
    return node;
  }
  function svgRoot(container, w, h, label) {
    container.innerHTML = "";
    return el("svg", { viewBox: `0 0 ${w} ${h}`, role: "img", "aria-label": label }, container);
  }
  function lane(svg, x, y, w, h, title) {
    el("rect", { class: "lane", x, y, width: w, height: h, rx: 6 }, svg);
    el("text", { class: "lane-title", x: x + 14, y: y + 20, text: title }, svg);
  }
  function actor(svg, cx, cy, name, sub) {
    const g = el("g", {}, svg);
    const c = el("circle", { class: "actor", cx, cy, r: 26 }, g);
    el("text", { class: "actor-name", x: cx, y: cy + 5, "text-anchor": "middle", text: name[0] }, g);
    el("text", { class: "actor-name", x: cx, y: cy + 46, "text-anchor": "middle", text: name }, g);
    if (sub) el("text", { class: "actor-sub", x: cx, y: cy + 62, "text-anchor": "middle", text: sub }, g);
    const x = el("g", { class: "dead-x hidden" }, g);
    el("line", { x1: cx - 14, y1: cy - 14, x2: cx + 14, y2: cy + 14 }, x);
    el("line", { x1: cx + 14, y1: cy - 14, x2: cx - 14, y2: cy + 14 }, x);
    return {
      g, circle: c, cx, cy,
      kill() { c.classList.add("dead"); x.classList.remove("hidden"); },
      fade() { g.classList.add("fade"); },
      reset() { c.classList.remove("dead"); x.classList.add("hidden"); g.classList.remove("fade"); },
    };
  }
  /** Caixa de ativo com nome e duas linhas de estado. */
  function asset(svg, x, y, w, h, name, state) {
    const g = el("g", {}, svg);
    const r = el("rect", { class: "asset", x, y, width: w, height: h, rx: 5 }, g);
    el("text", { class: "asset-name", x: x + 12, y: y + 22, text: name }, g);
    const s1 = el("text", { class: "asset-state", x: x + 12, y: y + 42, text: state }, g);
    const s2 = el("text", { class: "asset-state", x: x + 12, y: y + 58, text: "" }, g);
    return {
      g, rect: r, cx: x + w / 2, cy: y + h / 2, top: { x: x + w / 2, y },
      set(cls, line1, line2 = "") {
        r.setAttribute("class", "asset" + (cls ? " " + cls : ""));
        if (line1 !== undefined) s1.textContent = line1;
        s2.textContent = line2;
      },
    };
  }
  /** Mensagem que viaja de p0 a p1. Remove-se ao chegar. */
  async function message(svg, p0, p1, label, cls = "", ms = 900, signal) {
    const g = el("g", { class: "msg-g" }, svg);
    const w = label.length * 6.6 + 18;
    const rect = el("rect", { class: "msg " + cls, x: -w / 2, y: -11, width: w, height: 22, rx: 11 }, g);
    el("text", { class: "msg-text", x: 0, y: 4, "text-anchor": "middle", text: label }, g);
    if (cls) g.classList.add(cls);
    const place = (t) => g.setAttribute("transform", `translate(${p0.x + (p1.x - p0.x) * t} ${p0.y + (p1.y - p0.y) * t})`);
    place(0);
    await tween(ms, place, ease.inOut, signal);
    await wait(180, signal);
    g.remove();
    return rect;
  }

  /* --------------------------------------------------------- Player */
  /** Motor de passos: reproduz, avança um a um e reinicia um roteiro. */
  class Player {
    constructor(opts) {
      this.o = opts;               // { say, outcome, foot, play, step, reset, tabs, build, scenarios }
      this.signal = cancelToken();
      this.scenarioKey = null;
      this.ctx = null;
      this.steps = [];
      this.i = 0;
      this.playing = false;

      if (opts.play) opts.play.addEventListener("click", () => this.play());
      if (opts.step) opts.step.addEventListener("click", () => this.step());
      if (opts.reset) opts.reset.addEventListener("click", () => this.load(this.scenarioKey));
      if (opts.tabs) {
        opts.tabs.querySelectorAll("[data-scenario]").forEach((tab) => {
          tab.addEventListener("click", () => {
            opts.tabs.querySelectorAll("[data-scenario]").forEach((t) => t.setAttribute("aria-selected", String(t === tab)));
            this.load(tab.dataset.scenario);
          });
        });
      }
    }
    load(key) {
      this.signal.cancel();
      this.signal = cancelToken();
      this.playing = false;
      this.scenarioKey = key;
      this.ctx = this.o.build();
      const sc = this.o.scenarios[key];
      this.steps = sc.steps;
      this.i = 0;
      this.say(0, sc.intro || "Pressione ▶ Reproduzir.", true);
      this.setOutcome(null);
      if (this.o.foot) this.o.foot.innerHTML = sc.foot || "";
      this.buttons();
    }
    say(n, text, idle = false) {
      const s = this.o.say;
      if (!s) return;
      s.classList.toggle("idle", idle);
      const num = s.querySelector(".step-n");
      const body = s.querySelector(".step-n + span") || s;
      if (num) num.textContent = n ? `${n}/${this.steps.length}` : "";
      if (body === s) s.textContent = text; else body.innerHTML = text;
    }
    setOutcome(o) {
      const b = this.o.outcome;
      if (!b) return;
      if (!o) { b.className = "outcome pending"; b.textContent = "—"; return; }
      b.className = "outcome " + o.cls;
      b.innerHTML = `${o.code} <span class="plain">· ${o.plain}</span>`;
    }
    buttons() {
      const done = this.i >= this.steps.length;
      if (this.o.step) this.o.step.disabled = this.playing || done;
      if (this.o.play) this.o.play.disabled = this.playing || done;
      if (this.o.play) this.o.play.textContent = done ? "✓ Fim" : "▶ Reproduzir";
    }
    async runStep(signal) {
      const st = this.steps[this.i];
      this.i += 1;
      this.say(this.i, st.say);
      await st.run(this.ctx, signal);
      if (signal.cancelled) return;
      if (this.i >= this.steps.length) {
        const sc = this.o.scenarios[this.scenarioKey];
        this.setOutcome(sc.outcome);
      }
    }
    async step() {
      if (this.playing || this.i >= this.steps.length) return;
      this.playing = true; this.buttons();
      const signal = this.signal;
      await this.runStep(signal);
      if (signal.cancelled) return;
      this.playing = false; this.buttons();
    }
    async play() {
      if (this.playing || this.i >= this.steps.length) return;
      this.playing = true; this.buttons();
      const signal = this.signal;
      while (this.i < this.steps.length && !signal.cancelled) {
        await this.runStep(signal);
        if (signal.cancelled) return;
        await wait(reducedMotion ? 0 : 1100, signal);
      }
      if (signal.cancelled) return;
      this.playing = false; this.buttons();
    }
  }

  const OUT = {
    ok:      { cls: "ok",      code: "COMMITTED_BOTH", plain: "os dois lados efetivaram" },
    neutral: { cls: "neutral", code: "ABORTED_BOTH",   plain: "nenhum efetivou, nada perdido" },
    blocked: { cls: "blocked", code: "BLOCKED",        plain: "ativos presos, sem decisão" },
    bad:     { cls: "bad",     code: "VIOLATED",       plain: "só um lado efetivou" },
  };

  /* ================================================ cap. 1: blockchain */
  (function chainDemo() {
    const root = document.getElementById("chain");
    if (!root) return;
    const say = document.getElementById("chain-say");
    const initial = [
      "Alice → Bob: 10 tokens",
      "Bob → Carol: 3 tokens",
      "Carol → Alice: 1 título",
      "Alice → Dan: 5 tokens",
    ];
    const extra = ["Dan → Bob: 2 tokens", "Bob → Alice: 1 título", "Carol → Dan: 4 tokens", "Dan → Carol: 7 tokens"];
    let txs = [];
    // "recorded" é o hash que cada bloco anotou do anterior, na hora em que foi criado.
    let blocks = [], recorded = [], fresh = false;

    function hash(str) {
      let h = 0x811c9dc5;
      for (let i = 0; i < str.length; i++) { h ^= str.charCodeAt(i); h = Math.imul(h, 0x01000193) >>> 0; }
      return h.toString(16).padStart(8, "0");
    }
    function render(editedIdx = -1) {
      root.innerHTML = "";
      let prev = "00000000";
      let broken = false;
      txs.forEach((tx, i) => {
        const h = hash(prev + "|" + tx);
        const b = document.createElement("div");
        b.className = "block";
        blocks.push({ h });
        if (i === editedIdx) b.classList.add("edited");
        if (i > 0 && blocks[i - 1].h !== recorded[i - 1]) broken = true;
        if (broken && i !== editedIdx) b.classList.add("broken");
        b.innerHTML = `
          <div class="bn"><span>BLOCO ${i + 1}</span><span>${i === txs.length - 1 && fresh ? "novo" : ""}</span></div>
          <div class="tx" contenteditable="true" spellcheck="false" data-i="${i}">${tx}</div>
          <div class="field prev"><span>anterior</span><span>${recorded[i - 1] !== undefined ? recorded[i - 1] : "00000000"}</span></div>
          <div class="field"><span>hash</span><span>${h}</span></div>`;
        if (i === txs.length - 1 && fresh) b.classList.add("fresh");
        root.appendChild(b);
        prev = h;
      });
      root.querySelectorAll(".tx").forEach((t) => {
        t.addEventListener("input", () => {
          const i = Number(t.dataset.i);
          txs[i] = t.textContent;
          fresh = false;
          rerender(i, t);
        });
      });
      const anyBroken = root.querySelector(".block.broken");
      if (editedIdx >= 0 && anyBroken) {
        say.innerHTML = `O bloco ${editedIdx + 1} mudou, logo seu hash mudou. Os seguintes guardam o hash <strong>antigo</strong>: a corrente <strong>não bate</strong>, e qualquer nó percebe.`;
      } else if (editedIdx >= 0) {
        say.innerHTML = "Você mudou o <strong>último</strong> bloco: ainda não há bloco seguinte para denunciar. Por isso blockchains esperam alguns blocos de \"confirmação\".";
      } else if (fresh) {
        say.innerHTML = "Um novo bloco entrou no fim, carregando o hash do anterior. O passado continua intacto.";
      } else {
        say.innerHTML = "Cada bloco guarda o hash do anterior. Enquanto tudo bate, a corrente está íntegra.";
      }
      root.scrollLeft = root.scrollWidth;
    }
    function rebuildRecorded() {
      recorded = [];
      let prev = "00000000";
      txs.forEach((tx) => { const h = hash(prev + "|" + tx); recorded.push(h); prev = h; });
    }
    function rerender(editedIdx, focusEl) {
      blocks = [];
      const sel = focusEl ? { i: editedIdx, pos: getCaret(focusEl) } : null;
      render(editedIdx);
      if (sel) {
        const t = root.querySelector(`.tx[data-i="${sel.i}"]`);
        if (t) { t.focus(); setCaret(t, sel.pos); }
      }
    }
    function getCaret(node) {
      const s = window.getSelection(); if (!s || !s.rangeCount) return 0;
      const r = s.getRangeAt(0); const pre = r.cloneRange(); pre.selectNodeContents(node); pre.setEnd(r.endContainer, r.endOffset);
      return pre.toString().length;
    }
    function setCaret(node, pos) {
      const text = node.firstChild; if (!text) return;
      const r = document.createRange(); r.setStart(text, Math.min(pos, text.length)); r.collapse(true);
      const s = window.getSelection(); s.removeAllRanges(); s.addRange(r);
    }
    function reset() { txs = initial.slice(); fresh = false; rebuildRecorded(); blocks = []; render(); }
    reset();

    document.getElementById("chain-add").addEventListener("click", () => {
      if (txs.length >= 8) return;
      txs.push(extra[(txs.length - initial.length) % extra.length]);
      fresh = true; rebuildRecorded(); blocks = []; render();
    });
    document.getElementById("chain-tamper").addEventListener("click", () => {
      txs[1] = "Bob → Carol: 300 tokens";
      fresh = false; blocks = []; render(1);
    });
    document.getElementById("chain-reset").addEventListener("click", reset);
  })();

  /* ================================================= cap. 2: contrato */
  (function contractDemo() {
    const range = document.getElementById("balance");
    if (!range) return;
    const out = document.getElementById("balance-out");
    const nodes = Array.from(document.querySelectorAll("#contract-demo .node"));
    const say = document.getElementById("contract-say");
    range.addEventListener("input", () => { out.textContent = range.value; nodes.forEach((n) => { n.className = "node"; n.querySelector(".res").textContent = "aguardando"; }); });
    document.getElementById("contract-run").addEventListener("click", async () => {
      const bal = Number(range.value);
      const ok = bal >= 10;
      nodes.forEach((n) => { n.className = "node think"; n.querySelector(".res").textContent = "executando…"; });
      for (const n of nodes) {
        await wait(reducedMotion ? 0 : 450);
        n.className = "node " + (ok ? "ok" : "no");
        n.querySelector(".res").textContent = ok ? `✔ aprovado · Alice fica com ${bal - 10}` : `✖ recusado · saldo ${bal} < 10`;
      }
      say.innerHTML = ok
        ? "Mesmo resultado nos três nós: a transferência vira uma linha do livro."
        : "Recusa unânime, pelo mesmo motivo. Nada muda no livro. Ninguém negociou: a regra decidiu.";
    });
  })();

  /* ============================================ cap. 3: quem entrega? */
  (function firstMover() {
    const stage = document.getElementById("stage-fm");
    if (!stage) return;
    function build() {
      const svg = svgRoot(stage, 900, 250, "Alice e Bob, cada um com seu ativo numa rede diferente.");
      const alice = actor(svg, 80, 120, "Alice", "tem o título");
      const bob = actor(svg, 820, 120, "Bob", "tem os tokens");
      lane(svg, 160, 40, 270, 170, "REDE A · livro");
      lane(svg, 470, 40, 270, 170, "REDE B · livro");
      const a = asset(svg, 185, 95, 220, 70, "Título", "dono: Alice");
      const b = asset(svg, 495, 95, 220, 70, "Tokens", "dono: Bob");
      return { svg, alice, bob, a, b };
    }
    const scenarios = {
      alice: {
        outcome: { ...OUT.bad, plain: "Bob ficou com o título e com os tokens" },
        steps: [
          { say: "Alice vai primeiro: transfere o título para Bob na Rede A.",
            run: async (c, s) => { await message(c.svg, { x: 110, y: 120 }, c.a.top, "título → Bob", "", 900, s); c.a.set("moved", "dono: Bob", "transferido"); } },
          { say: "Bob some. Nunca envia os tokens. A Rede B nem sabe da troca.",
            run: async (c, s) => { await wait(400, s); c.bob.kill(); } },
          { say: "Só um lado aconteceu. Alice perdeu.",
            run: async () => {} },
        ],
      },
      bob: {
        outcome: { ...OUT.bad, plain: "Alice ficou com os tokens e com o título" },
        steps: [
          { say: "Bob vai primeiro: transfere os tokens para Alice na Rede B.",
            run: async (c, s) => { await message(c.svg, { x: 790, y: 120 }, c.b.top, "tokens → Alice", "", 900, s); c.b.set("moved", "dono: Alice", "transferido"); } },
          { say: "Alice some. Nunca transfere o título. A Rede A nem sabe da troca.",
            run: async (c, s) => { await wait(400, s); c.alice.kill(); } },
          { say: "Só um lado aconteceu. Quem se moveu primeiro perdeu.",
            run: async () => {} },
        ],
      },
    };
    const player = new Player({
      build, scenarios,
      say: document.getElementById("fm-say"),
      outcome: document.getElementById("fm-outcome"),
      reset: document.getElementById("fm-reset"),
    });
    player.load("alice");
    player.say(0, "Dois livros, dois donos. Quem age primeiro confia no outro.", true);
    document.getElementById("fm-alice").addEventListener("click", () => { player.load("alice"); player.play(); });
    document.getElementById("fm-bob").addEventListener("click", () => { player.load("bob"); player.play(); });
  })();

  /* ==================================================== cap. 4: votos */
  (function voteDemo() {
    const voters = Array.from(document.querySelectorAll("#voters .voter"));
    if (!voters.length) return;
    const vc = document.getElementById("v-consensus");
    const vk = document.getElementById("v-commit");
    function update() {
      const yes = voters.filter((v) => v.dataset.vote === "yes").length;
      const no = voters.length - yes;
      const maj = yes > no;
      vc.className = "verdict " + (maj ? "ok" : "no");
      vc.querySelector(".res").textContent = maj ? "Decidido: SIM" : (yes === no ? "Empate: sem decisão" : "Decidido: NÃO");
      vc.querySelector(".why").textContent = `${yes} de ${voters.length} disseram sim. Basta a maioria.`;
      const all = no === 0;
      vk.className = "verdict " + (all ? "ok" : "no");
      vk.querySelector(".res").textContent = all ? "EFETIVAR" : "ABORTAR tudo";
      vk.querySelector(".why").textContent = all ? "Unanimidade: todos efetivam." : `${no} recusa${no > 1 ? "s" : ""}. Uma basta para abortar a troca inteira.`;
    }
    voters.forEach((v) => v.addEventListener("click", () => {
      v.dataset.vote = v.dataset.vote === "yes" ? "no" : "yes";
      v.querySelector(".vote").textContent = v.dataset.vote === "yes" ? "SIM" : "NÃO";
      update();
    }));
    update();
  })();

  /* ====================================================== cap. 5: 2PC */
  (function sim2pc() {
    const stage = document.getElementById("stage-2pc");
    if (!stage) return;

    function build() {
      const svg = svgRoot(stage, 900, 400, "Coordenador no alto, duas redes embaixo; mensagens vão e voltam.");
      const cg = el("g", {}, svg);
      const cbox = el("rect", { class: "actor", x: 370, y: 28, width: 160, height: 58, rx: 8 }, cg);
      el("text", { class: "actor-name", x: 450, y: 52, "text-anchor": "middle", text: "Coordenador" }, cg);
      el("text", { class: "actor-sub", x: 450, y: 70, "text-anchor": "middle", text: "programa externo às redes" }, cg);
      const cx = el("g", { class: "dead-x hidden" }, cg);
      el("line", { x1: 430, y1: 40, x2: 470, y2: 74 }, cx);
      el("line", { x1: 470, y1: 40, x2: 430, y2: 74 }, cx);
      el("rect", { class: "log", x: 585, y: 22, width: 200, height: 72, rx: 5 }, svg);
      el("text", { class: "log-title", x: 597, y: 40, text: "DIÁRIO DO COORDENADOR" }, svg);
      const logLines = [el("text", { class: "log-line", x: 597, y: 60, text: "" }, svg), el("text", { class: "log-line", x: 597, y: 78, text: "" }, svg)];
      lane(svg, 40, 170, 370, 200, "REDE A · título de Alice");
      lane(svg, 490, 170, 370, 200, "REDE B · tokens de Bob");
      const a = asset(svg, 70, 220, 310, 74, "Título", "dono: Alice");
      const b = asset(svg, 520, 220, 310, 74, "Tokens", "dono: Bob");
      const clockA = el("text", { class: "clock hidden", x: 225, y: 340, "text-anchor": "middle", text: "" }, svg);
      const clockB = el("text", { class: "clock hidden", x: 675, y: 340, "text-anchor": "middle", text: "" }, svg);
      el("path", { class: "wire", d: "M 420 86 L 225 170" }, svg);
      el("path", { class: "wire", d: "M 480 86 L 675 170" }, svg);
      const A = { x: 225, y: 170 }, B = { x: 675, y: 170 };

      return {
        svg, a, b,
        toA: (label, cls, s, ms) => message(svg, { x: 420, y: 86 }, A, label, cls, ms, s),
        toB: (label, cls, s, ms) => message(svg, { x: 480, y: 86 }, B, label, cls, ms, s),
        fromA: (label, cls, s, ms) => message(svg, A, { x: 420, y: 86 }, label, cls, ms, s),
        fromB: (label, cls, s, ms) => message(svg, B, { x: 480, y: 86 }, label, cls, ms, s),
        log(line) { const t = logLines.find((l) => !l.textContent) || logLines[1]; t.textContent = line; },
        crash() { cbox.classList.add("dead"); cx.classList.remove("hidden"); },
        revive() { cbox.classList.remove("dead"); cx.classList.add("hidden"); },
        clock(which, text) { const c = which === "A" ? clockA : clockB; c.textContent = text; c.classList.toggle("hidden", !text); },
      };
    }

    const prepare = {
      say: "Fase 1. O coordenador pergunta às duas redes: <em>consegue?</em>",
      run: async (c, s) => { await Promise.all([c.toA("PREPARAR?", "", s), c.toB("PREPARAR?", "", s)]); },
    };
    const voteYes = {
      say: "Cada rede põe o ativo em <strong>custódia</strong>, anota no livro e responde SIM. Agora não pode mais desistir sozinha.",
      run: async (c, s) => {
        c.a.set("escrow", "em custódia", "prometeu: SIM"); c.b.set("escrow", "em custódia", "prometeu: SIM");
        await wait(300, s);
        await Promise.all([c.fromA("SIM", "yes", s), c.fromB("SIM", "yes", s)]);
      },
    };
    const decideCommit = {
      say: "Dois SIM. Antes de avisar, o coordenador <strong>grava a decisão no diário</strong>: EFETIVAR.",
      run: async (c, s) => { c.log("tx-42: votos SIM, SIM"); await wait(350, s); c.log("tx-42: decisão = EFETIVAR ✔"); },
    };
    const sendCommit = {
      say: "Fase 2. EFETIVAR para as duas redes.",
      run: async (c, s) => { await Promise.all([c.toA("EFETIVAR", "yes", s), c.toB("EFETIVAR", "yes", s)]); },
    };
    const commitBoth = {
      say: "Os ativos saem da custódia e trocam de dono. Os dois livros mudaram.",
      run: async (c) => { c.a.set("moved", "dono: Bob", "efetivado"); c.b.set("moved", "dono: Alice", "efetivado"); },
    };
    const waitForever = {
      say: "As redes esperam: não podem efetivar (<em>e se a outra disse NÃO?</em>) nem desistir (<em>e se a decisão foi efetivar?</em>). Ativos presos.",
      run: async (c, s) => { c.clock("A", "⏳ esperando decisão…"); c.clock("B", "⏳ esperando decisão…"); await wait(500, s); },
    };

    const scenarios = {
      ok: {
        outcome: OUT.ok,
        foot: "<strong>Medido (B2):</strong> 4 escritas em 2 rodadas de bloco, em paralelo. Latência mediana <strong>4,07 s</strong>; custódia por 2,0 s.",
        steps: [prepare, voteYes, decideCommit, sendCommit, commitBoth],
      },
      no: {
        outcome: OUT.neutral,
        foot: "<strong>Medido (T1):</strong> 2 escritas, 4,06 s, nada perdido.",
        steps: [
          prepare,
          { say: "Rede A promete SIM e põe o título em custódia. Rede B <strong>recusa</strong>: NÃO.",
            run: async (c, s) => { c.a.set("escrow", "em custódia", "prometeu: SIM"); c.b.set("", "dono: Bob", "recusou"); await wait(300, s); await Promise.all([c.fromA("SIM", "yes", s), c.fromB("NÃO", "no", s)]); } },
          { say: "Um NÃO basta. Decisão gravada: ABORTAR.",
            run: async (c, s) => { c.log("tx-42: votos SIM, NÃO"); await wait(350, s); c.log("tx-42: decisão = ABORTAR"); } },
          { say: "ABORTAR para as duas. O título volta a Alice; a Rede B não tinha nada a desfazer.",
            run: async (c, s) => { await Promise.all([c.toA("ABORTAR", "no", s), c.toB("ABORTAR", "no", s)]); c.a.set("back", "dono: Alice", "devolvido"); c.b.set("back", "dono: Bob", "nada a desfazer"); } },
        ],
      },
      crash: {
        outcome: OUT.blocked,
        foot: "<strong>Medido (T2):</strong> os dois ativos em custódia ao fim de todas as 10 execuções. O bloqueio <strong>não tem prazo</strong>.",
        steps: [
          prepare, voteYes,
          { say: "O coordenador <strong>cai</strong> aqui: tem os votos, não decidiu nem avisou.",
            run: async (c, s) => { await wait(300, s); c.crash(); } },
          waitForever,
        ],
      },
      recover: {
        outcome: OUT.ok,
        foot: "<strong>Medido (T2r):</strong> a decisão estava no diário; a recuperação a honra. Custo: <strong>24,6 s</strong> de ativos presos, o tempo até o coordenador voltar.",
        steps: [
          prepare, voteYes, decideCommit,
          { say: "Grava EFETIVAR… e <strong>cai</strong> antes de enviar.",
            run: async (c, s) => { await wait(300, s); c.crash(); c.clock("A", "⏳ esperando…"); c.clock("B", "⏳ esperando…"); } },
          { say: "Reinicia e lê o diário: EFETIVAR pendente para tx-42.",
            run: async (c, s) => { await wait(700, s); c.revive(); } },
          { say: "Retoma de onde parou: EFETIVAR para as duas.",
            run: async (c, s) => { c.clock("A", ""); c.clock("B", ""); await Promise.all([c.toA("EFETIVAR", "yes", s), c.toB("EFETIVAR", "yes", s)]); } },
          commitBoth,
        ],
      },
      timeout: {
        outcome: { ...OUT.bad, plain: "Bob ficou com o título e com os tokens" },
        foot: "<strong>Medido (T4):</strong> 10 de 10 violaram. É o que a válvula de escape custa: <strong>sem</strong> prazo o 2PC nunca viola, mas trava para sempre (T2); <strong>com</strong> prazo, herda a dependência do tempo do HTLC.",
        steps: [
          prepare, voteYes, decideCommit,
          { say: "Grava EFETIVAR e <strong>cai</strong>. Desta vez cada rede tem um prazo para desistir sozinha.",
            run: async (c, s) => { await wait(300, s); c.crash(); c.clock("A", "⏳ prazo correndo…"); c.clock("B", "⏳ prazo correndo…"); } },
          { say: "O prazo vence na Rede B: ela <strong>desiste</strong> e devolve os tokens a Bob. A Rede A ainda espera.",
            run: async (c, s) => { await wait(600, s); c.clock("B", "prazo venceu → desistiu"); c.b.set("back", "dono: Bob", "desistiu pelo prazo"); } },
          { say: "O coordenador volta, lê EFETIVAR e cumpre: EFETIVAR para as duas.",
            run: async (c, s) => { c.revive(); c.clock("A", ""); await Promise.all([c.toA("EFETIVAR", "yes", s), c.toB("EFETIVAR", "yes", s)]); } },
          { say: "Rede A obedece: título → Bob. Rede B <strong>recusa</strong>: já desistiu. Um lado só.",
            run: async (c) => { c.a.set("moved", "dono: Bob", "efetivado"); c.b.set("back", "dono: Bob", "recusou: já desistiu"); c.clock("B", "✖ recusou EFETIVAR"); } },
        ],
      },
    };

    const p = new Player({
      build, scenarios,
      tabs: document.getElementById("tabs-2pc"),
      say: document.getElementById("s2-say"),
      outcome: document.getElementById("s2-outcome"),
      foot: document.getElementById("s2-foot"),
      play: document.getElementById("s2-play"),
      step: document.getElementById("s2-step"),
      reset: document.getElementById("s2-reset"),
    });
    p.load("ok");
  })();

  /* ===================================================== cap. 6: HTLC */
  (function simHtlc() {
    const stage = document.getElementById("stage-htlc");
    if (!stage) return;
    const T2 = 0.48, T1 = 1.0; // frações da linha do tempo (T1 = 25 s, T2 = 12 s no experimento)
    const TL = { x: 160, y: 352, w: 580, h: 14 };

    function build() {
      const svg = svgRoot(stage, 900, 400, "Alice à esquerda, Bob à direita, as duas redes no meio e uma linha do tempo embaixo.");
      const alice = actor(svg, 80, 150, "Alice", "quer os tokens");
      const bob = actor(svg, 820, 150, "Bob", "quer o título");
      lane(svg, 160, 40, 270, 240, "REDE A · livro");
      lane(svg, 470, 40, 270, 240, "REDE B · livro");
      const a = asset(svg, 185, 110, 220, 74, "Título de Alice", "dono: Alice");
      const b = asset(svg, 495, 110, 220, 74, "Tokens de Bob", "dono: Bob");
      const sg = el("g", { class: "hidden" }, svg);
      el("rect", { class: "secret", x: 22, y: 26, width: 116, height: 40, rx: 5 }, sg);
      el("text", { class: "secret-text", x: 80, y: 43, "text-anchor": "middle", text: "s = 7c1e·92af" }, sg);
      el("text", { class: "asset-state", x: 80, y: 58, "text-anchor": "middle", text: "H(s) = a3f9·0b" }, sg);
      const leak = el("text", { class: "secret-text hidden", x: 605, y: 240, "text-anchor": "middle", text: "livro B agora mostra: s = 7c1e·92af" }, svg);
      const leakA = el("text", { class: "asset-state hidden", x: 295, y: 240, "text-anchor": "middle", text: "Bob lê s no livro B" }, svg);
      el("text", { class: "lane-title", x: TL.x, y: TL.y - 12, text: "TEMPO" }, svg);
      el("rect", { class: "tl-margin", x: TL.x + TL.w * T2, y: TL.y - 3, width: TL.w * (T1 - T2), height: TL.h + 6 }, svg);
      el("rect", { class: "tl-bar", x: TL.x, y: TL.y, width: TL.w, height: TL.h, rx: 3 }, svg);
      const fill = el("rect", { class: "tl-fill", x: TL.x, y: TL.y, width: 0, height: TL.h, rx: 3 }, svg);
      const now = el("line", { class: "tl-now", x1: TL.x, y1: TL.y - 6, x2: TL.x, y2: TL.y + TL.h + 6 }, svg);
      [[T2, "T2 · prazo de Bob"], [T1, "T1 · prazo de Alice"]].forEach(([f, label]) => {
        const x = TL.x + TL.w * f;
        el("line", { class: "tl-mark", x1: x, y1: TL.y - 6, x2: x, y2: TL.y + TL.h + 6 }, svg);
        el("text", { class: "tl-text", x: x, y: TL.y + TL.h + 22, "text-anchor": f === T1 ? "end" : "middle", text: label }, svg);
      });
      el("text", { class: "asset-state", x: TL.x + TL.w * (T2 + (T1 - T2) / 2), y: TL.y - 12, "text-anchor": "middle", text: "folga para Bob agir depois que s vaza" }, svg);

      let t = 0;
      return {
        svg, alice, bob, a, b,
        showSecret() { sg.classList.remove("hidden"); },
        leak() { leak.classList.remove("hidden"); },
        readA() { leakA.classList.remove("hidden"); },
        msg: (from, to, label, cls, s, ms) => message(svg, from, to, label, cls, ms, s),
        async advance(to, s, ms = 900) {
          const from = t;
          await tween(ms, (k) => {
            const f = from + (to - from) * k;
            fill.setAttribute("width", TL.w * f);
            now.setAttribute("x1", TL.x + TL.w * f); now.setAttribute("x2", TL.x + TL.w * f);
          }, ease.linear, s);
          t = to;
        },
        P: { alice: { x: 106, y: 150 }, bob: { x: 794, y: 150 }, aTop: a.top, bTop: b.top },
      };
    }

    const s1 = { say: "Alice sorteia um segredo <code>s</code> e publica só a impressão digital <code>H(s)</code>.",
      run: async (c, s) => { c.showSecret(); await wait(300, s); } };
    const s2 = { say: "Alice tranca o título na Rede A: só Bob abre, só com <code>s</code>, e volta para ela em <strong>T1</strong>.",
      run: async (c, s) => { await c.msg(c.P.alice, c.P.aTop, "TRANCAR · H(s) · até T1", "", s); c.a.set("locked", "🔒 trancado com H(s)", "para Bob · devolve em T1"); await c.advance(0.14, s, 500); } };
    const s3 = { say: "Bob tranca os tokens na Rede B com o <strong>mesmo</strong> <code>H(s)</code>: só Alice abre, prazo <strong>T2</strong>, menor.",
      run: async (c, s) => { await c.msg(c.P.bob, c.P.bTop, "TRANCAR · H(s) · até T2", "", s); c.b.set("locked", "🔒 trancado com H(s)", "para Alice · devolve em T2"); await c.advance(0.28, s, 500); } };
    const s4 = { say: "Alice abre a Rede B com <code>s</code> e leva os tokens. <strong><code>s</code> fica escrito no livro B.</strong>",
      run: async (c, s) => { await c.msg(c.P.alice, c.P.bTop, "ABRIR com s", "gold", s); c.b.set("moved", "dono: Alice", "aberto com s"); c.leak(); await c.advance(0.38, s, 400); } };
    const s5 = { say: "Bob lê <code>s</code> no livro B e abre a Rede A. Troca completa, sem coordenador.",
      run: async (c, s) => { c.readA(); await c.msg(c.P.bob, c.P.aTop, "ABRIR com s", "gold", s); c.a.set("moved", "dono: Bob", "aberto com s"); await c.advance(0.5, s, 400); } };

    const scenarios = {
      ok: {
        outcome: OUT.ok,
        foot: "<strong>Medido (B1):</strong> 4 escritas em sequência (o passo 5 depende do segredo do passo 4). Latência mediana <strong>8,15 s</strong>: o dobro do 2PC paralelo.",
        steps: [s1, s2, s3, s4, s5],
      },
      nolock: {
        outcome: OUT.neutral,
        foot: "<strong>Medido (H1):</strong> nada perdido, mas o título ficou preso <strong>30 s</strong>, o prazo T1 inteiro.",
        steps: [s1, s2,
          { say: "Bob nunca aparece.",
            run: async (c, s) => { c.bob.fade(); await c.advance(0.5, s, 800); } },
          { say: "T2 passa em branco. Depois, <strong>T1 vence</strong>.",
            run: async (c, s) => { await c.advance(T1, s, 1200); } },
          { say: "Alice pede o título de volta. Nada perdido, mas ficou preso o prazo inteiro.",
            run: async (c, s) => { await c.msg(c.P.alice, c.P.aTop, "DEVOLVER · T1 venceu", "no", s); c.a.set("back", "dono: Alice", "devolvido pelo prazo"); } },
        ],
      },
      leak: {
        outcome: { ...OUT.bad, plain: "Alice ficou com os tokens e com o título" },
        foot: "<strong>Medido (H2):</strong> 10 de 10 violaram. É a crítica de Zakhary (2020) ao HTLC, reproduzida.",
        steps: [s1, s2, s3, s4,
          { say: "Bob <strong>cai</strong> antes de usar o segredo, que está público no livro B.",
            run: async (c, s) => { await wait(300, s); c.bob.kill(); await c.advance(0.7, s, 900); } },
          { say: "<strong>T1 vence.</strong> Alice recupera o título: fica com os tokens <em>e</em> com o título.",
            run: async (c, s) => { await c.advance(T1, s, 700); await c.msg(c.P.alice, c.P.aTop, "DEVOLVER · T1 venceu", "no", s); c.a.set("back", "dono: Alice", "devolvido pelo prazo"); } },
        ],
      },
      delay: {
        outcome: OUT.neutral,
        foot: "<strong>Medido (H3):</strong> aborto seguro, mas 45 s de latência e <strong>35 s</strong> de capital preso. Pagou em vivacidade, não em segurança. Compare com T3 no capítulo 10.",
        steps: [s1,
          { say: "Rede lenta: <strong>+800 ms</strong> por mensagem. Alice tranca, a confirmação demora.",
            run: async (c, s) => { await c.msg(c.P.alice, c.P.aTop, "TRANCAR · H(s) · até T1  (+800 ms)", "", s, 1800); c.a.set("locked", "🔒 trancado com H(s)", "para Bob · devolve em T1"); await c.advance(0.3, s, 700); } },
          { say: "Bob calcula: a folga entre T2 e T1 ficou <strong>menor que o seguro</strong>. Seu cliente <strong>recusa trancar</strong>.",
            run: async (c, s) => { await c.advance(0.55, s, 800); c.bob.fade(); } },
          { say: "<strong>T1 vence.</strong> Alice recupera o título. Aborto correto, bloqueio longo.",
            run: async (c, s) => { await c.advance(T1, s, 900); await c.msg(c.P.alice, c.P.aTop, "DEVOLVER · T1 venceu", "no", s, 1600); c.a.set("back", "dono: Alice", "devolvido pelo prazo"); } },
        ],
      },
    };

    const p = new Player({
      build, scenarios,
      tabs: document.getElementById("tabs-htlc"),
      say: document.getElementById("s6-say"),
      outcome: document.getElementById("s6-outcome"),
      foot: document.getElementById("s6-foot"),
      play: document.getElementById("s6-play"),
      step: document.getElementById("s6-step"),
      reset: document.getElementById("s6-reset"),
    });
    p.load("ok");
  })();

  /* =================================================== cap. 8: Fabric */
  (function simFabric() {
    const stage = document.getElementById("stage-fabric");
    if (!stage) return;
    function box(svg, x, y, w, h, title, sub) {
      const g = el("g", {}, svg);
      const r = el("rect", { class: "pipe", x, y, width: w, height: h, rx: 6 }, g);
      el("text", { class: "pipe-title", x: x + w / 2, y: y + 26, "text-anchor": "middle", text: title }, g);
      if (sub) el("text", { class: "pipe-sub", x: x + w / 2, y: y + 44, "text-anchor": "middle", text: sub }, g);
      return { g, r, c: { x: x + w / 2, y: y + h / 2 }, left: { x, y: y + h / 2 }, right: { x: x + w, y: y + h / 2 }, bottom: { x: x + w / 2, y: y + h }, top: { x: x + w / 2, y },
        on() { r.classList.add("active"); }, off() { r.classList.remove("active"); } };
    }
    function build() {
      const svg = svgRoot(stage, 900, 300, "Cliente, peers que executam, orderer que corta blocos, peers que validam e o livro.");
      const cli = box(svg, 30, 100, 130, 60, "Cliente", "envia a proposta");
      const exe = box(svg, 215, 40, 170, 60, "1 · Executar", "peers simulam e assinam");
      const ord = box(svg, 440, 40, 170, 60, "2 · Ordenar", "orderer monta o bloco");
      const val = box(svg, 665, 40, 170, 60, "3 · Validar", "peers conferem e gravam");
      const led = box(svg, 665, 190, 170, 70, "Livro", "");
      el("text", { class: "lane-title", x: 440, y: 130, text: "ESPERA DO ORDERER · 2 s" }, svg);
      el("rect", { class: "tl-bar", x: 440, y: 138, width: 170, height: 10, rx: 3 }, svg);
      const clock = el("rect", { class: "tl-fill", x: 440, y: 138, width: 0, height: 10, rx: 3 }, svg);
      const clockText = el("text", { class: "clock", x: 525, y: 168, "text-anchor": "middle", text: "" }, svg);
      const blocks = el("g", {}, svg);
      [0, 1, 2].forEach((i) => { el("rect", { class: "lane", x: 680 + i * 38, y: 226, width: 30, height: 22, rx: 3 }, blocks); el("text", { class: "asset-state", x: 695 + i * 38, y: 241, "text-anchor": "middle", text: `#${41 + i}` }, blocks); });
      const newBlock = el("g", { class: "hidden" }, svg);
      el("rect", { class: "asset moved", x: 794, y: 226, width: 30, height: 22, rx: 3 }, newBlock);
      el("text", { class: "asset-state", x: 809, y: 241, "text-anchor": "middle", text: "#44" }, newBlock);
      el("path", { class: "wire", d: `M ${cli.right.x} ${cli.right.y} L ${exe.left.x} ${exe.left.y}` }, svg);
      el("path", { class: "wire", d: `M ${cli.right.x} ${cli.right.y} L ${ord.left.x} ${ord.left.y + 4}` }, svg);
      el("path", { class: "wire", d: `M ${ord.right.x} ${ord.right.y} L ${val.left.x} ${val.left.y}` }, svg);
      el("path", { class: "wire", d: `M ${val.bottom.x} ${val.bottom.y} L ${led.top.x} ${led.top.y}` }, svg);
      const total = el("text", { class: "clock", x: 95, y: 200, "text-anchor": "middle", text: "" }, svg);
      return { svg, cli, exe, ord, val, led, clock, clockText, newBlock, total,
        msg: (from, to, label, cls, s, ms) => message(svg, from, to, label, cls, ms, s) };
    }
    const scenarios = {
      one: {
        steps: [
          { say: "O cliente envia a <strong>proposta</strong> aos peers.",
            run: async (c, s) => { await c.msg(c.cli.right, c.exe.left, "proposta", "", s, 700); c.exe.on(); } },
          { say: "Cada peer simula o contrato e devolve o resultado <strong>assinado</strong>. Nada gravado ainda (~50 ms).",
            run: async (c, s) => { await wait(400, s); await c.msg(c.exe.left, c.cli.right, "resultado assinado ✓", "yes", s, 700); c.exe.off(); c.total.textContent = "50 ms"; } },
          { say: "O cliente envia a transação assinada ao <strong>orderer</strong>.",
            run: async (c, s) => { await c.msg(c.cli.right, { x: c.ord.left.x, y: c.ord.left.y + 4 }, "tx assinada", "", s, 800); c.ord.on(); } },
          { say: "O orderer espera até <strong>2 s</strong> para fechar o bloco. Só há uma transação, então espera tudo. (Barra em tempo real.)",
            run: async (c, s) => {
              c.clockText.textContent = "0,0 s";
              await tween(reducedMotion ? 0 : 2000, (t) => { c.clock.setAttribute("width", 170 * t); c.clockText.textContent = (2 * t).toFixed(1).replace(".", ",") + " s"; }, ease.linear, s);
              c.clockText.textContent = "bloco fechado";
            } },
          { say: "O bloco vai aos peers, que <strong>validam</strong> e gravam no livro.",
            run: async (c, s) => { c.ord.off(); await c.msg(c.ord.right, c.val.left, "bloco #44", "gold", s, 700); c.val.on(); await wait(300, s); await c.msg(c.val.bottom, c.led.top, "gravar", "yes", s, 500); c.val.off(); c.newBlock.classList.remove("hidden"); c.total.textContent = "≈ 2,1 s"; } },
          { say: "Uma escrita: ~<strong>2,1 s</strong>. Quatro em sequência: ~8 s. Duas rodadas em paralelo: ~4 s.",
            run: async () => {} },
        ],
      },
    };
    const p = new Player({
      build, scenarios,
      say: document.getElementById("s8-say"),
      play: document.getElementById("s8-play"),
      step: document.getElementById("s8-step"),
      reset: document.getElementById("s8-reset"),
    });
    p.load("one");
  })();
})();
