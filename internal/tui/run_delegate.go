package tui

import (
	"io"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"github.com/iw2rmb/ploy/internal/tui/statusrow"
)

type runDelegate struct {
	list.DefaultDelegate
	spinnerFrame int
}

func newRunDelegate(focused bool, frame int) runDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(3)
	if !focused {
		delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.BorderStyle(lipgloss.HiddenBorder())
		delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.BorderStyle(lipgloss.HiddenBorder())
	}
	return runDelegate{DefaultDelegate: delegate, spinnerFrame: frame}
}

func (d runDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	run, ok := item.(runItem)
	if !ok {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}
	run.spinnerFrame = d.spinnerFrame
	statusrow.Render(w, d.DefaultDelegate, m, index, run, string(run.run.Status))
}
