package tui

import (
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
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
	if ok {
		run.spinnerFrame = d.spinnerFrame
		item = run
	}
	if !ok || index == m.Index() || (run.run.Status != domaintypes.RunStatusSuccess && run.run.Status != domaintypes.RunStatusFail) {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}

	var row strings.Builder
	d.DefaultDelegate.Render(&row, m, index, item)
	title, description, _ := strings.Cut(row.String(), "\n")
	style := d.Styles.NormalTitle
	if m.SettingFilter() && m.FilterValue() == "" {
		style = d.Styles.DimmedTitle
	}
	glyph := cliruns.StatusGlyph(string(run.run.Status), 0)
	statusColor := ansi.NewStyle().ForegroundColor(cliruns.StatusGlyphStyle(string(run.run.Status)).GetForeground()).String()
	titleColor := ansi.NewStyle().ForegroundColor(style.GetForeground()).String()
	// Apply color after matching; restoring only color and intensity preserves filter underlines.
	icon := ansi.NewStyle().Bold().String() + statusColor + glyph + titleColor + ansi.NewStyle().Normal().String()
	title = strings.Replace(title, glyph, icon, 1)
	_, _ = io.WriteString(w, title+"\n"+description)
}
