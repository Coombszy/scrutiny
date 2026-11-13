// Sends a tab opened with a Goto Ticket on to its PR page, replacing this
// page in the tab's history. See internal/web/ticket.go.
location.replace(document.currentScript.dataset.to);
