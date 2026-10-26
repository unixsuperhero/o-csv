# o-csv

A CSV viewer for the o family, built on [ochazuke](../ochazuke) (Bubble Tea v2) with an [oden](../oden) command line. It started as a copy of ochazuke's `examples/csv` editor. Viewing comes first. Editing is still there, but it stays off until you ask for it.

```sh
o-csv data.csv                  # open a file
some-command | o-csv            # or read stdin (also: o-csv -)
o-csv --delim tab --no-header x.txt
o-csv --edit data.csv           # start with editing on
o-csv --smoke data.csv          # render one 80x24 frame to stdout and exit
```

Build: `go build -o ~/bin/o-csv .`

## The viewer

The table fills the screen: a header row, then only the rows that fit on screen. That means a 100k-row file costs one screen per frame, and it loads off the event loop (`sheet.load` is pending work, and the screen says `loading …` while it runs). Each column is as wide as the widest of its first 2,000 values and of the rows drawn so far, capped at 32 cells. Longer cells are cut with `…`. On a narrow screen the columns scroll with the cursor, and the next column shows partly while there is room for it. The status line above the table shows the file, `read-only` or `EDIT`, `row N/M` (with `(of TOTAL)` while a filter is on), `col N/M NAME`, and the active sort, filter, search, frozen column and hidden-column count. The delimiter is guessed from the first 64 KiB, trying comma, tab, semicolon and pipe. `--delim` overrides it. With `--no-header`, the columns are named A, B, C, …

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
| `-` / `=` | hide the cursor's column / show every column again |
| `F` | freeze the first shown column at the left (toggle) |
| `enter` | row detail: a modal with each column and its value, the long value wrapped below. `j`/`k` move, `y` copies the value, `esc`/`q`/`enter` close |
| `y` / `Y` | copy the cell / copy the row as one CSV line. The copy goes to the ochazuke registers (`"a y` names one) and to the system clipboard (pbcopy, wl-copy, xclip or xsel, whichever is on PATH) |
| `ctrl+o` | open another file |
| `:` | the command palette (every verb, including those without a key) |
| `ctrl+c` | quit |

### Editing

Editing is off by default. Start with `--edit` or press `ctrl+e` to toggle it. Leaving edit mode with unsaved edits is refused. While editing is off, every edit verb is guarded: its key does nothing, the palette does not offer it, and the sheet refuses it underneath. Nothing in a read-only session writes a file (`TestReadOnlyNeverWrites`).

While editing is on, ochazuke's CSV example keys apply: `i` edit a cell, `x` clear, `p` paste, `d d` delete a row (it asks first), `o` insert a row, `r` rename a column, `D` delete a column, `+` add a column, `t u`/`t l`/`t t`/`t i`/`t s` transform, `v`/`V`/`ctrl+v` visual block, row and column selections, `u` / `ctrl+r` undo and redo, `ctrl+s` save. A save keeps the delimiter the file was read with. Every edit is one patch, and its inverse is the undo record. Sorting while editing is an undoable edit. Read-only, a sort only reorders the view.

## Command line

`o-csv table VERB FILE [--ARG VALUE]` reads the file in the process and prints JSON. `-` reads stdin. Every verb takes `--delim` and `--no_header`. The output is always JSON. `--json '{...}'` passes the arguments as JSON instead, and a bare `--json` is accepted and ignored.

| Command | Prints |
|---|---|
| `o-csv table show FILE` | `{columns, total, offset, rows}`, where each row is an object keyed by column in header order. Options: `--columns a,b`, `--search TEXT`, `--where COL~TEXT`, `--sort COL [--desc]`, `--offset N`, `--limit N` |
| `o-csv table columns FILE` | `[{index, name, type}]` |
| `o-csv table stats FILE` | the row and column counts, the delimiter, and per column: the guessed type (empty, bool, int, float, date, string), empty and distinct counts, min, max, widest value |
| `o-csv catalog` | the runtime catalog for a host that mounts the bin |

`o-csv help` lists the commands, and `o-csv table --help` lists the verbs and their parameters.
