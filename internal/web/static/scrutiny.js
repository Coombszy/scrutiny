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
