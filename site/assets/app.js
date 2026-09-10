/* Navegação, tema, revelação e a animação ambiente da capa.
   Expõe window.Anim (tween/ease) para os simuladores. */
(function () {
  "use strict";

  const doc = document.documentElement;
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ------------------------------------------------------------ Anim */
  const ease = {
    inOut: (t) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2),
    out: (t) => 1 - Math.pow(1 - t, 3),
    linear: (t) => t,
  };

  /** Interpola de 0 a 1 durante `ms`, chamando onUpdate(t). Resolve ao fim.
      Sob prefers-reduced-motion, pula direto ao estado final. */
  function tween(ms, onUpdate, easing = ease.inOut, signal) {
    return new Promise((resolve) => {
      if (reducedMotion || ms <= 0) { onUpdate(1); resolve(); return; }
      const start = performance.now();
      function frame(now) {
        if (signal && signal.cancelled) { resolve(); return; }
        const t = Math.min(1, (now - start) / ms);
        onUpdate(easing(t));
        if (t < 1) requestAnimationFrame(frame); else resolve();
      }
      requestAnimationFrame(frame);
    });
  }

  function wait(ms, signal) {
    return new Promise((resolve) => {
      if (reducedMotion) { resolve(); return; }
      const id = setTimeout(resolve, ms);
      if (signal) signal.onCancel(() => { clearTimeout(id); resolve(); });
    });
  }

  /** Token de cancelamento simples, para abortar uma reprodução em curso. */
  function cancelToken() {
    const cbs = [];
    return {
      cancelled: false,
      cancel() { this.cancelled = true; cbs.forEach((cb) => cb()); },
      onCancel(cb) { cbs.push(cb); },
    };
  }

  window.Anim = { tween, wait, ease, cancelToken, reducedMotion };

  /* ----------------------------------------------------------- tema */
  const themeBtn = document.getElementById("theme-toggle");
  themeBtn.addEventListener("click", () => {
    const systemDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    const current = doc.dataset.theme || (systemDark ? "dark" : "light");
    const next = current === "dark" ? "light" : "dark";
    doc.dataset.theme = next;
    try { localStorage.setItem("theme", next); } catch (e) { /* sem storage, sem persistência */ }
  });

  /* ------------------------------------------------- trilho (mobile) */
  const rail = document.getElementById("rail");
  const railToggle = document.getElementById("rail-toggle");
  railToggle.addEventListener("click", () => {
    const open = rail.classList.toggle("open");
    railToggle.setAttribute("aria-expanded", String(open));
  });
  rail.addEventListener("click", (e) => {
    if (e.target.closest("a")) { rail.classList.remove("open"); railToggle.setAttribute("aria-expanded", "false"); }
  });
  document.addEventListener("click", (e) => {
    if (rail.classList.contains("open") && !rail.contains(e.target) && e.target !== railToggle && !railToggle.contains(e.target)) {
      rail.classList.remove("open");
      railToggle.setAttribute("aria-expanded", "false");
    }
  });

  /* -------------------------------------------- progresso + capítulo */
  const progressBar = document.getElementById("progress-bar");
  const chapters = Array.from(document.querySelectorAll("header.hero, section.chapter"));
  const railLinks = new Map(Array.from(rail.querySelectorAll("a[data-chapter]")).map((a) => [a.dataset.chapter, a]));

  function onScroll() {
    const max = doc.scrollHeight - window.innerHeight;
    progressBar.style.width = (max > 0 ? (window.scrollY / max) * 100 : 0) + "%";
  }
  window.addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  let activeId = null;
  function setActive(id) {
    if (id === activeId) return;
    activeId = id;
    railLinks.forEach((a, key) => a.classList.toggle("active", key === id));
    const link = railLinks.get(id);
    if (link && typeof link.scrollIntoView === "function") {
      link.scrollIntoView({ block: "nearest", behavior: reducedMotion ? "auto" : "smooth" });
    }
  }
  // O capítulo ativo é o que cruza a linha de leitura (35% da altura da janela).
  function pickActive() {
    const line = window.innerHeight * 0.35;
    let current = chapters[0];
    for (const c of chapters) {
      if (c.getBoundingClientRect().top <= line) current = c;
    }
    setActive(current.id);
  }
  window.addEventListener("scroll", pickActive, { passive: true });
  window.addEventListener("resize", pickActive);
  pickActive();

  /* -------------------------------------------------------- ampliar */
  // Toda figura com palco (ou um desenho SVG direto) ganha um botão que a
  // põe em tela cheia. Em tela cheia, → avança o passo e Esc fecha.
  const expandable = Array.from(document.querySelectorAll(".figure"))
    .filter((f) => f.querySelector(".sim-stage") || f.querySelector(":scope > svg") || f.querySelector(".chain"));
  function setExpanded(fig, on) {
    fig.classList.toggle("expanded", on);
    const btn = fig.querySelector(".expand-btn");
    if (btn) { btn.textContent = on ? "⤡ Fechar" : "⤢ Ampliar"; btn.setAttribute("aria-pressed", String(on)); }
    document.body.classList.toggle("has-expanded", document.querySelector(".figure.expanded") !== null);
  }
  expandable.forEach((fig) => {
    const head = fig.querySelector(".figure-head");
    if (!head) return;
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "btn btn-sm btn-ghost expand-btn";
    btn.textContent = "⤢ Ampliar";
    btn.setAttribute("aria-pressed", "false");
    btn.addEventListener("click", () => {
      const on = !fig.classList.contains("expanded");
      expandable.forEach((f) => { if (f !== fig) setExpanded(f, false); });
      setExpanded(fig, on);
      btn.blur();
    });
    head.appendChild(btn);
  });

  /* -------------------------------------------------------- teclado */
  function currentIndex() {
    return Math.max(0, chapters.findIndex((c) => c.id === activeId));
  }
  function goTo(idx) {
    const target = chapters[Math.max(0, Math.min(chapters.length - 1, idx))];
    if (!target) return;
    target.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "start" });
    history.replaceState(null, "", "#" + target.id);
  }
  document.addEventListener("keydown", (e) => {
    const tag = (e.target.tagName || "").toLowerCase();
    if (tag === "input" || tag === "textarea" || e.target.isContentEditable) return;
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    const open = document.querySelector(".figure.expanded");
    if (open) {
      if (e.key === "Escape") { e.preventDefault(); setExpanded(open, false); return; }
      if (e.key === "ArrowRight" || e.key === " ") {
        const step = open.querySelector('[id$="-step"]');
        if (step) { e.preventDefault(); if (!step.disabled) step.click(); }
      }
      return;
    }
    if (tag === "button") return;
    if (e.key === "ArrowRight" || e.key === "PageDown" || (e.key === " " && !e.shiftKey)) { e.preventDefault(); goTo(currentIndex() + 1); }
    else if (e.key === "ArrowLeft" || e.key === "PageUp" || (e.key === " " && e.shiftKey)) { e.preventDefault(); goTo(currentIndex() - 1); }
    else if (e.key === "Home") { e.preventDefault(); goTo(0); }
    else if (e.key === "End") { e.preventDefault(); goTo(chapters.length - 1); }
  });

  /* ------------------------------------------------------- revelação */
  // Estado de repouso é visível. Só o que está abaixo da dobra ganha .pending,
  // e um observer libera quando entra na tela.
  if (!reducedMotion && "IntersectionObserver" in window) {
    const io = new IntersectionObserver((entries) => {
      for (const en of entries) {
        if (en.isIntersecting) { en.target.classList.remove("pending"); io.unobserve(en.target); }
      }
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0.05 });
    document.querySelectorAll(".reveal").forEach((el) => {
      if (el.getBoundingClientRect().top > window.innerHeight) {
        el.classList.add("pending");
        io.observe(el);
      }
    });
  }

  /* ------------------------------------------------- capa: correntes */
  (function hero() {
    const svg = document.getElementById("hero-svg");
    if (!svg) return;
    const NS = "http://www.w3.org/2000/svg";
    const W = 78, H = 40, GAP = 22, STEP = W + GAP;
    const lanes = [
      { g: document.getElementById("hero-chain-a"), y: 60, n: 7, tag: "A" },
      { g: document.getElementById("hero-chain-b"), y: 176, n: 12, tag: "B" },
    ];
    const VISIBLE = 9;

    function hash(seed) {
      let h = 2166136261 ^ seed;
      h = Math.imul(h, 16777619) >>> 0;
      return h.toString(16).padStart(8, "0").slice(0, 6);
    }
    function block(lane, index, x) {
      const g = document.createElementNS(NS, "g");
      g.setAttribute("transform", `translate(${x} ${lane.y})`);
      const r = document.createElementNS(NS, "rect");
      r.setAttribute("class", "blk"); r.setAttribute("width", W); r.setAttribute("height", H); r.setAttribute("rx", 4);
      const t1 = document.createElementNS(NS, "text");
      t1.setAttribute("class", "blk-text"); t1.setAttribute("x", 8); t1.setAttribute("y", 16); t1.textContent = `#${index}`;
      const t2 = document.createElementNS(NS, "text");
      t2.setAttribute("class", "blk-text"); t2.setAttribute("x", 8); t2.setAttribute("y", 31); t2.textContent = hash(index * 7 + lane.tag.charCodeAt(0));
      const link = document.createElementNS(NS, "line");
      link.setAttribute("class", "link"); link.setAttribute("x1", W); link.setAttribute("y1", H / 2); link.setAttribute("x2", W + GAP); link.setAttribute("y2", H / 2);
      g.append(link, r, t1, t2);
      return g;
    }
    lanes.forEach((lane) => {
      lane.blocks = [];
      for (let i = 0; i < VISIBLE; i++) {
        const b = block(lane, lane.n + i, 24 + i * STEP);
        lane.g.appendChild(b);
        lane.blocks.push(b);
      }
      lane.n += VISIBLE;
      lane.offset = 0;
    });

    // Ponto que percorre o arco da troca, de um livro ao outro e de volta.
    const path = document.getElementById("hero-swap");
    const d1 = document.getElementById("hero-dot-1");
    const d2 = document.getElementById("hero-dot-2");
    const len = path.getTotalLength();

    if (reducedMotion) return;

    let last = performance.now();
    let swapT = 0;
    function frame(now) {
      const dt = Math.min(50, now - last); last = now;
      swapT = (swapT + dt / 5200) % 1;
      const p = path.getPointAtLength(len * (0.5 - 0.5 * Math.cos(Math.PI * swapT)));
      const q = path.getPointAtLength(len * (1 - (0.5 - 0.5 * Math.cos(Math.PI * swapT))));
      d1.setAttribute("cx", p.x); d1.setAttribute("cy", p.y);
      d2.setAttribute("cx", q.x); d2.setAttribute("cy", q.y);
      requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);

    // A cada ~2,4 s (o corte de bloco do Fabric é 2 s) cada corrente ganha um bloco.
    lanes.forEach((lane, li) => {
      setTimeout(() => {
        setInterval(() => {
          const nb = block(lane, lane.n++, 24 + VISIBLE * STEP);
          nb.querySelector("rect").classList.add("new");
          lane.g.appendChild(nb);
          lane.blocks.push(nb);
          const from = lane.offset, to = lane.offset - STEP;
          tween(700, (t) => {
            const x = from + (to - from) * t;
            lane.g.setAttribute("transform", `translate(${x} 0)`);
          }, ease.inOut).then(() => {
            lane.offset = to;
            const old = lane.blocks.shift();
            if (old) old.remove();
            setTimeout(() => nb.querySelector("rect").classList.remove("new"), 900);
          });
        }, 2400 + li * 300);
      }, li * 1200);
    });
  })();
})();
