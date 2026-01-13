package tui

import (
	"bore/internal/client/reqlogger"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	table       table.Model
	width       int
	height      int
	logger      *reqlogger.Logger
	appURL      string
	filterMode  bool
	filterQuery string
	cursorPos   int
	filterError string
	detailMode  bool
	selectedLog *reqlogger.Log
	viewport    viewport.Model
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
				m.filterError = ""
				m.filterMode = false
				m.cursorPos = 0
				m.updateTableRows()
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

		if m.detailMode {
			switch msg.String() {
			case "esc", "q":
				m.detailMode = false
				m.selectedLog = nil
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}

			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		// Normal mode keys
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "f":
			m.filterMode = !m.filterMode
			if m.filterMode {
				m.cursorPos = len(m.filterQuery)
			}
			return m, nil
		case "c":
			m.filterQuery = ""
			m.filterError = ""
			m.updateTableRows()
			return m, nil
		case "enter":
			selectedRow := m.table.SelectedRow()
			if selectedRow != nil {
				requestID := selectedRow[0]
				m.selectedLog = m.logger.GetLogByID(requestID)
				if m.selectedLog != nil {
					m.detailMode = true
					m.viewport.SetContent(m.renderLogDetails())
					m.viewport.GotoTop()
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(msg.Height - 4)
		m.table.SetColumns(getColumns(msg.Width))
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4

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
	if m.logger == nil || m.filterMode {
		return
	}

	var logs []*reqlogger.Log
	if m.filterQuery == "" {
		logs = m.logger.GetLogs()
	} else {
		var err error
		logs, err = m.logger.GetFilteredLogs(m.filterQuery)
		if err != nil {
			m.filterError = err.Error()
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

	// If in detail mode, show detail view
	if m.detailMode {
		detailView := lipgloss.
			NewStyle().
			Width(m.width).
			Height(m.height - 4).
			Render(m.viewport.View())

		helpLine := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(m.width).Align(lipgloss.Center).Render("↑/↓: scroll | esc/q: back to list")

		return urlLine + "\n" + webInspectorLine + "\n" + detailView + "\n" + helpLine
	}

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
		if m.filterQuery != "" {
			helpText += " | Active: " + m.filterQuery
		}
		helpText += " | c:clear | enter:details"
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

	width = width - 12  // leave room for borders/padding
	requestIDWidth := 0 // Hidden column for RequestID
	methodWidth := width * 8 / 100
	statusWidth := width * 8 / 100
	respTimeWidth := width * 18 / 100
	contentTypeWidth := width * 20 / 100
	sizeWidth := width * 10 / 100
	uriWidth := width - methodWidth - statusWidth - respTimeWidth - contentTypeWidth - sizeWidth

	return []table.Column{
		{Title: "", Width: requestIDWidth}, // Hidden RequestID column
		{Title: "Method", Width: methodWidth},
		{Title: "URI", Width: uriWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Content-Type", Width: contentTypeWidth},
		{Title: "Size", Width: sizeWidth},
		{Title: "Time (ms)", Width: respTimeWidth},
	}
}

func logsToRows(logs []*reqlogger.Log) []table.Row {
	var rows []table.Row

	for _, log := range logs {
		requestID := log.RequestID
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
		rows = append(rows, table.Row{requestID, method, uri, status, contentType, size, respTime})
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

func (m *model) renderLogDetails() string {
	if m.selectedLog == nil {
		return "No log selected"
	}

	log := m.selectedLog
	var content strings.Builder

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).MarginTop(1)
	subHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111")).MarginTop(1)

	// Helper function to render key-value pairs with proper wrapping
	renderKV := func(key, value string, indent int) string {
		keyWidth := 30
		maxValueWidth := max(m.width-keyWidth-indent-10, 40) // Account for borders, padding, and scrollbar

		keyRendered := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(key + ":")
		keyRendered = lipgloss.NewStyle().Width(keyWidth).Inline(true).Render(keyRendered)

		// Wrap value with proper indentation
		wrappedValue := lipgloss.NewStyle().
			Width(maxValueWidth).
			Foreground(lipgloss.Color("255")).
			Render(value)

		// Add left padding to continuation lines
		lines := strings.Split(wrappedValue, "\n")
		if len(lines) > 1 {
			for i := 1; i < len(lines); i++ {
				lines[i] = strings.Repeat(" ", indent+keyWidth+1) + lines[i]
			}
		}

		result := strings.Repeat(" ", indent) + keyRendered + " " + lines[0]
		if len(lines) > 1 {
			result += "\n" + strings.Join(lines[1:], "\n")
		}
		return result + "\n"
	}

	// Request ID
	content.WriteString(headerStyle.Render("━━━ Request Details ━━━"))
	content.WriteString("\n\n")
	content.WriteString(renderKV("Request ID", log.RequestID, 0))

	if log.Request != nil {
		req := log.Request

		// Method and Path
		content.WriteString(renderKV("Method", req.Method, 0))
		content.WriteString(renderKV("Path", req.Path, 0))

		// Timestamp
		if req.Timestamp > 0 {
			timestamp := time.UnixMilli(req.Timestamp).Format("2006-01-02 15:04:05.000")
			content.WriteString(renderKV("Timestamp", timestamp, 0))
		}

		// Request Headers
		if len(req.Headers) > 0 {
			content.WriteString("\n")
			content.WriteString(subHeaderStyle.Render("Request Headers:"))
			content.WriteString("\n")

			// Sort headers for consistent display
			headerKeys := make([]string, 0, len(req.Headers))
			for k := range req.Headers {
				headerKeys = append(headerKeys, k)
			}
			sort.Strings(headerKeys)

			for _, k := range headerKeys {
				v := req.Headers[k]
				content.WriteString(renderKV(k, v, 2))
			}
		}
	}

	if log.Response != nil {
		res := log.Response

		content.WriteString("\n")
		content.WriteString(headerStyle.Render("━━━ Response Details ━━━"))
		content.WriteString("\n\n")

		// Status Code
		statusColor := "255"
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			statusColor = "42"
		} else if res.StatusCode >= 300 && res.StatusCode < 400 {
			statusColor = "33"
		} else if res.StatusCode >= 400 && res.StatusCode < 500 {
			statusColor = "208"
		} else if res.StatusCode >= 500 {
			statusColor = "196"
		}
		statusValue := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Bold(true).Render(fmt.Sprintf("%d", res.StatusCode))
		content.WriteString(renderKV("Status Code", statusValue, 0))

		// Response Timestamp
		if res.Timestamp > 0 {
			timestamp := time.UnixMilli(res.Timestamp).Format("2006-01-02 15:04:05.000")
			content.WriteString(renderKV("Timestamp", timestamp, 0))
		}

		// Duration
		if log.Duration > 0 {
			durationStr := fmt.Sprintf("%d ms", log.Duration)
			if log.Duration >= 1000 {
				durationStr = fmt.Sprintf("%.2f s", float64(log.Duration)/1000)
			}
			content.WriteString(renderKV("Duration", durationStr, 0))
		}

		// Response Size
		if res.Body != nil {
			content.WriteString(renderKV("Body Size", formatSize(len(res.Body)), 0))
		}

		// Response Headers
		if len(res.Headers) > 0 {
			content.WriteString("\n")
			content.WriteString(subHeaderStyle.Render("Response Headers:"))
			content.WriteString("\n")

			// Sort headers for consistent display
			headerKeys := make([]string, 0, len(res.Headers))
			for k := range res.Headers {
				headerKeys = append(headerKeys, k)
			}
			sort.Strings(headerKeys)

			for _, k := range headerKeys {
				v := res.Headers[k]
				content.WriteString(renderKV(k, v, 2))
			}
		}
	}

	return content.String()
}

func NewModel(logger *reqlogger.Logger, appURL string) model {
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

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		PaddingLeft(2).
		PaddingRight(2)

	return model{
		table:    t,
		logger:   logger,
		appURL:   appURL,
		viewport: vp,
	}
}
