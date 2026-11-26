// Package statusrow renders the terminal status glyph on run and job list rows.
package statusrow

import (
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"
)

func Render(w io.Writer, d list.DefaultDelegate, m list.Model, index int, item list.Item, status string) {
	glyph := cliruns.StatusGlyph(status, 0)
	if index == m.Index() || (glyph != "✓" && glyph != "✗") {
		d.Render(w, m, index, item)
		return
	}
	var row strings.Builder
	d.Render(&row, m, index, item)
	title, description, _ := strings.Cut(row.String(), "\n")
	style := d.Styles.NormalTitle
	if m.SettingFilter() && m.FilterValue() == "" {
		style = d.Styles.DimmedTitle
	}
	statusColor := ansi.NewStyle().ForegroundColor(cliruns.StatusGlyphStyle(status).GetForeground()).String()
	titleColor := ansi.NewStyle().ForegroundColor(style.GetForeground()).String()
	// Apply color after matching; restoring only color and intensity preserves filter underlines.
	icon := ansi.NewStyle().Bold().String() + statusColor + glyph + titleColor + ansi.NewStyle().Normal().String()
	title = strings.Replace(title, glyph, icon, 1)
	_, _ = io.WriteString(w, title+"\n"+description)
}
