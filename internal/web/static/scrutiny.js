// Scrutiny's small client: everything else is htmx. Features register
// named actions in Scrutiny.actions; hotkeys and the Command Palette run them.
window.Scrutiny = { actions: {} };

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
    // Keep expanded panels and threads as the reader left them.
    const fresh = fragment.matches?.("details") ? fragment : fragment.querySelector?.("details");
    const old = target.matches("details") ? target : target.querySelector("details");
    if (fresh && old) fresh.open = old.open;
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

  Scrutiny.actions.next_comment = () => navigate(1);
  Scrutiny.actions.prev_comment = () => navigate(-1);
  Scrutiny.actions.refresh = () => location.reload();
  Scrutiny.actions.toggle_view_mode = () =>
    htmx.ajax("POST", "/settings/view-mode", {
      values: { mode: document.body.dataset.viewMode === "split" ? "unified" : "split" },
      swap: "none",
    });

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

  // position returns the commit, file, line and UTF-16 column under a mouse
  // event.
  function position(e) {
    const cell = e.target.closest("td.code[data-line]");
    if (!cell) return null;
    const table = cell.closest("[data-path]");
    const path = table?.dataset.path;
    const head = table?.dataset.head;
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
    return { head, path, line: Number(cell.dataset.line), col: col + offset };
  }

  function query(pos) {
    return new URLSearchParams({ head: pos.head, path: pos.path, line: pos.line, col: pos.col }).toString();
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
  Scrutiny.actions.symbol_search = () => {
    if (!palette.open) palette.showModal();
    input.select();
    search();
  };
})();

// Hotkeys and the Command Palette.
(() => {
  "use strict";
  const body = document.body;
  let bindings = JSON.parse(body.dataset.keys || "{}");
  const labels = {
    symbol_search: "Search files and symbols",
    command_palette: "Command palette",
    next_comment: "Next comment",
    prev_comment: "Previous comment",
    toggle_view_mode: "Toggle split/unified diff",
    refresh: "Refresh",
  };

  function toast(message) {
    const el = document.createElement("div");
    el.className = "toast";
    el.textContent = message;
    document.body.append(el);
    setTimeout(() => el.remove(), 4000);
  }

  // spec turns a keydown into a binding key such as "ctrl+k" or "]".
  // Shift is implied by characters it produces, so "]" not "shift+]".
  function spec(e) {
    if (["Control", "Alt", "Shift", "Meta"].includes(e.key)) return null;
    const key = e.key === " " ? "space" : e.key.length === 1 ? e.key.toLowerCase() : e.key.toLowerCase();
    const mods = [];
    if (e.ctrlKey) mods.push("ctrl");
    if (e.altKey) mods.push("alt");
    if (e.shiftKey && e.key.length > 1) mods.push("shift");
    if (e.metaKey) mods.push("meta");
    return [...mods, key].join("+");
  }
  const typing = (el) => el?.closest?.("input, textarea, select, [contenteditable]");

  let pending = [];
  let timer;
  let capture = null; // set while the Command Palette records a new binding

  // match finds the action bound to exactly typed, and whether typed starts
  // a longer binding. cmd counts as ctrl, for macOS.
  function match(typed) {
    const norm = (s) => s.replaceAll("meta+", "ctrl+");
    let prefix = false;
    for (const [action, binding] of Object.entries(bindings)) {
      const seq = norm(binding.split(/\s+/).join(" "));
      if (seq === norm(typed)) return { action };
      if (seq.startsWith(norm(typed) + " ")) prefix = true;
    }
    return { prefix };
  }

  document.addEventListener("keydown", (e) => {
    if (capture) return;
    if (e.key === "Escape") {
      document.querySelector(".context-menu")?.remove();
      document.getElementById("side-panel")?.remove();
      return;
    }
    const k = spec(e);
    if (!k) return;
    const modified = e.ctrlKey || e.altKey || e.metaKey;
    // Single-key bindings only work when no input has focus.
    if (!modified && typing(e.target)) return;
    clearTimeout(timer);
    // Try the sequence so far, then this key on its own, so a stray key
    // never swallows the next hotkey.
    for (const typed of [[...pending, k], [k]]) {
      const m = match(typed.join(" "));
      if (m.action && Scrutiny.actions[m.action]) {
        e.preventDefault();
        pending = [];
        Scrutiny.actions[m.action]();
        return;
      }
      if (m.prefix) {
        pending = typed;
        timer = setTimeout(() => (pending = []), 1000);
        return;
      }
    }
    pending = [];
  });

  // The Command Palette lists actions, and the page's own links that act
  // like commands (Stack, Since Last Review).
  const dialog = document.getElementById("commands");
  if (!dialog) return;
  const input = dialog.querySelector("input");
  const list = dialog.querySelector("ul");
  let items = [];
  let selected = 0;

  function link(id, label) {
    const a = document.getElementById(id);
    const href = a?.getAttribute("href") || a?.dataset.href;
    return href ? [{ label, run: () => (location.href = href) }] : [];
  }

  function commands() {
    const themes = JSON.parse(body.dataset.themes || "[]");
    return [
      ...Object.keys(labels)
        .filter((a) => a !== "command_palette" && Scrutiny.actions[a])
        .map((a) => ({ label: labels[a], hint: bindings[a], run: Scrutiny.actions[a] })),
      ...link("since-last-review", "Show changes since your last review"),
      ...link("stack-parent", "Go to the parent PR in the Stack"),
      ...link("stack-child", "Go to the child PR in the Stack"),
      { label: "Change theme…", run: () => show(themes.map((t) => ({ label: t.name, run: () => setTheme(t.id) }))) },
      { label: "Rebind a hotkey…", run: () => show(rebindItems()) },
      { label: "Go to the Dashboard", run: () => (location.href = "/") },
      { label: "Manage Language Servers", run: () => (location.href = "/settings/servers") },
    ];
  }

  function rebindItems() {
    return Object.keys(labels).map((a) => ({ label: `${labels[a]}`, hint: bindings[a], run: () => startCapture(a) }));
  }

  function setTheme(id) {
    htmx.ajax("POST", "/settings/theme", { values: { theme: id }, swap: "none" });
  }

  function show(next) {
    items = next;
    input.value = "";
    render();
    if (!dialog.open) dialog.showModal();
    input.focus();
  }

  // fuzzy scores label against q: its characters in order, preferring
  // word starts and runs; -1 when they do not all appear.
  function fuzzy(q, label) {
    const t = label.toLowerCase();
    let score = 0, ti = 0, prev = -2;
    for (const ch of q) {
      ti = t.indexOf(ch, ti);
      if (ti < 0) return -1;
      score += 1 + (ti === prev + 1 ? 5 : 0) + (ti === 0 || t[ti - 1] === " " ? 10 : 0);
      prev = ti++;
    }
    return score;
  }

  function render() {
    const q = input.value.toLowerCase().replace(/\s+/g, "");
    const shown = items
      .map((c) => ({ c, score: fuzzy(q, c.label) }))
      .filter((x) => x.score >= 0)
      .sort((a, b) => b.score - a.score)
      .map((x) => x.c);
    list.replaceChildren(
      ...shown.map((c, i) => {
        const li = document.createElement("li");
        const a = document.createElement("a");
        a.className = "result" + (i === 0 ? " selected" : "");
        a.href = "#";
        const name = document.createElement("span");
        name.className = "name";
        name.textContent = c.label;
        a.append(name);
        if (c.hint) {
          const hint = document.createElement("span");
          hint.className = "hint";
          hint.textContent = c.hint;
          a.append(hint);
        }
        a.addEventListener("click", (e) => {
          e.preventDefault();
          choose(c);
        });
        li.append(a);
        return li;
      }),
    );
    selected = 0;
    list._shown = shown;
  }

  function choose(c) {
    dialog.close();
    c.run();
  }

  function startCapture(action) {
    capture = { action, keys: [] };
    show([]);
    list.innerHTML = "";
    const p = document.createElement("li");
    p.className = "capture";
    p.textContent = `Press the new keys for “${labels[action]}”, then Enter. Escape cancels.`;
    list.append(p);
    input.blur();
    input.placeholder = "";
  }

  document.addEventListener("keydown", (e) => {
    if (!capture) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.key === "Escape") {
      capture = null;
      dialog.close();
      return;
    }
    if (e.key === "Enter" && capture.keys.length === 0) return;
    if (e.key === "Enter") {
      const { action, keys } = capture;
      capture = null;
      dialog.close();
      const binding = keys.join(" ");
      fetch("/settings/keys", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded", "X-CSRF-Token": document.querySelector('meta[name="csrf-token"]').content },
        body: new URLSearchParams({ action, binding }),
      }).then(async (res) => {
        if (!res.ok) return toast((await res.text()).trim());
        bindings = { ...bindings, [action]: binding };
        toast(`${labels[action]}: ${binding}`);
      });
      return;
    }
    const k = spec(e);
    if (!k || capture.keys.length >= 3) return;
    capture.keys.push(k);
    const p = list.querySelector(".capture");
    p.replaceChildren(`${labels[capture.action]}: `, ...capture.keys.map((key) => Object.assign(document.createElement("kbd"), { textContent: key })), " — Enter to save");
  }, true);

  input.addEventListener("input", render);
  input.addEventListener("keydown", (e) => {
    const links = list.querySelectorAll("a.result");
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      selected = (selected + (e.key === "ArrowDown" ? 1 : -1) + links.length) % Math.max(links.length, 1);
      links.forEach((a, i) => a.classList.toggle("selected", i === selected));
      links[selected]?.scrollIntoView({ block: "nearest" });
    } else if (e.key === "Enter") {
      e.preventDefault();
      const c = list._shown?.[selected];
      if (c) choose(c);
    }
  });
  dialog.addEventListener("close", () => (input.placeholder = "Type a command"));

  Scrutiny.actions.command_palette = () => show(commands());
})();
