// Scrutiny's small client: everything else is htmx. Features register
// named actions in Scrutiny.actions; hotkeys and the Command Palette run them.
window.Scrutiny = { actions: {} };

// The top bar's activity spinner shows while anything the user started is
// in flight: htmx requests, script fetches (Scrutiny.busy), and page loads
// that actions end in. It waits a moment so instant actions do not flicker;
// background polling marked data-quiet never shows it.
(() => {
  "use strict";
  const activity = document.getElementById("activity");
  let active = 0;
  let leaving = false;
  let timer;
  function update() {
    clearTimeout(timer);
    if (!activity) return;
    if (active > 0 || leaving) timer = setTimeout(() => (activity.hidden = false), 150);
    else activity.hidden = true;
  }
  function start() {
    active++;
    update();
  }
  function stop() {
    active = Math.max(0, active - 1);
    update();
  }
  Scrutiny.busy = (promise) => {
    start();
    return Promise.resolve(promise).finally(stop);
  };
  document.addEventListener("htmx:beforeRequest", (e) => {
    if (e.detail.elt?.closest?.("[data-quiet]")) return;
    e.detail.xhr.scrutinyBusy = true;
    start();
  });
  const done = (e) => {
    const xhr = e.detail.xhr;
    if (xhr?.scrutinyBusy) {
      xhr.scrutinyBusy = false;
      stop();
    }
  };
  document.addEventListener("htmx:afterRequest", done);
  document.addEventListener("htmx:sendError", done);
  addEventListener("beforeunload", () => {
    leaving = true;
    update();
  });
  // Back/forward cache restores the page as it was left.
  addEventListener("pageshow", (e) => {
    if (e.persisted) {
      leaving = false;
      update();
    }
  });
})();

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
    const toggle = e.target.closest("[data-full-toggle]");
    if (toggle) toggleFullFile(toggle);
    const jump = e.target.closest("[data-jump-thread]");
    if (jump) {
      e.preventDefault();
      jump.closest("dialog")?.close();
      jumpToThread(jump.dataset.jumpThread, jump.dataset.path);
    }
  });

  // toggleFullFile re-renders a file's diff with every line, or only the
  // changes again.
  async function toggleFullFile(button) {
    const file = button.closest(".file");
    const full = button.getAttribute("aria-pressed") !== "true";
    file.classList.add("peek"); // show it without changing its Viewed state
    await htmx.ajax("GET", file.dataset.fileUrl + (full ? "&full=1" : ""), { target: file.querySelector(".file-body"), swap: "innerHTML" });
    button.setAttribute("aria-pressed", String(full));
    button.textContent = full ? "Changes only" : "Full file";
  }

  // jumpToThread scrolls to a thread, first loading (and un-collapsing) its
  // file's diff if the thread is not on the page yet.
  async function jumpToThread(id, path) {
    let thread = document.getElementById("thread-" + id);
    if (!thread) {
      const file = [...document.querySelectorAll(".file[data-path]")].find((f) => f.dataset.path === path);
      if (file) {
        file.classList.add("peek");
        const lazy = file.querySelector(".lazy-diff");
        const load = file.querySelector(".file-body > .note button[hx-get]");
        if (lazy) await htmx.ajax("GET", lazy.getAttribute("hx-get"), { target: lazy, swap: "outerHTML" });
        else if (load) await htmx.ajax("GET", load.getAttribute("hx-get"), { target: file.querySelector(".file-body"), swap: "innerHTML" });
      }
      thread = document.getElementById("thread-" + id);
    }
    if (!thread) return toast("That comment is not shown in this Comparison.");
    thread.closest(".file")?.classList.add("peek");
    thread.querySelector("details")?.setAttribute("open", "");
    document.querySelectorAll(".thread.current").forEach((t) => t.classList.remove("current"));
    thread.classList.add("current");
    window.scrollTo({ top: window.scrollY + thread.getBoundingClientRect().top - 80, behavior: "smooth" });
  }

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

  // Edit swaps a comment's body for its edit form; Cancel restores it.
  document.addEventListener("click", (e) => {
    const edit = e.target.closest("[data-edit-comment], [data-cancel-edit]");
    if (!edit) return;
    const comment = edit.closest(".comment");
    const form = comment?.querySelector(".edit-comment");
    if (!form) return;
    const editing = edit.matches("[data-edit-comment]");
    if (!editing) form.reset();
    form.hidden = !editing;
    comment.querySelector(".markdown").hidden = editing;
    if (editing) {
      const box = form.querySelector("textarea");
      box.focus();
      box.setSelectionRange(box.value.length, box.value.length);
    }
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
      const file = e.target.closest(".file");
      file?.classList.toggle("viewed", e.target.checked);
      file?.classList.remove("peek");
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

  // Only the PR page has comments to step through and a View Mode.
  if (document.querySelector(".comment-nav")) {
    Scrutiny.actions.next_comment = () => navigate(1);
    Scrutiny.actions.prev_comment = () => navigate(-1);
    Scrutiny.actions.toggle_view_mode = () =>
      htmx.ajax("POST", "/settings/view-mode", {
        values: { mode: document.body.dataset.viewMode === "split" ? "unified" : "split" },
        swap: "none",
      });
  }
  // On the Dashboard, refresh means fetching its lists from GitHub again.
  Scrutiny.actions.refresh = () => {
    const button = document.querySelector("#dashboard-status button");
    if (!button) return location.reload();
    if (!button.disabled) button.click();
  };

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

// Code navigation on a head-side code cell (data-line): ctrl/cmd+click goes
// to the definition, ctrl/cmd+shift+click finds usages, and right-click
// offers both; ctrl+k for Symbol Search.
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
    const res = await Scrutiny.busy(fetch(url, { headers: { "HX-Request": "true" } }));
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
    file.classList.add("peek"); // show it without changing its Viewed state
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
  // The modifier for ctrl/cmd+click, as the platform writes it.
  const mac = /Mac|iPhone|iPad/.test(navigator.platform);
  const navMod = mac ? "⌘" : "Ctrl+";

  // menu shows items [label, action, shortcut?] at a point, with any
  // shortcut right-aligned.
  function menu(at, items) {
    closeMenu();
    const el = document.createElement("div");
    el.className = "context-menu";
    for (const [label, action, shortcut] of items) {
      const b = document.createElement("button");
      b.append(label);
      if (shortcut) {
        const hint = document.createElement("span");
        hint.className = "hint";
        hint.textContent = shortcut;
        b.append(hint);
      }
      b.addEventListener("click", () => {
        closeMenu();
        action();
      });
      el.append(b);
    }
    document.body.append(el);
    // Keep the menu on screen, measured now its shortcuts set its width.
    el.style.left = `${Math.max(0, Math.min(at.x, innerWidth - el.offsetWidth - 8))}px`;
    el.style.top = `${Math.max(0, Math.min(at.y, innerHeight - el.offsetHeight - 8))}px`;
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
      (e.shiftKey ? usages : definition)(pos);
    }
  });

  // shift+mousedown would extend the text selection before ctrl/cmd+shift+click
  // finds usages.
  document.addEventListener("mousedown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.button === 0 && position(e)) e.preventDefault();
  });

  document.addEventListener("contextmenu", (e) => {
    const pos = position(e);
    if (!pos) return;
    e.preventDefault();
    lastPoint = { x: e.clientX, y: e.clientY };
    menu(lastPoint, [
      ["Go to definition", () => definition(pos), `${navMod}Click`],
      ["Find usages", () => usages(pos), `${navMod}${mac ? "⇧" : "Shift+"}Click`],
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

// Dashboard search (ctrl+k): the PRs listed on the page, or one named by
// URL or owner/repo#123.
(() => {
  "use strict";
  const dialog = document.getElementById("pr-search");
  if (!dialog) return;
  const input = dialog.querySelector("input");
  const list = dialog.querySelector("ul");
  let shown = [];
  let selected = 0;

  // named turns a pasted GitHub PR URL or owner/repo#123 into its path here.
  function named(q) {
    const m =
      q.match(/^https?:\/\/github\.com\/([\w.-]+)\/([\w.-]+)\/pull\/(\d+)/) || q.match(/^([\w.-]+)\/([\w.-]+)#(\d+)$/);
    return m && { title: `Open ${m[1]}/${m[2]}#${m[3]}`, ref: "", href: `/github.com/${m[1]}/${m[2]}/pull/${m[3]}` };
  }

  // The PRs on the page, once each, in page order.
  function prs() {
    const seen = new Set();
    return [...document.querySelectorAll(".pr-list li")]
      .map((li) => {
        const a = li.querySelector("a");
        return { title: a.textContent.trim(), ref: li.querySelector(".muted")?.textContent.trim() ?? "", href: a.getAttribute("href") };
      })
      .filter((p) => !seen.has(p.href) && seen.add(p.href));
  }

  function fuzzy(q, text) {
    const t = text.toLowerCase();
    let score = 0, ti = 0, prev = -2;
    for (const ch of q) {
      ti = t.indexOf(ch, ti);
      if (ti < 0) return -1;
      score += 1 + (ti === prev + 1 ? 5 : 0) + (ti === 0 || " /#-_".includes(t[ti - 1]) ? 10 : 0);
      prev = ti++;
    }
    return score;
  }

  function render() {
    const q = input.value.trim();
    const target = named(q);
    const needle = q.toLowerCase().replace(/\s+/g, "");
    shown = [
      ...(target ? [target] : []),
      ...prs()
        .map((p) => ({ p, score: Math.max(fuzzy(needle, p.title), fuzzy(needle, p.ref)) }))
        .filter((x) => x.score >= 0)
        .sort((a, b) => b.score - a.score)
        .map((x) => x.p),
    ];
    selected = 0;
    list.replaceChildren(
      ...shown.map((p, i) => {
        const a = Object.assign(document.createElement("a"), { className: "result" + (i ? "" : " selected"), href: p.href });
        a.append(Object.assign(document.createElement("span"), { className: "name", textContent: p.title }));
        if (p.ref) a.append(Object.assign(document.createElement("span"), { className: "where muted", textContent: p.ref }));
        const li = document.createElement("li");
        li.append(a);
        return li;
      }),
    );
    if (shown.length === 0) list.replaceChildren(Object.assign(document.createElement("li"), { className: "muted", textContent: "No matching pull requests." }));
  }

  input.addEventListener("input", render);
  input.addEventListener("keydown", (e) => {
    const links = list.querySelectorAll("a.result");
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      selected = (selected + (e.key === "ArrowDown" ? 1 : -1) + links.length) % Math.max(links.length, 1);
      links.forEach((a, i) => a.classList.toggle("selected", i === selected));
      links[selected]?.scrollIntoView({ block: "nearest" });
    } else if (e.key === "Enter" && shown[selected]) {
      e.preventDefault();
      location.href = shown[selected].href;
    }
  });

  Scrutiny.actions.symbol_search = () => {
    if (!dialog.open) dialog.showModal();
    input.select();
    render();
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
    help: "Show keyboard shortcuts",
  };
  if (document.getElementById("pr-search")) {
    labels.symbol_search = "Search pull requests";
    labels.refresh = "Refresh pull requests from GitHub";
  }

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

  // Keyboard shortcuts help, built from the live bindings so it is right
  // straight after a rebind. Only pages that include the dialog offer it.
  const help = document.getElementById("help");
  if (help) {
    Scrutiny.actions.help = () => {
      const kbd = (key) => Object.assign(document.createElement("kbd"), { textContent: key });
      help.querySelector("tbody").replaceChildren(
        ...Object.keys(labels)
          .filter((a) => bindings[a] && Scrutiny.actions[a])
          .map((a) => {
            const keys = document.createElement("td");
            keys.append(...bindings[a].split(/\s+/).flatMap((k, i) => (i ? [" then ", kbd(k)] : [kbd(k)])));
            const what = Object.assign(document.createElement("td"), { textContent: labels[a] });
            const tr = document.createElement("tr");
            tr.append(keys, what);
            return tr;
          }),
      );
      if (!help.open) help.showModal();
    };
  }

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
      { label: "Open settings", run: () => (location.href = "/settings") },
      { label: "Manage Language Servers", run: () => (location.href = "/settings#language-servers") },
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
      Scrutiny.busy(fetch("/settings/keys", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded", "X-CSRF-Token": document.querySelector('meta[name="csrf-token"]').content },
        body: new URLSearchParams({ action, binding }),
      })).then(async (res) => {
        if (!res.ok) return toast((await res.text()).trim());
        bindings = { ...bindings, [action]: binding };
        const hint = action === "help" && document.querySelector("#help-hint kbd");
        if (hint) hint.textContent = binding;
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
