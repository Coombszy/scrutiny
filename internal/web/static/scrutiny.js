// Scrutiny's small client: everything else is htmx. Features register
// named actions in Scrutiny.actions; hotkeys and the Command Palette run them.
window.Scrutiny = { actions: {} };

// While anything the user started is in flight, the cursor shows progress:
// htmx requests from their clicks, forms and hotkeys, script work wrapped
// in Scrutiny.busy (code navigation, jumping to comments), and the page
// load an action ends in (switching View Mode or Theme). Requests the page
// makes by itself (on load, on scroll, polling, or marked data-quiet) do not.
(() => {
  "use strict";
  let active = 0;
  let leaving = false;
  let timer;
  function update() {
    clearTimeout(timer);
    // A short delay keeps instant actions from flickering the cursor.
    if (active > 0 || leaving) timer = setTimeout(() => document.body.classList.add("busy"), 80);
    else document.body.classList.remove("busy");
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
  const automatic = /\b(load|every|intersect|revealed)\b/;
  function userCaused(elt) {
    if (!elt?.closest) return true;
    if (elt.closest("[data-quiet]")) return false;
    return !automatic.test(elt.getAttribute("hx-trigger") || "");
  }
  document.addEventListener("htmx:beforeRequest", (e) => {
    if (!userCaused(e.detail.elt)) return;
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

// Scrutiny.pin scrolls el to offset px below the sticky top bar and holds
// it there while the page settles. Diffs load lazily as they come into view,
// including just above the target where the top bar covers them, and each
// would otherwise push the target out of place; so would the browser's own
// late jump to a #fragment, which it undoes for its first second. It lets go
// once nothing has moved for a moment, or as soon as the user scrolls or
// clicks.
(() => {
  "use strict";
  let release = null;
  Scrutiny.barBottom = () => document.querySelector(".topbar")?.getBoundingClientRect().bottom ?? 0;
  Scrutiny.pin = (el, offset = 0) => {
    release?.();
    const place = () => {
      if (!el.isConnected) return stop();
      const off = el.getBoundingClientRect().top - Scrutiny.barBottom() - offset;
      if (Math.abs(off) > 1) window.scrollBy(0, off);
      clearTimeout(quiet);
      quiet = setTimeout(stop, 1500);
    };
    const resize = new ResizeObserver(place);
    const user = ["wheel", "touchstart", "pointerdown", "keydown"];
    const started = Date.now();
    const scrolled = () => Date.now() - started < 1000 && place();
    let quiet;
    const giveUp = setTimeout(stop, 8000);
    function stop() {
      resize.disconnect();
      clearTimeout(quiet);
      clearTimeout(giveUp);
      for (const t of user) removeEventListener(t, stop, true);
      removeEventListener("scroll", scrolled);
      release = null;
    }
    release = stop;
    for (const t of user) addEventListener(t, stop, { capture: true, passive: true });
    addEventListener("scroll", scrolled, { passive: true });
    resize.observe(document.body);
    place();
  };
})();

// A link to a line (…#L148) puts it in the middle of the page, held there
// while the page settles, rather than at the top where the browser leaves
// it under the sticky top bar. The browser's own jump comes first, so this
// runs once the page has loaded, and again when the hash changes.
(() => {
  "use strict";
  function centre() {
    const n = location.hash.match(/^#L(\d+)$/)?.[1];
    const row = n && document.getElementById("L" + n);
    if (row) Scrutiny.pin(row, Math.max(0, (innerHeight - Scrutiny.barBottom() - row.offsetHeight) / 2));
  }
  // centreHash follows a link to a line on this same page: setting the hash
  // centres it, and one already set (no hashchange) is centred again.
  Scrutiny.centreHash = (hash) => {
    if (location.hash === hash) centre();
    else location.hash = hash;
  };
  addEventListener("load", () => requestAnimationFrame(centre));
  addEventListener("hashchange", centre);
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
  // Scrutiny.confirm asks a question in the confirm dialog rather than the
  // browser's own, resolving true if the user chose OK. opts.label names
  // the OK button and opts.danger colours it as destructive.
  const confirmDialog = document.getElementById("confirm");
  Scrutiny.confirm = (message, opts = {}) =>
    new Promise((resolve) => {
      const ok = confirmDialog.querySelector(".confirm-ok");
      confirmDialog.querySelector(".confirm-message").textContent = message;
      ok.textContent = opts.label || "OK";
      ok.classList.toggle("danger", !!opts.danger);
      confirmDialog.returnValue = "";
      confirmDialog.addEventListener("close", () => resolve(confirmDialog.returnValue === "ok"), { once: true });
      confirmDialog.showModal();
    });
  // hx-confirm questions go through it too; data-confirm-label names the
  // OK button, and a .danger element's is coloured as destructive.
  document.addEventListener("htmx:confirm", (e) => {
    const { question, elt, issueRequest } = e.detail;
    if (!question) return;
    e.preventDefault();
    Scrutiny.confirm(question, { label: elt.dataset.confirmLabel, danger: elt.classList.contains("danger") })
      .then((yes) => yes && issueRequest(true));
  });

  // GitHub rejecting the credentials gets the dialog rather than a toast:
  // it needs the user to log in again, not just to notice.
  const authFailed = document.getElementById("auth-failed");
  Scrutiny.authFailed = () => {
    if (authFailed && !authFailed.open) authFailed.showModal();
  };
  if (authFailed?.hasAttribute("data-open-on-load")) Scrutiny.authFailed();
  document.addEventListener("htmx:responseError", (e) => {
    if (e.detail.xhr.status === 401) Scrutiny.authFailed();
    else toast(e.detail.xhr.responseText.trim() || e.detail.xhr.statusText);
  });
  document.addEventListener("htmx:sendError", () => toast("Scrutiny is not reachable."));

  document.addEventListener("click", (e) => {
    const open = e.target.closest("[data-open-dialog]");
    if (open) document.getElementById(open.dataset.openDialog)?.showModal();
    if (e.target.closest("[data-close-dialog]")) e.target.closest("dialog")?.close();
    // A click on a palette's backdrop lands on the dialog itself, outside its box.
    if (e.target.matches("dialog.palette[open]")) {
      const r = e.target.getBoundingClientRect();
      if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) e.target.close();
    }
    if (e.target.closest("[data-remove-row]")) e.target.closest("tr")?.remove();
    if (e.target.closest("[data-reload]")) location.reload();
    const nav = e.target.closest("[data-comment-nav]");
    if (nav) {
      const dir = nav.dataset.commentNav;
      Scrutiny.busy(dir === "current" ? goToCurrent() : navigate(dir === "next" ? 1 : -1));
    }
    const collapse = e.target.closest("[data-file-collapse]");
    if (collapse) {
      const file = collapse.closest(".file");
      setCollapsed(file, !file.classList.contains("collapsed"));
    }
    const toggle = e.target.closest("[data-full-toggle]");
    if (toggle) Scrutiny.busy(toggleFullFile(toggle));
    const jump = e.target.closest("[data-jump-thread]");
    if (jump) {
      e.preventDefault();
      jump.closest("dialog")?.close();
      Scrutiny.busy(jumpToThread(jump.dataset.jumpThread, jump.dataset.path));
    }
  });

  // setCollapsed folds or opens a file's diff. It is separate from Viewed:
  // ticking Viewed collapses a file, but it can be opened (by hand, or by
  // going to a comment or line in it) and stay Viewed.
  function setCollapsed(file, collapsed) {
    if (!file) return;
    file.classList.toggle("collapsed", collapsed);
    file.querySelector("[data-file-collapse]")?.setAttribute("aria-expanded", String(!collapsed));
  }
  Scrutiny.setCollapsed = setCollapsed;

  // toggleFullFile re-renders a file's diff with every line, or only the
  // changes again.
  async function toggleFullFile(button) {
    const file = button.closest(".file");
    const full = button.getAttribute("aria-pressed") !== "true";
    setCollapsed(file, false);
    await htmx.ajax("GET", file.dataset.fileUrl + (full ? "&full=1" : ""), { source: file.querySelector(".file-body"), target: file.querySelector(".file-body"), swap: "innerHTML" });
    button.setAttribute("aria-pressed", String(full));
    button.textContent = full ? "Changes only" : "Full file";
  }

  // jumpToThread scrolls to a thread, first loading its file's diff if the
  // thread is not on the page yet, and opens the file if it is collapsed.
  async function jumpToThread(id, path) {
    let thread = document.getElementById("thread-" + id);
    if (!thread) {
      const file = [...document.querySelectorAll(".file[data-path]")].find((f) => f.dataset.path === path);
      if (file) {
        const lazy = file.querySelector(".lazy-diff");
        const load = file.querySelector(".file-body > .note button[hx-get]");
        if (lazy) await htmx.ajax("GET", lazy.getAttribute("hx-get"), { source: lazy, target: lazy, swap: "outerHTML" });
        else if (load) await htmx.ajax("GET", load.getAttribute("hx-get"), { source: file.querySelector(".file-body"), target: file.querySelector(".file-body"), swap: "innerHTML" });
      }
      thread = document.getElementById("thread-" + id);
    }
    if (!thread) return toast("That comment is not shown in this Comparison.");
    setCollapsed(thread.closest(".file"), false);
    thread.querySelector("details")?.setAttribute("open", "");
    select(thread);
    Scrutiny.pin(thread, 40);
  }

  // select marks thread as the selected comment, which Current returns to,
  // and keeps it in the URL (?comment=) so a reload, such as Refresh or
  // toggling the View Mode, selects it again.
  function select(thread) {
    document.querySelectorAll(".thread.current").forEach((t) => t.classList.remove("current"));
    thread?.classList.add("current");
    const url = new URL(location.href);
    if (thread) url.searchParams.set("comment", thread.id.replace(/^thread-/, ""));
    else url.searchParams.delete("comment");
    history.replaceState(history.state, "", url);
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
    stream.addEventListener("unauthorized", () => Scrutiny.authFailed());
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

  // A [data-copy] button copies its value, e.g. a comment's ID, and says so.
  document.addEventListener("click", async (e) => {
    const btn = e.target.closest("[data-copy]");
    if (!btn) return;
    e.preventDefault(); // inside a thread's summary, do not also toggle it
    try {
      await navigator.clipboard.writeText(btn.dataset.copy);
    } catch {
      toast("Could not copy to the clipboard.");
      return;
    }
    if (btn.dataset.copied) return;
    btn.dataset.copied = "true";
    const label = btn.textContent;
    btn.style.minWidth = btn.offsetWidth + "px";
    btn.textContent = "Copied";
    setTimeout(() => {
      btn.textContent = label;
      btn.style.minWidth = "";
      delete btn.dataset.copied;
    }, 1200);
  });

  // Ctrl/Cmd+Enter submits the form a textarea is in.
  document.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && e.target.matches("textarea")) {
      e.preventDefault();
      e.target.form?.requestSubmit();
    }
  });

  // Ticking Viewed collapses the file straight away, and unticking opens
  // it; the checkbox posts via htmx.
  document.addEventListener("change", (e) => {
    if (e.target.matches(".viewed-toggle input")) {
      const file = e.target.closest(".file");
      file?.classList.toggle("viewed", e.target.checked);
      setCollapsed(file, e.target.checked);
    }
  });

  // Next/previous comment. Files load lazily (or, when large or generated,
  // on demand), so files with threads that have not loaded are loaded first.
  // Collapsed (e.g. Viewed) files are included: going to a comment in one
  // opens it.
  async function loadThreadFiles(includeResolved) {
    const attr = includeResolved ? "threads" : "unresolved";
    const files = [...document.querySelectorAll(".file")].filter((f) => Number(f.dataset[attr] || 0) > 0);
    await Promise.all(files.map(loadDiff));
  }

  // loadDiff loads a file's diff if it is not on the page yet. Like every
  // htmx.ajax here it names its own source: without one htmx uses <body> for
  // all of them, and a request made while another is in flight is queued
  // with only the last kept, its promise resolving at once with nothing
  // loaded.
  function loadDiff(f) {
    const lazy = f.querySelector(".lazy-diff");
    if (lazy) return htmx.ajax("GET", lazy.getAttribute("hx-get"), { source: lazy, target: lazy, swap: "outerHTML" });
    const load = f.querySelector(".file-body > .note button[hx-get]");
    if (load) return htmx.ajax("GET", load.getAttribute("hx-get"), { source: f.querySelector(".file-body"), target: f.querySelector(".file-body"), swap: "innerHTML" });
  }

  // restoreSelection selects the comment named in the URL again after a
  // reload, loading the diffs with threads to find it; it leaves the scroll
  // where the browser put it back. A comment no longer shown is dropped.
  async function restoreSelection() {
    const id = new URL(location.href).searchParams.get("comment");
    if (!id) return;
    const find = () => document.getElementById("thread-" + id);
    if (!find()) await Promise.all([...document.querySelectorAll(".file")].filter((f) => Number(f.dataset.threads || 0) > 0).map(loadDiff));
    // Selected again, but left as the page renders it: a resolved thread
    // stays folded and a Viewed file collapsed. Going to it opens it.
    select(find());
  }

  // A reload (Refresh, toggling the View Mode, the browser's own) puts the
  // page back where it was. Diffs load lazily, so a reloaded page is shorter
  // than the one left and a pixel offset would land elsewhere; instead the
  // first thing in view (a diff line, a thread or a file) is noted with its
  // distance below the top bar, and put back there once the diffs opened by
  // hand are open again. Lines are found by number, so they survive a
  // switch between split and unified.
  const viewKey = "scrutiny:view:" + location.pathname;
  const barBottom = Scrutiny.barBottom;
  const fileKey = (f) => f.dataset.path ?? "";

  // lineNumbers reads a diff row's old and new line numbers (0 for none):
  // both views put the old one in the first .ln cell and the new in the
  // second.
  function lineNumbers(tr) {
    const cells = tr.querySelectorAll(":scope > td.ln");
    const num = (td) => Number(td?.textContent.replace(/\D/g, "")) || 0;
    return { old: num(cells[0]), new: num(cells[1]) };
  }

  function captureAnchor() {
    const bar = barBottom();
    for (const file of document.querySelectorAll(".file")) {
      const box = file.getBoundingClientRect();
      if (box.bottom <= bar) continue;
      for (const el of file.querySelectorAll("tr, .thread[id]")) {
        const r = el.getBoundingClientRect();
        if (r.bottom <= bar || r.height === 0 || el.classList.contains("thread-row")) continue;
        if (el.matches(".thread")) return { path: fileKey(file), thread: el.id, offset: r.top - bar };
        const n = lineNumbers(el);
        if (n.old || n.new) return { path: fileKey(file), old: n.old, new: n.new, offset: r.top - bar };
      }
      return { path: fileKey(file), offset: box.top - bar };
    }
    return null;
  }

  // findAnchor finds the noted element again, and the offset to put it at;
  // a line no longer shown (e.g. from expanded context) falls back to the
  // top of its file.
  function findAnchor(a) {
    if (a.thread) {
      const thread = document.getElementById(a.thread);
      if (thread) return [thread, a.offset];
    }
    const file = [...document.querySelectorAll(".file")].find((f) => fileKey(f) === a.path);
    if (!file) return [null, 0];
    if (a.old || a.new) {
      const row = [...file.querySelectorAll("tr")].find((tr) => {
        const n = lineNumbers(tr);
        return a.new ? n.new === a.new : n.old === a.old;
      });
      return row ? [row, a.offset] : [file, 0];
    }
    return [file, a.offset];
  }

  function captureView() {
    const files = [...document.querySelectorAll(".file[data-path]")];
    const paths = (test) => files.filter(test).map(fileKey);
    return {
      y: scrollY,
      anchor: captureAnchor(),
      conversation: !!document.querySelector(".conversation-panel")?.open,
      full: paths((f) => f.querySelector('[data-full-toggle][aria-pressed="true"]')),
      loaded: paths((f) => !!f.querySelector(".file-body table")),
    };
  }

  async function restoreView() {
    let saved = null;
    try {
      saved = JSON.parse(sessionStorage.getItem(viewKey));
      sessionStorage.removeItem(viewKey);
    } catch {}
    const nav = performance.getEntriesByType("navigation")[0]?.type;
    if (nav !== "reload" && nav !== "back_forward") saved = null;
    await restoreSelection();
    if (!saved) return;

    if (saved.conversation) document.querySelector(".conversation-panel")?.setAttribute("open", "");
    // Viewed files opened for a look are not reopened: they, like resolved
    // threads, start collapsed on every load, and files collapsed by hand
    // start open.
    const byPath = new Map([...document.querySelectorAll(".file[data-path]")].map((f) => [fileKey(f), f]));
    // Re-open what was opened by hand, and the anchor's own diff; lazy
    // diffs elsewhere load as they come into view, as before.
    const anchorFile = byPath.get(saved.anchor?.path);
    const loads = [];
    for (const p of saved.full) {
      const f = byPath.get(p);
      const button = f?.querySelector("[data-full-toggle]");
      if (button && !f.classList.contains("collapsed")) loads.push(toggleFullFile(button)); // it would open a collapsed file
    }
    for (const p of saved.loaded) {
      const f = byPath.get(p);
      if (f && !saved.full.includes(p) && (f === anchorFile || f.querySelector(".file-body > .note button[hx-get]"))) loads.push(loadDiff(f));
    }
    await Promise.all(loads);

    const [el, offset] = saved.anchor ? findAnchor(saved.anchor) : [null, 0];
    if (el) Scrutiny.pin(el, offset);
    else window.scrollTo(0, saved.y);
  }

  // Only the PR page has comments to step through and a View Mode.
  if (document.querySelector(".comment-nav")) {
    Scrutiny.actions.next_comment = () => Scrutiny.busy(navigate(1));
    Scrutiny.actions.prev_comment = () => Scrutiny.busy(navigate(-1));
    Scrutiny.actions.current_comment = () => Scrutiny.busy(goToCurrent());
    history.scrollRestoration = "manual"; // restoreView does it
    addEventListener("pagehide", () => {
      try {
        sessionStorage.setItem(viewKey, JSON.stringify(captureView()));
      } catch {}
    });
    addEventListener("load", () => Scrutiny.busy(restoreView()));
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
    // A thread in a collapsed file is not laid out, so it stands at its
    // file's header.
    const at = (t) => (t.getClientRects().length > 0 ? t : t.closest(".file.collapsed"));
    const threads = [...document.querySelectorAll(".thread:not(.new)")].filter(
      (t) => (includeResolved || t.dataset.resolved === "false") && at(t),
    );
    if (threads.length === 0) return toast("No more comments.");
    const offset = 80; // below the sticky top bar
    const tops = threads.map((t) => at(t).getBoundingClientRect().top - offset);
    const target =
      direction > 0
        ? threads.find((_, i) => tops[i] > 1) ?? threads[0]
        : threads.findLast((_, i) => tops[i] < -1) ?? threads.at(-1);
    setCollapsed(target.closest(".file"), false);
    target.querySelector("details")?.setAttribute("open", "");
    select(target);
    Scrutiny.pin(target, 40);
  }

  // goToCurrent scrolls back to the comment ↑/↓ or a jump last selected,
  // e.g. after scrolling away to read code; with none, it selects the next.
  async function goToCurrent() {
    const thread = document.querySelector(".thread.current");
    if (!thread) return navigate(1);
    setCollapsed(thread.closest(".file"), false);
    thread.querySelector("details")?.setAttribute("open", "");
    Scrutiny.pin(thread, 40);
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
    Scrutiny.setCollapsed(file, false); // open it without changing its Viewed state
    const lazy = file.querySelector(".lazy-diff");
    if (lazy) await htmx.ajax("GET", lazy.getAttribute("hx-get"), { source: lazy, target: lazy, swap: "outerHTML" });
    const cell = file.querySelector(`td.code[data-line="${line}"]`);
    if (!cell) return false;
    const row = cell.closest("tr");
    Scrutiny.pin(row, Math.max(0, (innerHeight - Scrutiny.barBottom() - row.offsetHeight) / 2));
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

  // definition and usages keep the progress cursor up for the whole flow:
  // waiting on the Language Server, then loading the diff to land in.
  const definition = (pos) => Scrutiny.busy(findDefinition(pos));
  const usages = (pos) => Scrutiny.busy(findUsages(pos));

  async function findDefinition(pos) {
    try {
      const targets = await (await fetchOK(`${prPath}/definition?${query(pos)}`)).json();
      if (targets.length === 0) return toast("No definition found.");
      if (targets.length === 1) return go(targets[0]);
      menu(lastPoint, targets.map((t) => [`${t.path}:${t.line}`, () => go(t)]));
    } catch (err) {
      toast(err.message);
    }
  }

  async function findUsages(pos) {
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
    if (link && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
      // A result chosen in Symbol Search closes it, including one on this
      // page, which the browser follows without leaving it.
      document.getElementById("palette")?.close();
      const url = new URL(link.href);
      if (link.dataset.navInPr !== "true" && url.pathname + url.search === location.pathname + location.search && url.hash) {
        e.preventDefault();
        Scrutiny.centreHash(url.hash);
        return;
      }
    }
    if (link && link.dataset.navInPr === "true" && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      const line = Number(link.dataset.navLine || 0);
      const path = link.dataset.navPath;
      Scrutiny.busy((line ? reveal(path, line) : revealFile(path)).then((shown) => shown || (location.href = link.href)));
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
    const items = [
      ["Go to definition", () => definition(pos), `${navMod}Click`],
      ["Find usages", () => usages(pos), `${navMod}${mac ? "⇧" : "Shift+"}Click`],
    ];
    // This menu replaces the browser's, so it offers Copy for a selection.
    const selected = getSelection().toString();
    if (selected) items.unshift(["Copy", () => copy(selected), `${navMod}C`]);
    menu(lastPoint, items);
  });

  async function copy(text) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      if (!document.execCommand("copy")) toast("Could not copy to the clipboard.");
    }
  }

  async function revealFile(path) {
    const file = [...document.querySelectorAll(".file[data-path]")].find((f) => f.dataset.path === path);
    if (file) Scrutiny.pin(file, 0);
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

// PR search (ctrl+p, and ctrl+k on the Dashboard): jump to a PR from the
// Dashboard's lists or those recently opened, or one named by URL,
// owner/repo#123, or #123 in the repo of the PR being viewed.
(() => {
  "use strict";
  const dialog = document.getElementById("pr-search");
  if (!dialog) return;
  const input = dialog.querySelector("input");
  const list = dialog.querySelector("ul");
  const status = dialog.querySelector(".palette-status");
  const here = location.pathname.match(/^\/github\.com\/([\w.-]+)\/([\w.-]+)\/pull\/(\d+)/);
  let prs = [];
  let shown = [];
  let selected = 0;

  // named turns a pasted GitHub PR URL, owner/repo#123, or #123 (on a PR's
  // pages) into its path here.
  function named(q) {
    let m =
      q.match(/^https?:\/\/github\.com\/([\w.-]+)\/([\w.-]+)\/pull\/(\d+)/) || q.match(/^([\w.-]+)\/([\w.-]+)#(\d+)$/);
    if (!m && here && (m = q.match(/^#?(\d+)$/))) m = [q, here[1], here[2], m[1]];
    if (!m) return null;
    const ref = `${m[1]}/${m[2]}#${m[3]}`;
    return { title: `Open ${ref}`, ref: "", href: `/github.com/${m[1]}/${m[2]}/pull/${m[3]}`, key: `github.com/${ref}` };
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
    const needle = q.toLowerCase().replace(/\s+/g, "");
    const matches = prs
      .map((p) => ({ p, score: Math.max(fuzzy(needle, p.title), fuzzy(needle, p.ref)) }))
      .filter((x) => x.score >= 0)
      .sort((a, b) => b.score - a.score)
      .map((x) => x.p);
    // A named PR already listed shows as itself, first.
    const target = named(q);
    const listed = target && matches.findIndex((p) => p.ref === target.key);
    if (listed >= 0) matches.unshift(...matches.splice(listed, 1));
    shown = [...(target && listed < 0 ? [target] : []), ...matches];
    selected = 0;
    list.replaceChildren(
      ...shown.map((p, i) => {
        const a = Object.assign(document.createElement("a"), { className: "result" + (i ? "" : " selected"), href: p.href });
        a.append(Object.assign(document.createElement("span"), { className: "name", textContent: p.title }));
        if (p.draft) a.append(Object.assign(document.createElement("span"), { className: "badge", textContent: "Draft" }));
        const where = p.author ? `${p.ref} by ${p.author}` : p.ref;
        if (where) a.append(Object.assign(document.createElement("span"), { className: "where muted", textContent: where }));
        const li = document.createElement("li");
        li.append(a);
        return li;
      }),
    );
    if (shown.length === 0) list.replaceChildren(Object.assign(document.createElement("li"), { className: "muted", textContent: "No matching pull requests." }));
  }

  async function get(url) {
    const res = await fetch(url);
    if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
    return res.json();
  }

  // load shows the cached PRs at once, then, when they are over ten minutes
  // old (as on the Dashboard), fetches them from GitHub and shows those.
  let refreshing = null;
  async function load() {
    try {
      const cached = await get(dialog.dataset.src);
      prs = cached.prs;
      if (dialog.open) render();
      if (!cached.stale) return;
      status.hidden = false;
      refreshing ??= get(`${dialog.dataset.src}?refresh=1`).finally(() => (refreshing = null));
      prs = (await refreshing).prs;
      if (dialog.open) render();
    } catch (err) {
      list.replaceChildren(Object.assign(document.createElement("li"), { className: "muted", textContent: err.message }));
    } finally {
      status.hidden = true;
    }
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

  Scrutiny.actions.pr_search = () => {
    if (!dialog.open) dialog.showModal();
    input.select();
    render();
    load();
  };
  // The Dashboard has no code to search, so ctrl+k searches PRs there too.
  if (document.getElementById("dashboard-lists")) Scrutiny.actions.symbol_search = Scrutiny.actions.pr_search;
})();

// Hotkeys and the Command Palette.
(() => {
  "use strict";
  const body = document.body;
  let bindings = JSON.parse(body.dataset.keys || "{}");
  const labels = {
    symbol_search: "Search files and symbols",
    pr_search: "Jump to a pull request",
    command_palette: "Command palette",
    next_comment: "Next comment",
    prev_comment: "Previous comment",
    current_comment: "Go to the selected comment",
    toggle_view_mode: "Toggle split/unified diff",
    refresh: "Refresh",
    help: "Show keyboard shortcuts",
    back_to_pr: "Back to the pull request",
  };
  // An action that only stands in for another (ctrl+k searching PRs on the
  // Dashboard) is not listed twice.
  const alias = (a) => a !== "pr_search" && Scrutiny.actions[a] === Scrutiny.actions.pr_search;
  if (document.getElementById("dashboard-lists")) {
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
    // Escape closes what is open first; only then may it run a binding.
    if (e.key === "Escape") {
      const open = document.querySelectorAll(".context-menu, #side-panel");
      if (open.length || document.querySelector("dialog[open]")) {
        open.forEach((el) => el.remove());
        return;
      }
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

  // The file view links back to its PR; return there, to where it was left
  // when that is the page we came from.
  const back = document.getElementById("back-to-pr");
  if (back) {
    Scrutiny.actions.back_to_pr = () => {
      const from = document.referrer && new URL(document.referrer);
      if (from?.origin === location.origin && from.pathname === back.pathname && history.length > 1) history.back();
      else location.href = back.href;
    };
  }

  // Keyboard shortcuts help, built from the live bindings so it is right
  // straight after a rebind. Only pages that include the dialog offer it.
  const help = document.getElementById("help");
  if (help) {
    Scrutiny.actions.help = () => {
      const kbd = (key) => Object.assign(document.createElement("kbd"), { textContent: key });
      help.querySelector("tbody").replaceChildren(
        ...Object.keys(labels)
          .filter((a) => bindings[a] && Scrutiny.actions[a] && !alias(a))
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
        .filter((a) => a !== "command_palette" && Scrutiny.actions[a] && !alias(a))
        .map((a) => ({ label: labels[a], hint: bindings[a], run: Scrutiny.actions[a] })),
      ...link("since-last-review", "Show changes since your last review"),
      ...link("stack-parent", "Go to the parent PR in the Stack"),
      ...link("stack-child", "Go to the child PR in the Stack"),
      {
        label: "Change theme…",
        run: () =>
          show([
            ...themes.map((t) => ({ label: t.name, run: () => setTheme(t.id) })),
            { label: "Create a new theme…", run: () => Scrutiny.editTheme("/settings/themes/new") },
          ]),
      },
      ...(themes.find((t) => t.id === body.dataset.theme)?.custom
        ? [{ label: "Edit this theme…", run: () => Scrutiny.editTheme(`/settings/themes/${body.dataset.theme}/edit`) }]
        : []),
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

// The tagline after the brand types a phrase, now and then hitting a
// neighbouring key and correcting it, pauses, backspaces it, then types the
// next. With reduced motion it shows one phrase and stays still.
(() => {
  "use strict";
  const root = document.querySelector(".tagline");
  if (!root) return;
  const text = root.querySelector(".tagline-text");
  let phrases;
  try {
    phrases = JSON.parse(root.dataset.phrases || "[]");
  } catch {
    phrases = [];
  }
  if (!phrases.length) return;
  let index = Math.floor(Math.random() * phrases.length);

  if (matchMedia("(prefers-reduced-motion: reduce)").matches) {
    text.textContent = phrases[index];
    return;
  }

  const neighbours = {
    a: "qwsz", b: "vghn", c: "xdfv", d: "serfcx", e: "wsdr", f: "drtgvc", g: "ftyhbv",
    h: "gyujnb", i: "ujko", j: "huikmn", k: "jiolm", l: "kop", m: "njk", n: "bhjm",
    o: "iklp", p: "ol", q: "wa", r: "edft", s: "awedxz", t: "rfgy", u: "yhji",
    v: "cfgb", w: "qase", x: "zsdc", y: "tghu", z: "asx",
  };
  const typoChance = 0.04;
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const between = (min, max) => min + Math.random() * (max - min);

  function typo(char) {
    const keys = neighbours[char.toLowerCase()];
    if (!keys) return null;
    const pick = keys[Math.floor(Math.random() * keys.length)];
    return char === char.toLowerCase() ? pick : pick.toUpperCase();
  }
  function type(char) {
    const span = document.createElement("span");
    span.textContent = char;
    text.append(span);
  }
  const backspace = () => text.lastChild?.remove();

  async function run() {
    for (;;) {
      const chars = Array.from(phrases[index]);
      root.classList.add("typing");
      for (let i = 0; i < chars.length; i++) {
        const wrong = i > 0 && Math.random() < typoChance ? typo(chars[i]) : null;
        if (wrong) {
          await sleep(between(50, 110));
          type(wrong);
          // Sometimes the next key lands too before the slip is noticed.
          const extra = chars[i + 1]?.trim() && Math.random() < 0.4;
          if (extra) {
            await sleep(between(50, 110));
            type(chars[i + 1]);
          }
          await sleep(between(260, 580));
          for (let n = extra ? 2 : 1; n > 0; n--) {
            backspace();
            await sleep(60);
          }
          await sleep(between(90, 230));
        }
        await sleep(between(50, 110));
        type(chars[i]);
      }
      root.classList.remove("typing");
      await sleep(2500);
      root.classList.add("typing");
      while (text.lastChild) {
        backspace();
        await sleep(35);
      }
      root.classList.remove("typing");
      await sleep(600);
      index = (index + 1) % phrases.length;
    }
  }
  run();
})();

// Review Threads start from a line's + button: click it for one line, or
// drag from it to another line's + for the lines between, as on GitHub. A
// range stays within its hunk and, in a split diff, on its side; its lines
// stay highlighted while the form is open.
(() => {
  "use strict";
  let drag = null; // { start, end, reach }
  let marked = [];

  const isBreak = (tr) => tr.classList.contains("hunk") || tr.classList.contains("expander");
  const isSplit = (b) => b.closest("table").classList.contains("split");

  // The + buttons a range from start can reach, top to bottom.
  function reachable(start) {
    const split = isSplit(start);
    let first = start.closest("tr");
    while (first.previousElementSibling && !isBreak(first.previousElementSibling)) first = first.previousElementSibling;
    const out = [];
    for (let tr = first; tr && !isBreak(tr); tr = tr.nextElementSibling) {
      for (const b of tr.querySelectorAll(".add-comment")) {
        if (!split || b.dataset.side === start.dataset.side) out.push(b);
      }
    }
    return out;
  }

  const anchor = (b) => ({ side: b.dataset.side, line: Number(b.dataset.line) });

  // span anchors the range from first to last (top to bottom), or is null
  // when GitHub could not take it: across sides it must start on the left,
  // so one from a context line onto deleted lines starts at its old number.
  function span(first, last) {
    let start = anchor(first);
    const end = anchor(last);
    if (start.side === "RIGHT" && end.side === "LEFT") {
      if (!first.dataset.oldLine) return null;
      start = { side: "LEFT", line: Number(first.dataset.oldLine) };
    }
    if (start.side === end.side && start.line >= end.line) return null;
    return { start, end };
  }

  function ordered(a, b) {
    return a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_PRECEDING ? [b, a] : [a, b];
  }

  function clear() {
    for (const td of marked) td.classList.remove("in-range");
    marked = [];
  }
  function mark(first, last) {
    clear();
    const split = isSplit(first);
    const side = first.dataset.side;
    const end = last.closest("tr");
    for (let tr = first.closest("tr"); tr; tr = tr.nextElementSibling) {
      if (!tr.classList.contains("thread-row")) {
        const cells = [...tr.children];
        marked.push(...(split ? (side === "LEFT" ? cells.slice(0, 2) : cells.slice(2)) : cells));
      }
      if (tr === end) break;
    }
    for (const td of marked) td.classList.add("in-range");
  }

  // open loads the new-thread form below last's row, for range if given.
  function open(last, range) {
    const row = last.closest("tr");
    const existing = row.nextElementSibling;
    if (existing?.classList.contains("new-thread")) {
      if (existing.querySelector("textarea")?.value) return existing.querySelector("textarea").focus();
      existing.remove();
    }
    const end = range?.end ?? anchor(last);
    let url = `${last.closest("table").dataset.newThread}&side=${end.side}&line=${end.line}`;
    if (range) url += `&start_side=${range.start.side}&start_line=${range.start.line}`;
    htmx.ajax("GET", url, { source: row, target: row, swap: "afterend" });
  }

  // Mouse and touch open the form on release, a click being a drag that
  // never left its line; nothing hangs on the click event that follows,
  // whose timing browsers differ on. Keyboard activation is a click with
  // no pointer behind it (detail 0).
  function stop() {
    drag = null;
    document.documentElement.classList.remove("dragging-range");
  }
  document.addEventListener("pointerdown", (e) => {
    const b = e.target.closest(".add-comment");
    if (!b || e.button !== 0) return;
    e.preventDefault(); // no text selection or focus change while dragging
    document.documentElement.classList.add("dragging-range");
    drag = { start: b, end: b, reach: reachable(b) };
  });
  document.addEventListener("pointermove", (e) => {
    if (!drag) return;
    const tr = document.elementFromPoint(e.clientX, e.clientY)?.closest("tr");
    const b = drag.reach.find((x) => x.closest("tr") === tr);
    if (!b || b === drag.end) return;
    const [first, last] = ordered(drag.start, b);
    if (first !== last && !span(first, last)) return;
    drag.end = b;
    if (first === last) clear();
    else mark(first, last);
  });
  addEventListener("pointerup", () => {
    if (!drag) return;
    const { start, end } = drag;
    stop();
    if (start === end) {
      clear();
      open(start, null);
      return;
    }
    const [first, last] = ordered(start, end);
    open(last, span(first, last));
  });
  addEventListener("pointercancel", () => {
    if (!drag) return;
    stop();
    clear();
  });

  document.addEventListener("click", (e) => {
    const b = e.target.closest(".add-comment");
    if (b && e.detail === 0) {
      clear();
      open(b, null);
    }
    if (e.target.closest(".new-thread [data-remove-row]")) clear();
  });
  // A posted thread replaces its form.
  document.addEventListener("htmx:afterRequest", (e) => {
    if (e.detail.successful && e.detail.elt.closest?.(".new-thread")) clear();
  });
})();

// Comparison commit pickers: a button and a listbox standing in for a
// select, so options can show a title, hash and time on two lines.
(() => {
  "use strict";
  const pickers = document.querySelectorAll(".commit-picker");
  if (pickers.length === 0) return;
  function close(picker, focusButton) {
    picker.querySelector(".commit-picker-list").hidden = true;
    picker.querySelector(".commit-picker-button").setAttribute("aria-expanded", "false");
    if (focusButton) picker.querySelector(".commit-picker-button").focus();
  }
  function open(picker) {
    pickers.forEach((p) => p !== picker && close(p));
    const list = picker.querySelector(".commit-picker-list");
    list.hidden = false;
    picker.querySelector(".commit-picker-button").setAttribute("aria-expanded", "true");
    const current = list.querySelector('[aria-selected="true"]') || list.firstElementChild;
    activate(list, current);
  }
  function activate(list, item) {
    if (!item) return;
    list.querySelectorAll(".active").forEach((i) => i.classList.remove("active"));
    item.classList.add("active");
    item.focus();
    item.scrollIntoView({ block: "nearest" });
  }
  function choose(picker, item) {
    picker.querySelector("input[type=hidden]").value = item.dataset.value;
    picker.querySelectorAll('[role="option"]').forEach((o) => o.setAttribute("aria-selected", String(o === item)));
    picker.querySelector(".commit-picker-button").innerHTML = item.innerHTML;
    close(picker, true);
  }
  pickers.forEach((picker) => {
    const button = picker.querySelector(".commit-picker-button");
    const list = picker.querySelector(".commit-picker-list");
    button.addEventListener("click", () => (list.hidden ? open(picker) : close(picker)));
    button.addEventListener("keydown", (e) => {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        open(picker);
      }
    });
    list.addEventListener("click", (e) => {
      const item = e.target.closest('[role="option"]');
      if (item) choose(picker, item);
    });
    list.addEventListener("keydown", (e) => {
      const active = list.querySelector(".active");
      if (e.key === "ArrowDown") activate(list, active?.nextElementSibling);
      else if (e.key === "ArrowUp") activate(list, active?.previousElementSibling);
      else if (e.key === "Enter" || e.key === " ") active && choose(picker, active);
      else if (e.key === "Tab") return close(picker); // let focus move on
      else if (e.key === "Escape") close(picker, true);
      else return;
      e.preventDefault();
      e.stopPropagation();
    });
  });
  document.addEventListener("click", (e) => pickers.forEach((p) => !p.contains(e.target) && close(p)));
})();

// Comment boxes: Write and Preview tabs and a Markdown formatting toolbar,
// as on GitHub. Edits go through the browser's insertText so ctrl+z undoes
// them; the preview is rendered by GitHub, so it matches the posted comment.
(() => {
  "use strict";

  // replace swaps box's text from start to end for text, then selects
  // selStart..selEnd (offsets into the whole new value).
  function replace(box, start, end, text, selStart, selEnd) {
    box.focus();
    box.setSelectionRange(start, end);
    if (!document.execCommand("insertText", false, text)) box.setRangeText(text, start, end, "end");
    box.setSelectionRange(selStart, selEnd ?? selStart);
    box.dispatchEvent(new Event("input", { bubbles: true }));
  }

  // wrap surrounds the selection with before and after, or removes them
  // when it is already wrapped; with nothing selected the cursor lands
  // between them, ready to type.
  function wrap(box, before, after) {
    const { value, selectionStart: s, selectionEnd: e } = box;
    const sel = value.slice(s, e);
    if (value.slice(s - before.length, s) === before && value.slice(e, e + after.length) === after) {
      return replace(box, s - before.length, e + after.length, sel, s - before.length, e - before.length);
    }
    if (sel.startsWith(before) && sel.endsWith(after) && sel.length >= before.length + after.length) {
      const inner = sel.slice(before.length, sel.length - after.length);
      return replace(box, s, e, inner, s, s + inner.length);
    }
    replace(box, s, e, before + sel + after, s + before.length, s + before.length + sel.length);
  }

  // lines extends the selection to whole lines, returning its bounds.
  function lines(box) {
    const { value, selectionStart: s, selectionEnd: e } = box;
    const start = value.lastIndexOf("\n", s - 1) + 1;
    let end = value.indexOf("\n", e > s && value[e - 1] === "\n" ? e - 1 : e);
    if (end < 0) end = value.length;
    return { start, end, text: value.slice(start, end).split("\n") };
  }

  // prefix starts each selected line with make(i), or strips it from all
  // of them when every one already has it (matching pattern). A list
  // marker already there is swapped for the new one when list is set.
  function prefix(box, pattern, make, list = false) {
    const { start, end, text } = lines(box);
    const all = text.every((l) => pattern.test(l));
    const out = text
      .map((l, i) => {
        if (!list) return all ? l.replace(pattern, "") : make(i) + l;
        const [, indent, rest] = l.match(listItem);
        return indent + (all ? "" : make(i)) + rest;
      })
      .join("\n");
    if (text.length === 1 && box.selectionStart === box.selectionEnd) {
      const caret = box.selectionStart + out.length - text[0].length;
      return replace(box, start, end, out, Math.max(start, caret));
    }
    replace(box, start, end, out, start, start + out.length);
  }
  // listItem splits a line into its indent and its text after any marker.
  const listItem = /^(\s*)(?:[-*+]\s\[[ xX]\]\s|[-*+]\s|\d+[.)]\s)?(.*)$/;

  function link(box) {
    const { value, selectionStart: s, selectionEnd: e } = box;
    const sel = value.slice(s, e);
    if (/^https?:\/\/\S+$/.test(sel)) return replace(box, s, e, `[](${sel})`, s + 1);
    const text = `[${sel}](url)`;
    const url = s + sel.length + 3;
    replace(box, s, e, text, url, url + 3);
  }

  // suggest adds a suggested change holding the lines it replaces, read
  // from the diff; lines not on the page leave the block empty.
  function suggest(box, button) {
    const start = Number(button.dataset.start);
    const end = Number(button.dataset.end);
    const table = box.closest("table.diff");
    const code = [];
    for (let n = start; n <= end && table; n++) {
      const cell = table.querySelector(`td.code[data-line="${n}"]`);
      if (!cell) {
        code.length = 0;
        break;
      }
      code.push(cell.textContent.replace(/\n$/, ""));
    }
    const { value, selectionStart: s, selectionEnd: e } = box;
    const lead = s === 0 || value[s - 1] === "\n" ? "" : "\n";
    const body = code.join("\n");
    const text = `${lead}\`\`\`suggestion\n${body}\n\`\`\`\n`;
    const caret = s + lead.length + "```suggestion\n".length;
    replace(box, s, e, text, caret, caret + body.length);
  }

  const actions = {
    heading: (box) => prefix(box, /^#{1,6}\s/, () => "### "),
    bold: (box) => wrap(box, "**", "**"),
    italic: (box) => wrap(box, "_", "_"),
    underline: (box) => wrap(box, "<ins>", "</ins>"),
    strike: (box) => wrap(box, "~~", "~~"),
    quote: (box) => prefix(box, /^>\s?/, () => "> "),
    code: (box) => {
      const { value, selectionStart: s, selectionEnd: e } = box;
      if (value.slice(s, e).includes("\n")) wrap(box, "```\n", "\n```");
      else wrap(box, "`", "`");
    },
    link,
    ordered: (box) => prefix(box, /^\s*\d+[.)]\s/, (i) => `${i + 1}. `, true),
    unordered: (box) => prefix(box, /^\s*[-*+]\s(?!\[[ xX]\]\s)/, () => "- ", true),
    task: (box) => prefix(box, /^\s*[-*+]\s\[[ xX]\]\s/, () => "- [ ] ", true),
  };

  const editorOf = (el) => el.closest(".md-editor");
  const boxOf = (editor) => editor.querySelector("textarea");

  async function show(editor, tab) {
    const box = boxOf(editor);
    const pane = editor.querySelector(".md-preview");
    const previewing = tab === "preview";
    for (const t of editor.querySelectorAll("[data-md-tab]")) t.setAttribute("aria-selected", String(t.dataset.mdTab === tab));
    editor.classList.toggle("previewing", previewing);
    if (!previewing) {
      pane.hidden = true;
      box.hidden = false;
      box.focus();
      return;
    }
    pane.style.minHeight = box.offsetHeight + "px";
    box.hidden = true;
    pane.hidden = false;
    pane.innerHTML = '<p class="muted">Loading preview…</p>';
    const body = box.value;
    try {
      const res = await Scrutiny.busy(
        fetch(editor.dataset.preview, {
          method: "POST",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded",
            "HX-Request": "true",
            "X-CSRF-Token": document.querySelector('meta[name="csrf-token"]').content,
          },
          body: new URLSearchParams({ body }),
        }),
      );
      const text = await res.text();
      if (box.value !== body || !editor.classList.contains("previewing")) return; // superseded
      if (res.status === 401) Scrutiny.authFailed();
      if (res.ok) pane.innerHTML = text;
      else pane.replaceChildren(Object.assign(document.createElement("p"), { className: "muted", textContent: `Preview failed: ${text.trim() || res.statusText}` }));
    } catch {
      pane.innerHTML = '<p class="muted">Preview failed: Scrutiny is not reachable.</p>';
    }
  }

  // Toolbar buttons keep the textarea's focus and selection.
  document.addEventListener("mousedown", (e) => {
    if (e.target.closest(".md-toolbar button")) e.preventDefault();
  });
  document.addEventListener("click", (e) => {
    const tab = e.target.closest("[data-md-tab]");
    if (tab) return show(editorOf(tab), tab.dataset.mdTab);
    const button = e.target.closest(".md-toolbar [data-md]");
    if (!button) return;
    const editor = editorOf(button);
    if (editor.classList.contains("previewing")) show(editor, "write");
    const box = boxOf(editor);
    if (button.dataset.md === "suggestion") suggest(box, button);
    else actions[button.dataset.md]?.(box);
  });

  // Back to Write when the form resets (Cancel) or the browser needs to
  // show the textarea to say it is required.
  document.addEventListener("reset", (e) => {
    for (const editor of e.target.querySelectorAll(".md-editor.previewing")) show(editor, "write");
  });
  document.addEventListener(
    "invalid",
    (e) => {
      const editor = e.target.closest?.(".md-editor.previewing");
      if (editor) show(editor, "write");
    },
    true,
  );

  const shortcuts = { b: "bold", i: "italic", e: "code" };
  // Captured ahead of the hotkeys, which are not meant for comment text.
  document.addEventListener(
    "keydown",
    (e) => {
      const box = e.target;
      if (!box.matches?.(".md-editor textarea")) return;
      const mod = (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey;
      if (mod && shortcuts[e.key.toLowerCase()]) {
        e.preventDefault();
        e.stopPropagation();
        actions[shortcuts[e.key.toLowerCase()]](box);
        return;
      }
      if (e.key === "Enter" && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey && !e.isComposing) continueList(e, box);
    },
    true,
  );

  // Enter in a list item starts the next one; on an empty item it ends
  // the list instead.
  function continueList(e, box) {
    const { value, selectionStart: s, selectionEnd: end } = box;
    if (s !== end) return;
    const start = value.lastIndexOf("\n", s - 1) + 1;
    const line = value.slice(start, s);
    const m = line.match(/^(\s*)([-*+]\s\[[ xX]\]\s|[-*+]\s|(\d+)([.)])\s)/);
    if (!m) return;
    e.preventDefault();
    if (line === m[0]) return replace(box, start, s, "", start);
    let marker = m[2];
    if (m[3]) marker = `${Number(m[3]) + 1}${m[4]} `;
    else if (/\[[ xX]\]/.test(marker)) marker = marker.replace(/\[[xX]\]/, "[ ]");
    const text = "\n" + m[1] + marker;
    replace(box, s, s, text, s + text.length);
  }

  // Pasting a URL over selected text links it.
  document.addEventListener("paste", (e) => {
    const box = e.target;
    if (!box.matches?.(".md-editor textarea") || box.selectionStart === box.selectionEnd) return;
    const url = e.clipboardData?.getData("text/plain").trim();
    if (!url || !/^https?:\/\/\S+$/.test(url)) return;
    const { value, selectionStart: s, selectionEnd: end } = box;
    const sel = value.slice(s, end);
    if (/^https?:\/\//.test(sel) || sel.includes("\n")) return;
    e.preventDefault();
    const text = `[${sel}](${url})`;
    replace(box, s, end, text, s + text.length);
  });
})();

// The Theme Editor loads into its dialog, which opens once it has. While its
// colour pickers are showing, the page previews their colours.
(() => {
  "use strict";
  // Looked up each time, so a swap that replaced the dialog cannot leave a
  // stale one behind.
  const editor = () => document.getElementById("theme-editor");
  Scrutiny.editTheme = (url) => htmx.ajax("GET", url, { target: "#theme-editor", swap: "innerHTML" });

  function rgba(hex, alpha) {
    if (!(alpha < 1)) return hex;
    const n = parseInt(hex.slice(1), 16);
    return `rgba(${n >> 16 & 255}, ${n >> 8 & 255}, ${n & 255}, ${alpha})`;
  }
  function preview() {
    const dialog = editor();
    const form = dialog?.querySelector("form");
    let style = document.getElementById("theme-preview");
    if (!dialog?.open || form?.elements.mode?.value !== "pick") return style?.remove();
    const ui = [], syntax = [];
    for (const input of form.querySelectorAll("[data-key]")) {
      const v = input.type === "color" ? rgba(input.value, parseFloat(input.dataset.alpha)) : input.value;
      if (input.dataset.section === "ui") ui.push(`--${input.dataset.key}: ${v};`);
      else syntax.push(`.hl-${input.dataset.key.replaceAll(".", "-")} { color: ${v}; }`);
    }
    if (!style) {
      style = document.createElement("style");
      style.id = "theme-preview";
      document.head.append(style);
    }
    style.textContent = `:root { color-scheme: ${form.elements.scheme.value}; ${ui.join(" ")} }\n${syntax.join("\n")}`;
  }
  document.addEventListener("htmx:afterSwap", (e) => {
    const dialog = editor();
    if (!dialog || e.detail.target !== dialog) return;
    if (!dialog.open) dialog.showModal();
    preview();
  });
  const inEditor = (e) => e.target.closest?.("#theme-editor") && preview();
  document.addEventListener("input", inEditor);
  document.addEventListener("change", inEditor);
  // close does not bubble, so it is caught on the way down.
  document.addEventListener("close", inEditor, true);
})();
