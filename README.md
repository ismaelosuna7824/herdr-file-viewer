# File Viewer — a Herdr plugin

[![build](https://github.com/ismaelosuna7824/herdr-file-viewer/actions/workflows/build.yml/badge.svg)](https://github.com/ismaelosuna7824/herdr-file-viewer/actions/workflows/build.yml)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
![herdr-plugin](https://img.shields.io/badge/herdr--plugin-✓-8b5cf6)

A fast, keyboard-driven **file explorer, code viewer and full git client** in a
single [Herdr](https://herdr.dev) pane — written in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea). Think a tiny mash-up of
a VS Code file tree, a fuzzy finder, ripgrep-style search, and lazygit — all in
the terminal.

## Preview

The four-panel browse screen — file tree + code view on top, the search panel
and the git panel docked below:

![File Viewer running in a Herdr pane](docs/screenshots/browse.png)

Plain-text renders of other views live in
[`docs/screenshots/`](docs/screenshots).

## Features

Three things in one pane:

- **File browser** — a navigable, lazily-expanded directory tree with **git
  status decorations**: modified, new, deleted and renamed files are colored and
  badged, and directories containing changes are tinted (VS Code style).
- **Rendered markdown** — `.md` files display as formatted documents (headings,
  lists, code blocks) via glamour; press `m` to toggle to raw source.
- **Persistent search panel** — always docked bottom-left. `Ctrl+P` focuses it in
  fuzzy file-find mode, `Ctrl+F` in content-search mode (with case-sensitive
  `Aa`, whole-word `ab` and regex `.*` toggles). Arrow keys preview each result
  live in the file view; `Enter` opens it.
- **Persistent git panel** — always docked bottom-right, titled with the current
  branch. Focus it with `g` or `Tab`; press `g` again to toggle between the
  **commit history** (Enter opens a commit's full multi-file diff) and the
  **branch list** (Enter switches to the selected branch).
- **Review / diff view** (`d`) — see a file's changes against `HEAD`. Modified
  files render **side-by-side** (old on the left, new on the right, changed lines
  aligned) so you can see exactly what changed; brand-new files render inline as
  all-additions. Press `s` to toggle the layout; narrow panes fall back to
  inline automatically.

The search and fuzzy-find engines are **pure Go standard library** — no
`ripgrep`, no `fzf`. The only external tool is `git`, used solely for the status
decorations; without it (or outside a repo) the viewer still works, just without
decorations.

## Layout

The browse screen is a fixed four-panel workspace:

```
┌───────────────┬─────────────────────────────┐
│ file tree     │ file view (syntax / markdown)│
│               │                              │
├───────────────┴──────────────┬──────────────┤
│ ── FIND FILE / SEARCH ──      │ ── GIT LOG · branch ──
│ (fuzzy find / content search) │ (commit history)      │
└───────────────────────────────┴──────────────┘
```

`Tab` cycles focus through the four panels: tree → file view → search → git log.
`Alt+h/j/k/l` (or `Alt+arrows`) moves focus by direction. The search and
git-log panels are always visible.

The git panel cycles through three views with `g`: **staging** (changed files),
**branches**, and **commit history**.

### Staging view (changes)

A lazygit-style **directory tree** of changed files, with common path chains
collapsed. Each row has a checkbox: `[✓]` staged (green), `[~]` partially staged,
`[ ]` unstaged. Directory rows aggregate their subtree.

| Key | Action |
|-----|--------|
| `space` | Stage / unstage the selected file — or a whole directory subtree |
| `A` / `U` | Stage all / unstage all |
| `c` | Commit the staged changes (prompts for a message) |
| `Enter` | View the selected file's diff |

### Branch view

From the branch view you can drive the whole git flow:

| Key | Action | |
|-----|--------|--|
| `Enter` | Switch to the selected branch | |
| `n` | Create a new branch (from the current one) | prompt |
| `c` | Commit staged changes (stage first with `space`/`A`) | prompt |
| `a` | Amend the last commit with current changes | confirm |
| `u` | Undo the last commit (keeps changes staged) | confirm |
| `m` | Merge the selected branch into the current one | confirm |
| `r` | Rebase the current branch onto the selected one | confirm |
| `x` | Delete the selected branch | confirm |
| `A` | Stage all (`git add -A`) | |
| `t` | Create a tag at HEAD | prompt |
| `f` | Fetch all remotes (prune) | |
| `p` | Pull | |
| `P` | Push | |
| `F` | Force-push (`--force-with-lease`) | confirm |
| `s` / `S` | Stash / stash pop | |
| `H` | Reset `--hard HEAD` (discard all changes) | confirm |

In the **history** view: `y` cherry-picks the selected commit onto the current
branch (confirm), and `R` resets `--hard` the current branch to the selected
commit (confirm).

Anything that rewrites history, deletes, discards, or force-pushes asks for
confirmation first. Any git error (conflicts, refusals, no upstream) is shown in
red inside the panel — nothing silently fails.

## Keys

### Browser

| Key | Action |
|-----|--------|
| `↑`/`k`, `↓`/`j` | Move the cursor |
| `→`/`l`, `←`/`h` | Expand / collapse a directory |
| `Enter` / `Space` | Open a file or toggle a directory |
| `Tab` | Cycle focus through the four panels |
| `Alt+h/j/k/l`, `Alt+arrows` | Move focus by direction |
| `Ctrl+P` | Focus the search panel in file-find mode |
| `Ctrl+F` | Focus the search panel in content-search mode |
| `L` | Locate the open file in the tree (reveal + focus it) |
| `o` | Open the file's location in the OS file manager |
| `d` | Review the selected/open file's diff against `HEAD` |
| `g` | Focus the git-log panel (toggle back to the tree) |
| `m` | Toggle rendered markdown ↔ source (markdown files only) |
| `r` | Refresh git status and the file index (e.g. after a commit) |
| `q` / `Ctrl+C` | Quit |

### Review / diff view (`d`)

Scroll with `↑`/`↓` / `PgUp` / `PgDn`; `s` toggles split ↔ inline; `d`, `Esc`
or `q` return to the browser.

### Git status legend

| Badge | Color | Meaning |
|-------|-------|---------|
| `M` | amber | Modified |
| `U` | green | Untracked (new) |
| `A` | green | Added (staged) |
| `D` | red | Deleted |
| `R` | blue | Renamed |
| `!` | pink | Conflicted |

Decorations appear only inside a git repository; elsewhere the tree renders
plainly.

The search panel is always docked bottom-left. `Ctrl+P` / `Ctrl+F` focus it and
switch it between file-find and content-search; within it, `↑`/`↓` preview and
`Enter` opens. Press `?` any time for the full keybinding reference.

### Content search toggles

The status line always shows each toggle's on/off state. `Ctrl` shortcuts work
everywhere (a terminal can't send `Alt` from macOS's Option key); the `Alt`
aliases work on Linux/Windows.

| Key | Toggle |
|-----|--------|
| `Ctrl+t` (`Alt+c`) | Case sensitive (`Aa`) |
| `Ctrl+w` (`Alt+w`) | Whole word (`ab`) |
| `Ctrl+r` (`Alt+r`) | Regular expression (`.*`) |

## Install

```sh
herdr plugin install ismaelosuna7824/herdr-file-viewer
```

On install the `[[build]]` step **downloads a prebuilt binary** for your platform
(macOS/Linux, amd64/arm64) from the GitHub release — **no Go required**. If a
prebuilt binary isn't available it falls back to `go build` (needs Go 1.25+).

This repo is tagged with the `herdr-plugin` topic, so it also shows up in Herdr's
plugin marketplace (`/plugins/`).

### Local development

```sh
git clone https://github.com/ismaelosuna7824/herdr-file-viewer
herdr plugin link herdr-file-viewer
```

## Opening the viewer

The plugin ships with two default keybindings (declared in its manifest):

| Keys | Opens |
|------|-------|
| `<prefix> f` | The viewer in a **split** beside your work |
| `<prefix> Shift+f` | The viewer in **its own tab** |

(`<prefix>` is your Herdr prefix key.) You can also run it from the action menu:
**Open File Viewer** / **Open File Viewer (tab)**.

### Custom keybindings

To use different keys, add them to `~/.config/herdr/config.toml`, invoking the
plugin's actions (`open` = split, `open-tab` = tab):

```toml
[[keys.command]]              # open beside your work (split)
key = "prefix+f"
type = "shell"
command = "herdr plugin action invoke open --plugin ismaelosuna.file-viewer"

[[keys.command]]              # …or in its own tab
key = "prefix+shift+f"
type = "shell"
command = "herdr plugin action invoke open-tab --plugin ismaelosuna.file-viewer"
```

Swap `prefix+f` / `prefix+shift+f` for whatever keys you like. Reload Herdr (or
your config) for the changes to take effect. Once open, press `?` for the full
in-app keybinding reference.

## Updating

On startup the viewer checks GitHub for a newer release and shows a notice in the
header (e.g. `⬆ v0.1.4 available`) when one exists. To update, re-run the install
command — it pulls the latest and rebuilds/downloads the binary:

```sh
herdr plugin install ismaelosuna7824/herdr-file-viewer
```

## Develop

```sh
go build ./...      # compile everything
go test ./...       # run the engine + UI composition tests
go build -o bin/file-viewer ./cmd/file-viewer   # what the build step runs
```

You can also run the binary directly outside Herdr against any directory:

```sh
./bin/file-viewer /path/to/project
```

With no argument it uses the workspace directory from Herdr's context, falling
back to the current working directory.

## Project structure

```
cmd/file-viewer/   entrypoint — the binary Herdr launches in the pane
internal/
  explorer/        navigable directory-tree model
  finder/          fuzzy file-path matcher (right-to-left, basename-biased)
  search/          pure-Go content search engine + .gitignore matcher
  gitstatus/       git working-tree status (shells out to `git`)
  gitdiff/         parsed diff (file vs HEAD, or a whole commit) via `git`
  gitlog/          commit history + branch/git operations
  reveal/          open a path in the OS file manager (cross-platform)
  viewer/          read-only file content pane with line numbers
  ui/              Bubble Tea app that composes the above (Model-Update-View)
```

## Notes & limits

- The `.gitignore` support is pragmatic, not spec-complete: directory names,
  `*.ext` patterns and anchored paths are honoured; negation (`!`) and nested
  `.gitignore` files are not. A built-in list (`.git`, `node_modules`, `dist`,
  …) is always skipped.
- Content search caps at 5000 matches and skips binary and very large files to
  stay responsive; the status line flags a truncated result.
- Syntax highlighting runs asynchronously (files open instantly as plain text,
  colours arrive a beat later) so scrolling the tree never stutters.

## Built with

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling
- [Chroma](https://github.com/alecthomas/chroma) — syntax highlighting
- [Glamour](https://github.com/charmbracelet/glamour) — markdown rendering

## Contributing

Issues and PRs welcome. `go test ./...` and `gofmt -l .` must stay clean (CI
enforces both across Linux, macOS and Windows).

## License

[MIT](LICENSE) © ismaelosuna7824
