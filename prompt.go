package main

import (
	"errors"

	"ofam.dev/ochazuke"
)

// The prompt: one single-line input of the field noun (insert pinned, like
// the framework's prompt noun) in one modal, reused by every verb that needs
// a line of text: cell edit, rename, add column, filter, open and save-as.
// enter commits, esc cancels, ctrl+o opens the path picker into the field.
// OpenModal opens the body and the field in one call; the label rides in the
// body's payload, the commit is application state, and the *Input keeps its
// undo tree across uses although its instance closes with the modal.

const promptFrame = "o-csv-prompt"
const fieldID = "field"

type question struct {
	commit func(text string) error
}

// registerPrompt creates the field and the verbs that close it.
func (a *App) registerPrompt() {
	r := a.r
	r.Define("field").Pin("insert").
		Verb("commit", func(r *ochazuke.Runtime, c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			q := a.ask
			if q == nil || r.Instances[r.Focus] == nil || r.Focus != fieldID {
				return nil, errors.New("no prompt is open")
			}
			text := string(a.field.Text)
			a.ask = nil
			if err := r.Pop(); err != nil {
				return nil, err
			}
			return text, q.commit(text)
		}, "Close the prompt and run the question with the field's text").
		Verb("cancel", func(r *ochazuke.Runtime, c ochazuke.Context, args ochazuke.Arguments) (any, error) {
			if a.ask == nil {
				return nil, errors.New("no prompt is open")
			}
			a.ask = nil
			return nil, r.Pop()
		}, "Close the prompt without running the question").
		Mode("insert").
		Bind("enter", ochazuke.Do("field.commit")).
		Bind("esc", ochazuke.Do("modal.close")).
		Bind("ctrl+o", ochazuke.Do("picker.open"))
	// commit runs the question: the edit, rename or path it asked for.
	setHistory(r, ochazuke.HistoryChange, "field.commit")
	setHistory(r, ochazuke.HistoryAction, "field.cancel")
	a.field = ochazuke.NewInput(fieldID, "", false)
	a.m.ModalViews[promptFrame] = ochazuke.ModalView{Title: "Prompt", Render: func(r *ochazuke.Runtime, body *ochazuke.Instance, w, h int) ochazuke.Block {
		field := ochazuke.InputBlock(a.field, max(1, w), 1)
		return field.Offset(0, 2).WithText(body.Payload.Prompt + "\n\n" + field.Text)
	}}
}

// prompt opens the modal with a label, initial text and kind ("path" lets
// ctrl+o browse); commit runs with the text when enter is pressed.
func (a *App) prompt(label, text, kind string, commit func(string) error) error {
	if a.ask != nil {
		return errors.New("a prompt is already open")
	}
	a.field.Kind = kind
	if err := a.field.Replace(0, len(a.field.Text), text); err != nil {
		return err
	}
	a.field.Cursor = len(a.field.Text)
	view := a.m.ModalViews[promptFrame]
	view.Title = label
	a.m.ModalViews[promptFrame] = view
	a.ask = &question{commit: commit}
	if _, err := a.r.OpenModal(promptFrame, ochazuke.Payload{Prompt: label},
		ochazuke.WithInput(a.field, ochazuke.Kind("field")),
		ochazuke.CloseWith(ochazuke.Request{Noun: "field", Verb: "cancel"})); err != nil {
		a.ask = nil
		return err
	}
	return nil
}
