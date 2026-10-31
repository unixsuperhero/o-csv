// Command o-csv is the family's CSV viewer: a full-screen table over one
// file (or stdin) with vim motions, search, a column filter, sorting,
// hidden and frozen columns, a row detail pane and copy to the registers
// and the system clipboard. Editing (ochazuke's CSV example: a patch per
// edit, undo, write-back) is off unless --edit or ctrl+e turns it on;
// read-only, nothing writes the file.
//
//	o-csv [--delim D] [--no-header] [--edit] [--smoke] FILE|-
//	o-csv table show|columns|stats FILE [--ARG VALUE]
//	o-csv catalog
//
// The table noun (shell.go) is the command line: it reads the file in
// process and prints JSON. The command line runs on the viewer's runtime
// without its page (newSheet), so the catalog lists the viewer's verbs;
// with no sheet open there, its guards keep them inert.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
	"ofam.dev/ochazuke/grid"
	"ofam.dev/oden"
)

const (
	pageID = "sheet"
	gridID = "grid"
)

func main() {
	os.Exit(oden.Report(os.Stderr, progName(), Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)))
}

// progName is how messages name this command: the word under the parent
// oden app that ran it, else the bin name.
func progName() string {
	if parent := os.Getenv("ODEN_PARENT"); parent != "" {
		return parent + " csv"
	}
	return "o-csv"
}

// Main runs one invocation. A first word that is an existing file opens it
// in the TUI even when it shadows a command name; otherwise oden resolves
// the command words, and anything it does not know (flags, -, a path)
// reaches the TUI.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 && isFile(args[0]) {
		return runTUI(args, stdin, stdout, stderr)
	}
	app := oden.New("o-csv", args, oden.WithBuiltins(), oden.WithStdio(stdin, stdout, stderr),
		oden.WithDefault(func(_ context.Context, args []string) error {
			if len(args) > 0 && !strings.HasPrefix(args[0], "-") && !strings.ContainsAny(args[0], "./") {
				return oden.ErrNotFound
			}
			return runTUI(args, stdin, stdout, stderr)
		}))
	ocli.Mount(app, newShell(stdin, stdout, stderr))
	return app.Run(context.Background())
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// runTUI parses the viewer's flags and runs it (or renders one frame).
func runTUI(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("o-csv", flag.ContinueOnError)
	flags.SetOutput(stderr)
	delim := flags.String("delim", "", "field delimiter: one character, or auto, comma, tab, semicolon, pipe (default: the delimiter setting, auto)")
	noHeader := flags.Bool("no-header", false, "the first line is data; columns are named A, B, C, ...")
	edit := flags.Bool("edit", false, "start with editing on (ctrl+e toggles it)")
	smoke := flags.Bool("smoke", false, "render one frame to stdout and exit")
	width := flags.Int("width", 80, "--smoke frame width")
	height := flags.Int("height", 24, "--smoke frame height")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: o-csv [--delim D] [--no-header] [--edit] [--smoke] FILE|-\n       o-csv table show|columns|stats FILE\n       o-csv catalog\n\nKeys: ? in the viewer lists them; README.md has the table.")
		flags.PrintDefaults()
	}
	words, err := parseInterspersed(flags, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &oden.ExitError{Code: 2}
	}
	path := ""
	switch {
	case len(words) > 1:
		return errors.New("one file at a time")
	case len(words) == 1:
		path = words[0]
	case !isTerminal(stdin):
		path = "-"
	default:
		flags.Usage()
		return &oden.ExitError{Code: 2}
	}
	a, err := New(Options{NoHeader: *noHeader}, *edit)
	if err != nil {
		return err
	}
	// --delim wins; without it the delimiter setting applies (auto detects).
	if *delim == "" {
		*delim = a.r.Settings().String(settingDelimiter)
	}
	if a.Opts.Delim, err = ParseDelim(*delim); err != nil {
		return err
	}
	if *smoke {
		if err := a.Load(path); err != nil {
			return err
		}
		a.m.Width, a.m.Height = *width, *height
		_, err := fmt.Fprintln(stdout, ansi.Strip(a.m.View().Content))
		return err
	}
	a.Clipboard = systemClipboard()
	var opts []tea.ProgramOption
	if path == "-" {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("reading the file from stdin needs a terminal for keys: %w", err)
		}
		defer tty.Close()
		opts = append(opts, tea.WithInput(tty))
	}
	if _, err := a.r.Execute(ochazuke.Request{Noun: "sheet", Verb: "load", Target: pageID, Args: ochazuke.Arguments{"path": path}}); err != nil {
		return err
	}
	_, err = tea.NewProgram(a.m, opts...).Run()
	if a.Dirty {
		fmt.Fprintln(stderr, "o-csv: unsaved edits were discarded")
	}
	return err
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// parseInterspersed parses flags that may come before or after the words.
func parseInterspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	var words []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		args = flags.Args()
		if len(args) == 0 {
			return words, nil
		}
		if args[0] == "-" || !strings.HasPrefix(args[0], "-") {
			words = append(words, args[0])
			args = args[1:]
			continue
		}
		words = append(words, args...)
		return words, nil
	}
}

// New builds the viewer on a fresh runtime: the sheet page holding the
// grid, the row detail and the filter field, the prompt, the picker, the
// nouns, verbs and bindings. The sheet starts empty; Load or sheet.load
// fills it.
func New(opts Options, edit bool) (*App, error) {
	a, err := newSheet(opts, edit)
	if err != nil {
		return nil, err
	}
	if _, err := a.r.AddPage(pageID, "Sheet", a.render, ochazuke.Focus(gridID)); err != nil {
		return nil, err
	}
	if err := a.r.OpenPage(pageID); err != nil {
		return nil, err
	}
	return a, nil
}

// newSheet is New without the page: the runtime with every noun, verb,
// guard and key row the viewer has, and the sheet instance holding the
// grid, the detail and the filter field. The command line (newShell)
// builds on it, so the catalog and operations list describe the viewer's
// verbs; nothing focuses the grid there and no sheet is open, so the
// guarded verbs stay inert.
func newSheet(opts Options, edit bool) (*App, error) {
	r := ochazuke.NewRuntime()
	a := &App{r: r, Opts: opts, Edit: edit, Hidden: map[int]bool{}}
	a.m = ochazuke.NewModel(r)
	a.m.Title = "o-csv"
	if err := declareSettings(r); err != nil {
		return nil, err
	}
	a.colCap = r.Settings().Int(settingColumnWidth)
	r.Settings().Observe(settingColumnWidth, func(_ string, v any) {
		a.colCap = v.(int)
		if a.grid != nil {
			a.grid.MaxCellWidth = a.colCap
		}
		a.measure()
	})
	// The sheet instance is registered first so the grid can sit under it;
	// AddPage then adopts it as the page of the sheet noun.
	r.Instance(pageID, ochazuke.Kind("sheet"))
	g, err := grid.New(r, gridID, pageID, nil, nil, grid.WithInstanceRows(a.instanceRows))
	if err != nil {
		return nil, err
	}
	a.grid = g
	// z > and z < step from the grid's automatic width; capping it as the
	// view caps a column keeps the first step close to the drawn width.
	g.MaxCellWidth = a.colCap
	a.registerPrompt()
	a.registerVerbs()
	if err := a.registerDetail(); err != nil {
		return nil, err
	}
	if err := a.registerFilter(); err != nil {
		return nil, err
	}
	a.bind()
	picker := ochazuke.NewPicker(r)
	a.m.ModalViews["picker"] = ochazuke.ModalView{Title: "Choose a path", Render: func(_ *ochazuke.Runtime, _ *ochazuke.Instance, w, h int) ochazuke.Block {
		return picker.ViewBlock(w, h)
	}}
	return a, nil
}
