package tui

import (
	"bore/internal/client"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	table       table.Model
	width       int
	height      int
	logger      *client.Logger
	appURL      string
	filterMode  bool
	filterQuery string
	cursorPos   int
	filterError string
	filters     []*client.Filter
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
		// Handle filter input mode
		if m.filterMode {
			switch msg.String() {
			case "esc":
				m.filterMode = false
				m.filterQuery = ""
				m.cursorPos = 0
				m.filterError = ""
				return m, nil
			case "enter":
				parsedFilters, err := client.ParseQuery(m.filterQuery)
				if err != nil {
					m.filterError = err.Error()
				} else {
					m.filters = parsedFilters
					m.filterError = ""
					m.filterMode = false
					m.cursorPos = 0
					m.updateTableRows()
				}
				return m, nil
			case "left":
				if m.cursorPos > 0 {
					m.cursorPos--
				}
				return m, nil
			case "right":
				if m.cursorPos < len(m.filterQuery) {
					m.cursorPos++
				}
				return m, nil
			case "home", "ctrl+a":
				m.cursorPos = 0
				return m, nil
			case "end", "ctrl+e":
				m.cursorPos = len(m.filterQuery)
				return m, nil
			case "backspace":
				if m.cursorPos > 0 && len(m.filterQuery) > 0 {
					m.filterQuery = m.filterQuery[:m.cursorPos-1] + m.filterQuery[m.cursorPos:]
					m.cursorPos--
				}
				return m, nil
			case "delete", "ctrl+d":
				if m.cursorPos < len(m.filterQuery) {
					m.filterQuery = m.filterQuery[:m.cursorPos] + m.filterQuery[m.cursorPos+1:]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.filterQuery = m.filterQuery[:m.cursorPos] + msg.String() + m.filterQuery[m.cursorPos:]
					m.cursorPos++
				}
				return m, nil
			}
		}

		// Normal mode keys
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "f":
			m.filterMode = !m.filterMode
			if m.filterMode {
				if len(m.filters) > 0 {
					m.filterQuery = client.FormatQuery(m.filters)
				}
				m.cursorPos = len(m.filterQuery)
			}
			return m, nil
		case "c":
			m.filters = nil
			m.filterQuery = ""
			m.filterError = ""
			m.updateTableRows()
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(msg.Height - 4)
		m.table.SetColumns(getColumns(msg.Width))

	case tickMsg:
		m.updateTableRows()
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg {
			return tickMsg{}
		})
	}

	m.table, cmd = m.table.Update(msg)

	return m, cmd
}

func (m *model) updateTableRows() {
	if m.logger == nil {
		return
	}

	var logs []*client.Log
	if len(m.filters) == 0 {
		logs = m.logger.GetLogs()
	} else {
		filterQuery := client.FormatQuery(m.filters)
		var err error
		logs, err = m.logger.GetFilteredLogs(filterQuery)
		if err != nil {
			logs = []*client.Log{}
		}
	}

	m.table.SetRows(logsToRows(logs))
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
		Bold(true).
		Foreground(lipgloss.Color("111")).
		Width(m.width).
		Align(lipgloss.Center).
		Render("Web Inspector URL: http://localhost:8000")

	// Filter input area - always single line to prevent jitter
	var filterLine string
	if m.filterMode {
		exampleText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Ex: method:GET path:/api status:>=200 |")

		// Insert cursor at current position
		queryWithCursor := m.filterQuery[:m.cursorPos] + "_" + m.filterQuery[m.cursorPos:]
		filterInput := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true).Render(" Filter: " + queryWithCursor + " ")
		helpText := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("| Enter:apply Esc:cancel")

		// Calculate padding to center the input
		totalLen := len("Ex: method:GET path:/api status:>=200 |") + len(" Filter: ") + len(queryWithCursor) + len(" ") + len("| Enter:apply Esc:cancel")
		leftPadding := (m.width - totalLen) / 2
		if leftPadding < 0 {
			leftPadding = 0
		}

		filterLine = strings.Repeat(" ", leftPadding) + exampleText + filterInput + helpText
	} else if m.filterError != "" {
		errorText := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Error: " + m.filterError)
		helpText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" | Press 'f' to retry")
		filterLine = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(errorText + helpText)
	} else {
		helpText := "f:filter"
		if len(m.filters) > 0 {
			helpText += " | Active: " + client.FormatQuery(m.filters)
		}
		helpText += " | c:clear"
		filterLine = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(m.width).Align(lipgloss.Center).Render(helpText)
	}

	requestLoggerTable := lipgloss.
		NewStyle().
		Width(m.width).
		Height(m.height - 4).
		Render(m.table.View())

	return urlLine + "\n" + webInspectorLine + "\n" + requestLoggerTable + "\n" + filterLine
}

func getColumns(width int) []table.Column {
	if width <= 0 {
		width = 80
	}

	width = width - 12 // leave room for borders/padding
	methodWidth := width * 8 / 100
	statusWidth := width * 8 / 100
	respTimeWidth := width * 18 / 100
	contentTypeWidth := width * 20 / 100
	sizeWidth := width * 10 / 100
	uriWidth := width - methodWidth - statusWidth - respTimeWidth - contentTypeWidth - sizeWidth

	return []table.Column{
		{Title: "Method", Width: methodWidth},
		{Title: "URI", Width: uriWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Content-Type", Width: contentTypeWidth},
		{Title: "Size", Width: sizeWidth},
		{Title: "Time (ms)", Width: respTimeWidth},
	}
}

func logsToRows(logs []*client.Log) []table.Row {
	var rows []table.Row

	for _, log := range logs {
		method := ""
		uri := ""
		status := ""
		contentType := ""
		size := ""
		respTime := ""

		if log.Request != nil {
			method = log.Request.Method
			uri = log.Request.Path
		}

		if log.Response != nil {
			status = fmt.Sprintf("%d", log.Response.StatusCode)

			if ct, ok := log.Response.Headers["Content-Type"]; ok {
				contentType = ct
			}

			if log.Response.Body != nil {
				size = formatSize(len(log.Response.Body))
			}
		}

		respTime = fmt.Sprintf("%d", log.Duration)
		rows = append(rows, table.Row{method, uri, status, contentType, size, respTime})
	}

	return rows
}

func formatSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

func NewModel(logger *client.Logger, appURL string) model {
	columns := getColumns(80)

	var rows []table.Row

	if logger != nil {
		rows = logsToRows(logger.GetLogs())
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
		table:  t,
		logger: logger,
		appURL: appURL,
	}
}
