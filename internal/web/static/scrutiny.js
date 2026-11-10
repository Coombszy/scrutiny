// Scrutiny's small client: everything else is htmx.
(() => {
  "use strict";

  // Show failed htmx requests as a toast, since htmx does not swap errors.
  function toast(message) {
    const el = document.createElement("div");
    el.className = "toast";
    el.textContent = message;
    document.body.append(el);
    setTimeout(() => el.remove(), 6000);
  }
  document.addEventListener("htmx:responseError", (e) => {
    toast(e.detail.xhr.responseText.trim() || e.detail.xhr.statusText);
  });
  document.addEventListener("htmx:sendError", () => toast("Scrutiny is not reachable."));

  document.addEventListener("click", (e) => {
    const open = e.target.closest("[data-open-dialog]");
    if (open) document.getElementById(open.dataset.openDialog)?.showModal();
    if (e.target.closest("[data-close-dialog]")) e.target.closest("dialog")?.close();
    if (e.target.closest("[data-remove-row]")) e.target.closest("tr")?.remove();
    if (e.target.closest("[data-reload]")) location.reload();
    const add = e.target.closest(".add-comment");
    if (add) {
      const row = add.closest("tr");
      if (row.nextElementSibling?.classList.contains("new-thread")) return;
      const url = `${add.closest("table").dataset.newThread}&side=${add.dataset.side}&line=${add.dataset.line}`;
      htmx.ajax("GET", url, { target: row, swap: "afterend" });
    }
    const nav = e.target.closest("[data-comment-nav]");
    if (nav) navigate(nav.dataset.commentNav === "next" ? 1 : -1);
  });

  // Live updates arrive as out-of-band HTML over an event stream, held open
  // only while the page is visible: the server polls GitHub only for PRs
  // someone is looking at.
  const live = document.querySelector("[data-live]");
  let stream = null;
  function connect() {
    if (stream || document.hidden || !live) return;
    stream = new EventSource(live.dataset.live);
    stream.addEventListener("update", (e) => htmx.swap(document.body, e.data, { swapStyle: "none" }));
  }
  function disconnect() {
    stream?.close();
    stream = null;
  }
  document.addEventListener("visibilitychange", () => (document.hidden ? disconnect() : connect()));
  window.addEventListener("pagehide", disconnect);
  connect();

  // Out-of-band swaps replace threads and the Conversation wholesale, so
  // carry over any unsent text and the focus.
  document.addEventListener("htmx:oobBeforeSwap", (e) => {
    const { target, fragment } = e.detail;
    for (const old of target.querySelectorAll("textarea[id]")) {
      const fresh = fragment.querySelector?.("#" + CSS.escape(old.id));
      if (fresh && old.value) fresh.textContent = old.value;
    }
    const focused = document.activeElement;
    if (focused?.id && target.contains(focused)) e.detail.refocus = focused.id;
  });
  document.addEventListener("htmx:oobAfterSwap", (e) => {
    if (e.detail.refocus) document.getElementById(e.detail.refocus)?.focus();
  });

  // Ctrl/Cmd+Enter submits the form a textarea is in.
  document.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && e.target.matches("textarea")) {
      e.preventDefault();
      e.target.form?.requestSubmit();
    }
  });

  // Viewed files collapse straight away; the checkbox posts via htmx.
  document.addEventListener("change", (e) => {
    if (e.target.matches(".viewed-toggle input")) {
      e.target.closest(".file")?.classList.toggle("viewed", e.target.checked);
    }
  });

  // Next/previous comment. Files load lazily (or, when large or generated,
  // on demand), so files with threads that have not loaded are loaded first.
  // Threads in Viewed (collapsed) files are skipped.
  async function loadThreadFiles(includeResolved) {
    const attr = includeResolved ? "threads" : "unresolved";
    const loads = [...document.querySelectorAll(".file:not(.viewed)")]
      .filter((f) => Number(f.dataset[attr] || 0) > 0)
      .map((f) => {
        const lazy = f.querySelector(".lazy-diff");
        if (lazy) return htmx.ajax("GET", lazy.getAttribute("hx-get"), { target: lazy, swap: "outerHTML" });
        const load = f.querySelector(".file-body > .note button[hx-get]");
        if (load) return htmx.ajax("GET", load.getAttribute("hx-get"), { target: f.querySelector(".file-body"), swap: "innerHTML" });
      })
      .filter(Boolean);
    await Promise.all(loads);
  }

  async function navigate(direction) {
    const includeResolved = document.getElementById("nav-include-resolved")?.checked;
    await loadThreadFiles(includeResolved);
    const threads = [...document.querySelectorAll(".thread")].filter(
      (t) => (includeResolved || t.dataset.resolved === "false") && t.getClientRects().length > 0,
    );
    if (threads.length === 0) return toast("No more comments.");
    const offset = 80; // below the sticky top bar
    const tops = threads.map((t) => t.getBoundingClientRect().top - offset);
    const target =
      direction > 0
        ? threads.find((_, i) => tops[i] > 1) ?? threads[0]
        : [...threads].reverse().find((t) => t.getBoundingClientRect().top - offset < -1) ?? threads.at(-1);
    target.querySelector("details")?.setAttribute("open", "");
    window.scrollTo({ top: window.scrollY + target.getBoundingClientRect().top - offset, behavior: "smooth" });
    document.querySelectorAll(".thread.current").forEach((t) => t.classList.remove("current"));
    target.classList.add("current");
  }
})();

// Code navigation: ctrl/cmd-click or right-click a head-side code cell
// (data-line) to go to a definition or find usages; ctrl+k for Symbol Search.
(() => {
  "use strict";
  const prPath = location.pathname.match(/^\/github\.com\/[^/]+\/[^/]+\/pull\/\d+/)?.[0];
  if (!prPath) return;

  function toast(message) {
    const el = document.createElement("div");
    el.className = "toast";
    el.textContent = message;
    document.body.append(el);
    setTimeout(() => el.remove(), 6000);
  }

  async function fetchOK(url) {
    const res = await fetch(url, { headers: { "HX-Request": "true" } });
    if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
    return res;
  }

  // position returns the file, line and UTF-16 column under a mouse event.
  function position(e) {
    const cell = e.target.closest("td.code[data-line]");
    if (!cell) return null;
    const path = cell.closest("[data-path]")?.dataset.path;
    let node, offset;
    if (document.caretPositionFromPoint) {
      const p = document.caretPositionFromPoint(e.clientX, e.clientY);
      node = p?.offsetNode;
      offset = p?.offset;
    } else if (document.caretRangeFromPoint) {
      const r = document.caretRangeFromPoint(e.clientX, e.clientY);
      node = r?.startContainer;
      offset = r?.startOffset;
    }
    if (!node || !cell.contains(node)) return null;
    let col = 0;
    const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT);
    for (let t = walker.nextNode(); t && t !== node; t = walker.nextNode()) col += t.data.length;
    return { path, line: Number(cell.dataset.line), col: col + offset };
  }

  function query(pos) {
    return new URLSearchParams({ path: pos.path, line: pos.line, col: pos.col }).toString();
  }

  // reveal scrolls to a line of a file shown on this page, loading the file's
  // diff first if needed. It returns false when the line is not on the page.
  async function reveal(path, line) {
    const file = [...document.querySelectorAll(".file[data-path]")].find((f) => f.dataset.path === path);
    if (!file) return false;
    if (file.classList.contains("viewed")) {
      file.classList.remove("viewed");
      const box = file.querySelector(".viewed-toggle input");
      if (box) box.checked = false;
    }
    const lazy = file.querySelector(".lazy-diff");
    if (lazy) await htmx.ajax("GET", lazy.getAttribute("hx-get"), { target: lazy, swap: "outerHTML" });
    const cell = file.querySelector(`td.code[data-line="${line}"]`);
    if (!cell) return false;
    cell.scrollIntoView({ block: "center" });
    const row = cell.closest("tr");
    row.classList.remove("flash");
    void row.offsetWidth;
    row.classList.add("flash");
    return true;
  }

  async function go(target) {
    if (target.outside) return toast(`Defined outside the repository: ${target.path}:${target.line}`);
    if (await reveal(target.path, target.line)) return;
    location.href = target.url;
  }

  async function definition(pos) {
    try {
      const targets = await (await fetchOK(`${prPath}/definition?${query(pos)}`)).json();
      if (targets.length === 0) return toast("No definition found.");
      if (targets.length === 1) return go(targets[0]);
      menu(lastPoint, targets.map((t) => [`${t.path}:${t.line}`, () => go(t)]));
    } catch (err) {
      toast(err.message);
    }
  }

  async function usages(pos) {
    try {
      const html = await (await fetchOK(`${prPath}/references?${query(pos)}`)).text();
      document.getElementById("side-panel")?.remove();
      document.body.insertAdjacentHTML("beforeend", html);
    } catch (err) {
      toast(err.message);
    }
  }

  // A small context menu of [label, action] items at a point.
  let lastPoint = { x: 0, y: 0 };
  function menu(at, items) {
    closeMenu();
    const el = document.createElement("div");
    el.className = "context-menu";
    for (const [label, action] of items) {
      const b = document.createElement("button");
      b.textContent = label;
      b.addEventListener("click", () => {
        closeMenu();
        action();
      });
      el.append(b);
    }
    el.style.left = `${Math.min(at.x, innerWidth - 220)}px`;
    el.style.top = `${Math.min(at.y, innerHeight - 40 * items.length)}px`;
    document.body.append(el);
  }
  function closeMenu() {
    document.querySelector(".context-menu")?.remove();
  }

  document.addEventListener("click", (e) => {
    if (!e.target.closest(".context-menu")) closeMenu();
    if (e.target.closest("[data-close-panel]")) document.getElementById("side-panel")?.remove();
    const link = e.target.closest("a[data-nav-path]");
    if (link && link.dataset.navInPr === "true" && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      document.getElementById("palette")?.close();
      const line = Number(link.dataset.navLine || 0);
      const path = link.dataset.navPath;
      (line ? reveal(path, line) : revealFile(path)).then((shown) => shown || (location.href = link.href));
      return;
    }
    if ((e.ctrlKey || e.metaKey) && e.button === 0) {
      const pos = position(e);
      if (!pos) return;
      e.preventDefault();
      lastPoint = { x: e.clientX, y: e.clientY };
      definition(pos);
    }
  });

  document.addEventListener("contextmenu", (e) => {
    const pos = position(e);
    if (!pos) return;
    e.preventDefault();
    lastPoint = { x: e.clientX, y: e.clientY };
    menu(lastPoint, [
      ["Go to definition", () => definition(pos)],
      ["Find usages", () => usages(pos)],
    ]);
  });

  async function revealFile(path) {
    const file = [...document.querySelectorAll(".file[data-path]")].find((f) => f.dataset.path === path);
    file?.scrollIntoView({ block: "start" });
    return !!file;
  }

  // Show a pointer over navigable code while ctrl/cmd is held.
  addEventListener("keydown", (e) => document.body.classList.toggle("nav-modifier", e.ctrlKey || e.metaKey));
  addEventListener("keyup", (e) => document.body.classList.toggle("nav-modifier", e.ctrlKey || e.metaKey));
  addEventListener("blur", () => document.body.classList.remove("nav-modifier"));

  // Symbol Search (ctrl+k).
  const palette = document.getElementById("palette");
  if (!palette) return;
  const input = palette.querySelector("input");
  const results = palette.querySelector(".palette-results");
  let timer, seq = 0, selected = 0;
  function select(i) {
    const items = results.querySelectorAll("a.result");
    if (items.length === 0) return;
    selected = (i + items.length) % items.length;
    items.forEach((a, j) => a.classList.toggle("selected", j === selected));
    items[selected].scrollIntoView({ block: "nearest" });
  }
  async function search() {
    const mine = ++seq;
    try {
      const html = await (await fetchOK(`${palette.dataset.symbols}?q=${encodeURIComponent(input.value)}`)).text();
      if (mine !== seq) return;
      results.innerHTML = html;
      select(0);
    } catch (err) {
      results.textContent = err.message;
    }
  }
  input.addEventListener("input", () => {
    clearTimeout(timer);
    timer = setTimeout(search, 120);
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      select(selected + (e.key === "ArrowDown" ? 1 : -1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      results.querySelectorAll("a.result")[selected]?.click();
    }
  });
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      if (!palette.open) palette.showModal();
      input.select();
      search();
    }
    if (e.key === "Escape") {
      closeMenu();
      document.getElementById("side-panel")?.remove();
    }
  });
})();
