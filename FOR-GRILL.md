# Project idea

I want to make a basic Go CLI tool that I run locally. This CLI can run in the background and expose a webpage that can be accessed on local host. The purpose of this tool is to make a better git diff tool for GitHub PRs. There are some key features i'd like to have.

- Treesitter support, 
  - allow control clicking symbols,functions, vars whatever to be able to jump to definitions. Also accessible by right clicking.
  - allow right clicking to find usages
  - color scheme
- Allow normal git diff versus base branch, but also see diff between commits in the PR history. E.g changes since last 2 commits.
- Toggle between side-by-side and unified git diff.
- Fast, using Go, Templ, and HTMX. Use sqlite for a local state + cache, and will likely need a XDG_HOME/'.local/state/' directory for pulling repos to for interrogation locally.
- Extendable, utilise hexagonal architecture with internal packages to allow extending and testing individual components.
- Basic themes, by default we will want to use Gruvbox
- Using hexagonal architecture, we want the first version to interface with GitHub (Preferably using the gh cli, especially if it has an SDK?)
  - Leave comments, and add comments to comment threads and emojis etc
  - Allow approval, reject and comment with the dialog box, just like the GitHub UI
  - See GitHub actions in progress/blocking merging
- Hotkey support
  - ctrl + k for fuzzy search symbols, vars, methods and file names. To work a bit like the global search in IntelliJ IDEA or Telescope in Neovim.
  - ctrl + j for setting/feature search. E.g toggle side-by-side/unified view, refresh changes, change theme.
- Useful UI buttons and state
  - Next/Previous review comment
  - Next(Child)/Previous(Parent) PR stack
  - When a PR changes, a little "Refresh" button like in GitHub.
  
