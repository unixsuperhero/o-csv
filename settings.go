package main

import (
	"fmt"

	"github.com/unixsuperhero/ochazuke"
)

// The knobs a user sets once instead of on every command line, declared as
// ochazuke settings: they live in ~/.config/o-csv/settings.json, and
// `o-csv settings list|set|reset`, `o-csv config export` and the TUI's
// settings editor (`: settings edit`) all know them. Both runtimes declare
// them, the viewer's (New) and the command line's (newShell), so the CLI
// and the TUI read one file. A flag (--delim) still wins for one run.

const (
	settingDelimiter   = "delimiter"
	settingColumnWidth = "column-width"
)

// declareSettings declares o-csv's settings on r.
func declareSettings(r *ochazuke.Runtime) error {
	return r.DeclareSetting(
		ochazuke.Setting{
			Name: settingDelimiter, Type: ochazuke.SettingString, Default: "auto",
			Description: "Field delimiter when --delim is not given: auto (detect), comma, tab, semicolon, pipe, or one character",
			Validate: func(v any) error {
				_, err := ParseDelim(v.(string))
				return err
			},
		},
		ochazuke.Setting{
			Name: settingColumnWidth, Type: ochazuke.SettingInteger, Default: 32,
			Description: "The widest a column grows in the viewer, in cells; longer values are cut with …",
			Validate: func(v any) error {
				if n := v.(int); n < minCellWidth || n > 200 {
					return fmt.Errorf("between %d and 200", minCellWidth)
				}
				return nil
			},
		},
	)
}
