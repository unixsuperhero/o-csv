# o-csv: agent guide

`o-csv` (so `o csv`) is the family's CSV viewer: a full-screen ochazuke table over one file or stdin, with editing behind `--edit` or `ctrl+e`, and a `table` noun on the command line that prints JSON. It is a copy of ochazuke's `examples/csv` that has been adapted, not an import of it. Usage and keys are in README.md.

It is an ochazuke application, so ochazuke's rules apply here unchanged. Read `~/proj/ochazuke/AGENTS.md` first, especially "Building an application on ochazuke" and the Invariants: keys are binding rows (no key-name switches), every row is atomic, motions are visual, hints derive from bindings, Bubble Tea v2 idioms only, never bind F1 to F12.

## Plans and todos

Family plans, todos and the roadmap live in `~/hub/o-plans/` (`README.md` is the index). Read it before planning work; add or update items there. Do not add a TODO.md or docs/plans/ to this repo.

## Where does X live

| Concern | File |
|---|---|
| `main`, oden dispatch (a file word opens the TUI), viewer flags, `--smoke`, `New` (the runtime, page, grid) | `main.go` |
| Reading and writing CSV, delimiter detection, type guesses, stats, cell ordering | `sheet.go` |
| `App` state, `Patch` and undo, `Load`/`Save`, sort, the edit toggle | `app.go` |
| The table view (window, widths, frozen and hidden columns, highlights) and the status line | `view.go` |
| Verbs: viewer verbs, then the guarded edit verbs | `verbs.go` |
| Key rows | `bindings.go` |
| Row detail modal | `detail.go` |
| Filter field, prompt modal | `filter.go`, `prompt.go` |
| Edit-mode transformations and shaped yanks/pastes | `edit.go` |
| System clipboard | `clipboard.go` |
| The `table` CLI noun (`show`, `columns`, `stats`) and `catalog` | `shell.go` |

## Invariants

- Read-only never writes: every edit verb is guarded by `edit`/`edit-grid`, and `Apply`, `UndoPatch`, `RedoPatch` and `Save` refuse while `App.Edit` is false. `TestReadOnlyNeverWrites` pins this.
- Filter, search, hide, freeze and a read-only sort are view state. Only patches are edits.
- File reads run off the loop (`sheet.load`, the `table` verbs are `Pending`). `view.go` draws only the window, so per-frame cost never scales with the row count.
- The grid component owns the cursor and the visible rows. o-csv draws them itself (`tableView`) and calls `Placed`/`PlacedAt` so the page follows the cursor.

## Checks

```sh
gofmt -l . && go vet ./... && GOWORK=off go test -race ./...
go run . --smoke testdata/cities.csv
go run . catalog >/dev/null
```

ochazuke (`v0.6.0`) and oden (`v0.1.1`) are required at their tags. An untracked `go.work` (`go work init . ../ochazuke ../oden`) builds against the sibling checkouts, and `GOWORK=off` builds from the tags as a fresh clone does. Never commit `go.work` or a `replace`.
