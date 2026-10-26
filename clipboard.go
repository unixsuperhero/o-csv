package main

import (
	"os/exec"
	"strings"
)

// systemClipboard is a copy function for the first clipboard tool on PATH
// (pbcopy, wl-copy, xclip, xsel), or nil when there is none. Copies run off
// the loop (copyText), so a slow tool never stalls keys.
func systemClipboard() func(string) error {
	for _, tool := range [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}} {
		path, err := exec.LookPath(tool[0])
		if err != nil {
			continue
		}
		args := tool[1:]
		return func(text string) error {
			cmd := exec.Command(path, args...)
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}
	return nil
}
