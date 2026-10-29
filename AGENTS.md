# o-csv: agent guide

`o-csv` (so `o csv`) is the family's CSV viewer: a full-screen ochazuke table over one file or stdin, with editing behind `--edit` or `ctrl+e`, and a `table` noun on the command line that prints JSON. It is a copy of ochazuke's `examples/csv` that has been adapted, not an import of it. Usage and keys are in README.md.

o-csv is the family's external reference consumer of ochazuke: the example an author reads to build an app on the latest release (README "How o-csv is built" maps each file to the concept it shows). Keep it small and readable, adopt each new ochazuke feature here first where it fits, and keep that README table true when a file changes its job.

It is an ochazuke application, so ochazuke's rules apply here unchanged. Read `~/proj/ochazuke/AGENTS.md` first, especially "Building an application on ochazuke" and the Invariants: keys are binding rows (no key-name switches), every row is atomic, motions are visual, hints derive from bindings, Bubble Tea v2 idioms only, never bind F1 to F12.

## Plans and todos

Family plans, todos and the roadmap live in `~/hub/o-plans/` (`README.md` is the index). Read it before planning work; add or update items there. Do not add a TODO.md or docs/plans/ to this repo.

## Where does X live

| Concern | File |
|---|---|
| `main`, oden dispatch (a file word opens the TUI), viewer flags, `--smoke`, `New` (the page) over `newSheet` (the runtime, grid, detail, settings, verbs, keys; the CLI shares it) | `main.go` |
| Reading and writing CSV, delimiter detection, type guesses, stats, cell ordering | `sheet.go` |
| `App` state, `Patch` and undo, `Load`/`Save`, sort, the edit toggle | `app.go` |
| The table view (window, widths, frozen and hidden columns, highlights), the table/detail split (`PaneLayout`) and the status line | `view.go` |
| Verbs: viewer verbs, then the guarded edit verbs | `verbs.go` |
| Key rows | `bindings.go` |
| Row detail pane (an ochazuke `Detail`: J/K scroll it) | `detail.go` |
| Filter field, prompt modal | `filter.go`, `prompt.go` |
| Edit-mode transformations and shaped yanks/pastes | `edit.go` |
| System clipboard | `clipboard.go` |
| The `table` CLI noun (`show` answers rows with cells for `--format`, `columns`, `stats`) and `catalog`, over the viewer's runtime without its page (`newSheet`) | `shell.go` |
| Declared settings (`delimiter`, `column-width`), shared by the viewer and the CLI | `settings.go` |

## Invariants

- Read-only never writes: every edit verb is guarded by `edit`/`edit-grid`, and `Apply`, `UndoPatch`, `RedoPatch` and `Save` refuse while `App.Edit` is false. `TestReadOnlyNeverWrites` pins this.
- Filter, search, hide, freeze and a read-only sort are view state. Only patches are edits.
- File reads run off the loop (`sheet.load`, the `table` verbs are `Pending`): a file read is I/O, and application I/O never runs on the loop. Settings are read on the loop and passed to the work. `view.go` draws only the window, so per-frame cost never scales with the row count.
- The grid component owns the cursor and the visible rows. o-csv draws them itself (`tableView`) and calls `Placed`/`PlacedAt` so the page follows the cursor.

## Checks

```sh
gofmt -l . && go vet ./... && GOWORK=off go test -race ./...
go run . --smoke testdata/cities.csv
go run . catalog >/dev/null
go mod tidy   # no diff
```

ochazuke (`v0.31.0`) and oden (`v0.1.1`) are required at their tags. An untracked `go.work` (`go work init . ../ochazuke ../oden`) builds against the sibling checkouts, and `GOWORK=off` builds from the tags as a fresh clone does. Never commit `go.work` or a `replace`.
