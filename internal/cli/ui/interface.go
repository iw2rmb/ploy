package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type Model struct{}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	return "Ploy CLI — try `ploy apps new` or `ploy push`\n"
}

func Usage() {
	fmt.Println(`Ploy CLI
Usage:
  ploy apps new --lang <go|node> --name <app>
  ploy apps destroy --name <app> [--force]
  ploy push -a <app> [-lane A|B|C|D|E|F] [-main com.example.Main] [-sha <sha>]
  ploy open <app>
  ploy env list <app>
  ploy env set <app> <key> <value>
  ploy env get <app> <key>
  ploy env delete <app> <key>
  ploy domains add <app> <domain>
  ploy domains list <app>
  ploy domains remove <app> <domain>
  ploy certs issue <domain>
  ploy certs list
  ploy debug shell <app> [--lane <A-F>]
  ploy rollback <app> <sha>
  ploy arf recipe generate --repo <path> --type <type>
  ploy arf transform <path> --recipe <id>
  ploy arf validate <recipe-file>
  ploy arf patterns list
  ploy arf test ab --recipe1 <id> --recipe2 <id>
  ploy arf status`)
}