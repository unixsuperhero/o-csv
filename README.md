<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-animated-dark.svg">
    <img alt="o-csv" src="assets/logo-animated-light.svg" width="360">
  </picture>
  <br>
  <a href="https://github.com/ofamtools/o-csv/tags"><img alt="Latest tag" src="https://img.shields.io/github/v/tag/ofamtools/o-csv"></a>
  <a href="https://pkg.go.dev/ofam.dev/o-csv"><img alt="Go Reference" src="https://pkg.go.dev/badge/ofam.dev/o-csv.svg"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/ofamtools/o-csv"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/ofamtools/o-csv"></a>
</p>

<p align="center">A CSV viewer built on ochazuke. Viewing comes first; editing is there but stays off until you ask for it.</p>

A CSV viewer for the o family, built on [ochazuke](https://github.com/ofamtools/ochazuke) (Bubble Tea v2) with an [oden](https://github.com/ofamtools/oden) command line. It started as a copy of ochazuke's `examples/csv` editor. Viewing comes first. Editing is still there, but it stays off until you ask for it.

## Install

Install with the Go version in [go.mod](go.mod) or newer:

```sh
go install ofam.dev/oden/cmd/o@latest   # oden's o: `o csv` runs o-csv, no dash needed
go install ofam.dev/o-csv@latest
mtm extension register csv   # optional: the table verbs in mtm and its MCP gateway
```

## Usage

```sh
o csv data.csv                  # open a file
some-command | o csv            # or read stdin (also: o csv -)
o csv --delim tab --no-header x.txt
o csv --edit data.csv           # start with editing on
o csv --smoke data.csv          # render one 80x24 frame to stdout and exit
```

## The viewer

The table fills the screen: a header row, then only the rows that fit on screen. That means a 100k-row file costs one screen per frame, and it loads off the event loop (`sheet.load` is pending work, and the screen says `loading …` while it runs). Each column is as wide as the widest of its first 2,000 values and of the rows drawn so far, capped at 32 cells (the `column-width` setting). Longer cells are cut with `…`. `z >` and `z <` widen or narrow the cursor's column by 2 cells, past the cap, and `z =` makes every column automatic again; the widths are remembered by column number in the state directory (`tables.json`), for every file. On a narrow screen the columns scroll with the cursor, and the next column shows partly while there is room for it. The status line above the table shows the file, `read-only` or `EDIT`, `row N/M` (with `(of TOTAL)` while a filter is on), `col N/M NAME`, and the active sort, filter, search, frozen column and hidden-column count. The delimiter is guessed from the first 64 KiB, trying comma, tab, semicolon and pipe. `--delim` (or the `delimiter` setting) overrides it. With `--no-header`, the columns are named A, B, C, …

### Keys

`?` lists every key the focus answers to, generated from the binding rows. The footer shows the keys that are not motions.

| Key | Does |
|---|---|
| `h j k l`, arrows | move (counts work: `5j`) |
| `0` `$` / `home` `end` | first / last column |
| `g g` / `G` | first / last row (`12G` goes to row 12) |
| `ctrl+d` `ctrl+u` | half a screen down / up |
| `ctrl+f` `ctrl+b` | a screen down / up |
| `\|` | column by number (`4\|`; plain `\|` is the first column) |
| `g c` | column by name: you type it, and an exact match wins, then a prefix, then a substring |
| `/` | search every shown cell (case-insensitive); matches are underlined |
| `n` / `N` | next / previous match, wrapping |
| `f` | filter rows by the cursor's column in the field below the table: `enter` applies it, `esc` or `tab` goes back |
| `esc` | clear the search and the filter |
| `s` | sort by the cursor's column: ascending, then descending, then back to file order. Numbers sort numerically and empty cells go last |
| `-` / `g =` | hide the cursor's column / show every column again |
| `F` | freeze the first shown column at the left (toggle) |
| `enter` | show or hide the row detail: every column of the cursor's row with its whole value, wrapped, beside the table (below it under 100 columns). It follows the cursor; the focus stays in the table |
| `J` / `K` | scroll the row detail while it is longer than its pane (counts work: `5J`) |
| `ctrl+w >` / `ctrl+w <` / `ctrl+w =` | while the detail shows: grow the table, shrink it, put the divider back (counts work: `3 ctrl+w >`). The divider is remembered in `~/.local/state/o-csv/panes.json` |
| `y` / `Y` | copy the cell / copy the row as one CSV line. The copy goes to the ochazuke registers (`"a y` names one) and to the system clipboard (pbcopy, wl-copy, xclip or xsel, whichever is on PATH). `.` yanks again at the cursor (`y j .` copies the next cell) |
| `ctrl+o` | open another file |
| `:` | the command palette (every verb, including those without a key) |
| `ctrl+c` | quit |

### Editing

Editing is off by default. Start with `--edit` or press `ctrl+e` to toggle it. Leaving edit mode with unsaved edits is refused. While editing is off, every edit verb is guarded: its key does nothing, the palette does not offer it, and the sheet refuses it underneath. Nothing in a read-only session writes a file (`TestReadOnlyNeverWrites`).

While editing is on, ochazuke's CSV example keys apply: `i` edit a cell, `x` clear, `p` paste, `d d` delete a row (it asks first), `o` insert a row, `r` rename a column, `D` delete a column, `+` add a column, `t u`/`t l`/`t t`/`t i`/`t s` transform (upper, lower, trim, Title, slug), `v`/`V`/`ctrl+v` visual block, row and column selections, `u` / `ctrl+r` undo and redo, `ctrl+s` save. A save keeps the delimiter the file was read with.

Transforms act on the innermost thing on the stack: in the table the `t` rows reach the cell, and in a selection the selected cells (`f` there fills down from the top selected cell). `row.transform` (every shown cell of the row) and `column.transform` (every visible cell of the column, through the filter) carry the same `t` rows on their nouns and run from the palette (`: row transform upper`) or a `bindings.json` row; `transformations.list` names the transformations. Inside a selection, `s` sorts by the cursor's column, like `s` outside one, and ends the selection. Every edit is one patch, and its inverse is the undo record. Sorting while editing is an undoable edit. Read-only, a sort only reorders the view.

## Settings

Two knobs live in `~/.config/o-csv/settings.json` (`$XDG_CONFIG_HOME/o-csv`), declared as ochazuke settings, so the viewer and the command line read the same file:

| Setting | Default | Does |
|---|---|---|
| `delimiter` | `auto` | the field delimiter when `--delim` is not given: `auto` (detect), `comma`, `tab`, `semicolon`, `pipe`, or one character |
| `column-width` | `32` | the widest a column grows in the viewer, in cells (3 to 200) |

`o csv settings list`, `o csv settings set column-width 24` and `o csv settings reset delimiter` change them; in the viewer `: settings edit` opens the settings editor and a change applies at once. `o csv config export` writes a reference of every setting and key row into `~/.config/o-csv/reference/`.

## Command line

`o csv table VERB FILE [--ARG VALUE]` reads the file in the process and prints JSON. `-` reads stdin. Every verb takes `--delim` and `--no_header`. `--format table|markdown|toon` prints the rows of `table show` (and `table columns`) as a table instead: `table show` draws the CSV's own columns, numbers right-aligned. `--json '{...}'` passes the arguments as JSON instead, and a bare `--json` is accepted and ignored.

| Command | Prints |
|---|---|
| `o csv table show FILE` | the matching rows, each `{noun: "row", id: "row/N", data, cells}`: `N` is its row in the file (whatever the sort), `data` the row as an object keyed by column in header order, `cells` its values. When `--offset`/`--limit` leave rows out, stderr says `status: rows 11–20 of 250`. Options: `--columns a,b`, `--search TEXT`, `--where COL~TEXT`, `--sort COL [--desc]`, `--offset N`, `--limit N` |
| `o csv table columns FILE` | `[{index, name, type}]` |
| `o csv table stats FILE` | the row and column counts, the delimiter, and per column: the guessed type (empty, bool, int, float, date, string), empty and distinct counts, min, max, widest value |
| `o csv catalog` | the runtime catalog for a host that mounts the bin: the `table` noun and every noun, verb and key row of the viewer (`sheet`, `cell`, `row`, `column`, the selection, `transformations`). On the command line no sheet is open, so the viewer's edit and grid verbs answer `is not applicable now`; `o csv operations list` lists them all |

`o csv help` lists the commands, and `o csv table --help` lists the verbs and their parameters.

## How o-csv is built (a reference app)

o-csv is the family's reference for building an application on the current ochazuke (the version in [go.mod](go.mod)): a separate module that imports `ofam.dev/ochazuke`, owns its state, and gets keys, help, the palette, the jobs ticker, settings and a CLI from the framework. Read the files in this order; each one shows one concept.

| Concept | Where | What to look at |
|---|---|---|
| The app owns its state | `app.go` | `App` holds the file, the view state and the undo stack; the runtime holds only interaction state, and instances carry ids, never data. `Patch` is the one edit shape, and its inverse is the undo record. |
| Domain code without the framework | `sheet.go` | Reading, writing, delimiter detection, type guesses and stats are plain Go, tested on their own. |
| Building the tree | `main.go` (`New`, `newSheet`) | `NewRuntime`, an instance for the page, a component under it (`grid.New`), `AddDetail`, an input (`filter.go`), `AddPage`, `OpenPage`, `NewModel`. |
| Nouns, verbs, guards, confirms | `verbs.go` | `Define(noun).Verb(verb, handler, description)`; `.When(guard)` hides and skips a verb (`DefineGuard("edit", …)`), `.Confirm("Delete row {{id}}?")` asks first, `.Motion()` keeps a verb off the footer. Every verb declares its history kind: `.Fetch()` on a read (`sheet.state`, the `table` verbs), `setHistory(r, ochazuke.HistoryChange|HistoryAction, keys...)` for edits and view moves, so none falls to the default (`history_test.go`). `setRepeatable` marks the yanks `Operation.Repeatable`, so `.` repeats them. Handlers are `func(Ctx) Result` (`Done`, `Failed`, `Pending`). |
| Binding rows as data | `bindings.go`, `filter.go`, `main.go` | Every key is a row: `Do("noun.verb", args)`, arguments from the key's payload (`FromPayload("steps", SourceCount)`), a pattern row for digits, mode tables that extend others, instance rows (`grid.WithInstanceRows`, `r.Bind`). No key-name switch anywhere; `TestBindingRowsAreAtomic` runs `ValidateBindings`. |
| Work off the loop | `verbs.go` (`sheet.load`, `copyText`), `shell.go` | `Pending(work)` runs I/O as a job; `.Replacing("load")` lets a newer load cancel an older one; `.Then` runs on the loop when it lands. Reads are `.Quiet()` so they stay off the ticker. |
| Telling the user | `verbs.go` (`copyText`), `app.go` (`saveTo`) | `ctx.Info("copied …")` for a plain message, `ctx.Status` (the error tone) for a failure, `ctx.Notify(…)` / `ctx.ClearNotice(id)` for a lasting problem, `r.UI().Info` where there is no `Ctx`. The TUI shows them in the footer; the CLI prints `status:` (plain), `error:` and `notice:` on stderr. |
| A page detail | `detail.go` | `AddDetail(id, item)` and `Detail.Window`: the framework keeps the scroll offset, `J`/`K` scroll it while the focus stays in the grid, and a new item starts at the top. |
| Resizable panes | `view.go` (`sheetView`) | `r.PaneLayout(page, layout)` and `Layout.Divide` make `ctrl+w >` `<` `=` move the divider, remembered per page; the sheet keeps its own rows off those keys. |
| Settings | `settings.go` | `DeclareSetting` with a `Validate`, read with `Settings().String/Int`, followed with `Observe`. `settings list|set`, the settings editor and `config export` cover them. |
| Modals | `prompt.go` | `OpenModal(kind, Payload, WithInput(in, Kind("field")), CloseWith(cancel))` for a one-line question; the noun pins insert mode, `enter` commits, and `esc` runs `modal.close`, which calls `field.cancel` to clear the question. |
| Drawing a component's view | `view.go` (`tableView`) | The grid owns the cursor and the visible rows; the app draws only the window and calls `Placed`/`PlacedAt`, so motions and scrolling follow the screen. `Runtime.Selection(focused)` paints its cursor with the framework's focus and appearance settings; `EmptyText`, `LoadingText` and `NoMatchText` supply placeholders. |
| The CLI and the catalog | `shell.go`, `main.go` (`Main`, `newSheet`) | The viewer's runtime without its page (`newSheet`), plus the `table` noun, behind `ocli.New`, so the catalog describes every verb and key row while the guards keep the viewer's verbs inert there; mounted on oden with `ocli.Mount`: parameters, `.Positional("path")`, `.Rows("row")`, rows in the `{noun, id, data, cells}` shape and the noun's columns, so `--format table|markdown|toon` work. `o csv catalog` publishes it. |
| Tests | `app_test.go`, `shell_test.go`, `keys_test.go` | Drive the `Model` with key names (`Press`, `runCmds` for jobs), run the CLI through `Main`, check the frame with `--smoke`. |

## Build

```sh
GOWORK=off GOFLAGS=-trimpath go build -o o-csv .
gofmt -l . && GOWORK=off go vet ./... && GOWORK=off go test ./...
GOWORK=off go run . --smoke testdata/cities.csv
```
