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

  // Next/previous comment. Files load lazily, so any file with threads that
  // has not loaded yet is loaded first.
  async function loadThreadFiles(includeResolved) {
    const attr = includeResolved ? "threads" : "unresolved";
    const pending = [...document.querySelectorAll(".file")]
      .filter((f) => Number(f.dataset[attr] || 0) > 0)
      .map((f) => f.querySelector(".lazy-diff"))
      .filter(Boolean)
      .map((el) => htmx.ajax("GET", el.getAttribute("hx-get"), { target: el, swap: "outerHTML" }));
    await Promise.all(pending);
  }

  async function navigate(direction) {
    const includeResolved = document.getElementById("nav-include-resolved")?.checked;
    await loadThreadFiles(includeResolved);
    const threads = [...document.querySelectorAll(".thread")].filter(
      (t) => includeResolved || t.dataset.resolved === "false",
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
