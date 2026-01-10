package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"bore/internal/web/logger"
)

type model struct {
	table   table.Model
	width   int
	height  int
	getLogs func() []*logger.Log
	appURL  string
}

type tickMsg struct{}

func (m model) Init() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(msg.Height - 5)
		m.table.SetColumns(getColumns(msg.Width))

	case tickMsg:
		if m.getLogs != nil {
			logs := m.getLogs()
			m.table.SetRows(logsToRows(logs))
		}

		return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
			return tickMsg{}
		})
	}

	m.table, cmd = m.table.Update(msg)

	return m, cmd
}

func (m model) View() string {
	urlLine := lipgloss.
		NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Width(m.width).
		Align(lipgloss.Center).
		Render(fmt.Sprintf("Bore URL: %s", m.appURL))

	webInspectorLine := lipgloss.
		NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(m.width).
		Align(lipgloss.Center).
		Render("Web Inspector URL: http://localhost:8000")

	requestLoggerTable := lipgloss.
		NewStyle().
		Width(m.width).
		Height(m.height - 4).
		Render(m.table.View())

	return urlLine + "\n" + webInspectorLine + "\n\n" + requestLoggerTable
}

func getColumns(width int) []table.Column {
	if width <= 0 {
		width = 80
	}

	width = width - 8 // leave room for borders/padding
	methodWidth := width * 10 / 100
	statusWidth := width * 10 / 100
	respTimeWidth := width * 25 / 100
	uriWidth := width - methodWidth - statusWidth - respTimeWidth

	return []table.Column{
		{Title: "Method", Width: methodWidth},
		{Title: "URI", Width: uriWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Response Time (ms)", Width: respTimeWidth},
	}
}

func logsToRows(logs []*logger.Log) []table.Row {
	var rows []table.Row

	for _, log := range logs {
		method := ""
		uri := ""
		status := ""
		respTime := ""

		if log.Request != nil {
			method = log.Request.Method
			uri = log.Request.Path
		}

		if log.Response != nil {
			status = fmt.Sprintf("%d", log.Response.StatusCode)
		}

		respTime = fmt.Sprintf("%d", log.Duration)
		rows = append(rows, table.Row{method, uri, status, respTime})
	}

	return rows
}

func NewModel(getLogs func() []*logger.Log, appURL string) model {
	columns := getColumns(80)

	var rows []table.Row

	if getLogs != nil {
		rows = logsToRows(getLogs())
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
	)

	s := table.DefaultStyles()

	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)

	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)

	t.SetStyles(s)

	return model{
		table:   t,
		getLogs: getLogs,
		appURL:  appURL,
	}
}
