package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"bore/internal/ui/logger"
)

type model struct {
	table       table.Model
	width       int
	height      int
	getLogs     func() []*logger.Log
	appURL      string
	filterMode  bool
	filterQuery string
	filterError string
	filters     []*Filter
}

type Filter struct {
	Field string // column name: method, path, status, type, time, size
	Op    string // comparison operator: =, >, <, >=, <=
	Value string // the value to compare against
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
				m.filterError = ""
				return m, nil
			case "enter":
				filters, err := parseFilterQuery(m.filterQuery)
				if err != nil {
					m.filterError = err.Error()
				} else {
					m.filters = filters
					m.filterError = ""
					m.filterMode = false
					m.updateTableRows()
				}
				return m, nil
			case "backspace":
				if len(m.filterQuery) > 0 {
					m.filterQuery = m.filterQuery[:len(m.filterQuery)-1]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.filterQuery += msg.String()
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
			if m.filterMode && len(m.filters) > 0 {
				m.filterQuery = formatFilterQuery(m.filters)
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
	if m.getLogs == nil {
		return
	}

	logs := m.getLogs()
	if len(m.filters) == 0 {
		m.table.SetRows(logsToRows(logs))
		return
	}

	// Apply all filters
	filteredLogs := []*logger.Log{}
	for _, log := range logs {
		match := true
		for _, filter := range m.filters {
			if !matchesFilter(log, filter) {
				match = false
				break
			}
		}
		if match {
			filteredLogs = append(filteredLogs, log)
		}
	}
	m.table.SetRows(logsToRows(filteredLogs))
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

	// Filter input area - always single line to prevent jitter
	var filterText string
	if m.filterMode {
		filterInput := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render("Filter: " + m.filterQuery + "_")
		helpText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" | Enter:apply Esc:cancel | Ex: method:GET path:/api status:200 type:json time:<200")
		filterText = filterInput + helpText
	} else if m.filterError != "" {
		errorText := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Error: " + m.filterError)
		helpText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" | Press 'f' to retry")
		filterText = errorText + helpText
	} else {
		helpText := "f:filter"
		if len(m.filters) > 0 {
			helpText += " | Active: " + formatFilterQuery(m.filters)
		}
		helpText += " | c:clear"
		filterText = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(helpText)
	}

	filterLine := lipgloss.
		NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(filterText)

	requestLoggerTable := lipgloss.
		NewStyle().
		Width(m.width).
		Height(m.height - 4).
		Render(m.table.View())

	return urlLine + "\n" + webInspectorLine + "\n" + filterLine + "\n" + requestLoggerTable
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

func logsToRows(logs []*logger.Log) []table.Row {
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

func parseFilterQuery(query string) ([]*Filter, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var filters []*Filter
	parts := strings.Fields(query) // Split by whitespace

	for _, part := range parts {
		if !strings.Contains(part, ":") {
			return nil, fmt.Errorf("invalid filter format: %s (expected field:value)", part)
		}

		keyValue := strings.SplitN(part, ":", 2)
		field := strings.TrimSpace(strings.ToLower(keyValue[0]))
		value := strings.TrimSpace(keyValue[1])

		// Validate field
		switch field {
		case "method", "path", "status", "type", "content-type", "contenttype", "time", "size":
			// Valid field
		default:
			return nil, fmt.Errorf("unknown filter field: %s", field)
		}

		// Normalize field names
		if field == "content-type" || field == "contenttype" {
			field = "type"
		}

		// Parse operator and value
		op := "="
		if strings.HasPrefix(value, ">=") {
			op = ">="
			value = strings.TrimSpace(value[2:])
		} else if strings.HasPrefix(value, "<=") {
			op = "<="
			value = strings.TrimSpace(value[2:])
		} else if strings.HasPrefix(value, ">") {
			op = ">"
			value = strings.TrimSpace(value[1:])
		} else if strings.HasPrefix(value, "<") {
			op = "<"
			value = strings.TrimSpace(value[1:])
		}

		// Handle units for time and size fields
		if field == "time" {
			parsedValue, err := parseTimeValue(value)
			if err != nil {
				return nil, fmt.Errorf("invalid time value: %s", value)
			}
			value = fmt.Sprintf("%d", parsedValue)
		} else if field == "size" {
			parsedValue, err := parseSizeValue(value)
			if err != nil {
				return nil, fmt.Errorf("invalid size value: %s", value)
			}
			value = fmt.Sprintf("%d", parsedValue)
		} else if field == "status" {
			// Validate status value
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				return nil, fmt.Errorf("invalid status value: %s", value)
			}
		}

		if field == "method" {
			value = strings.ToUpper(value)
		}

		filters = append(filters, &Filter{
			Field: field,
			Op:    op,
			Value: value,
		})
	}

	return filters, nil
}

func matchesFilter(log *logger.Log, filter *Filter) bool {
	switch filter.Field {
	case "method":
		if log.Request == nil {
			return false
		}
		return compareString(log.Request.Method, filter.Op, filter.Value)

	case "path":
		if log.Request == nil {
			return false
		}
		return compareString(log.Request.Path, filter.Op, filter.Value)

	case "status":
		if log.Response == nil {
			return false
		}
		return compareInt(int64(log.Response.StatusCode), filter.Op, filter.Value)

	case "type":
		if log.Response == nil {
			return false
		}
		ct, ok := log.Response.Headers["Content-Type"]
		if !ok {
			return false
		}
		return compareString(ct, filter.Op, filter.Value)

	case "time":
		return compareInt(log.Duration, filter.Op, filter.Value)

	case "size":
		if log.Response == nil || log.Response.Body == nil {
			return false
		}
		return compareInt(int64(len(log.Response.Body)), filter.Op, filter.Value)
	}

	return false
}

func compareString(actual, op, expected string) bool {
	switch op {
	case "=":
		// For strings, do case-insensitive substring match
		return strings.Contains(strings.ToLower(actual), strings.ToLower(expected))
	default:
		return false
	}
}

func compareInt(actual int64, op, expectedStr string) bool {
	expected, err := strconv.ParseInt(expectedStr, 10, 64)
	if err != nil {
		return false
	}

	switch op {
	case "=":
		return actual == expected
	case ">":
		return actual > expected
	case "<":
		return actual < expected
	case ">=":
		return actual >= expected
	case "<=":
		return actual <= expected
	default:
		return false
	}
}

func formatFilterQuery(filters []*Filter) string {
	if len(filters) == 0 {
		return ""
	}

	parts := []string{}
	for _, filter := range filters {
		if filter.Op == "=" {
			parts = append(parts, fmt.Sprintf("%s:%s", filter.Field, filter.Value))
		} else {
			parts = append(parts, fmt.Sprintf("%s:%s%s", filter.Field, filter.Op, filter.Value))
		}
	}

	return strings.Join(parts, " ")
}

func parseTimeValue(value string) (int64, error) {
	value = strings.TrimSpace(strings.ToLower(value))

	// Check for unit suffix
	if strings.HasSuffix(value, "ms") {
		numStr := strings.TrimSuffix(value, "ms")
		return strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
	} else if strings.HasSuffix(value, "s") {
		numStr := strings.TrimSuffix(value, "s")
		num, err := strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
		if err != nil {
			return 0, err
		}
		return num * 1000, nil // Convert seconds to milliseconds
	}

	// No unit provided, assume milliseconds
	return strconv.ParseInt(value, 10, 64)
}

func parseSizeValue(value string) (int64, error) {
	value = strings.TrimSpace(strings.ToLower(value))

	// Check for unit suffix
	if strings.HasSuffix(value, "gb") {
		numStr := strings.TrimSuffix(value, "gb")
		num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return int64(num * 1024 * 1024 * 1024), nil
	} else if strings.HasSuffix(value, "mb") {
		numStr := strings.TrimSuffix(value, "mb")
		num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return int64(num * 1024 * 1024), nil
	} else if strings.HasSuffix(value, "kb") {
		numStr := strings.TrimSuffix(value, "kb")
		num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return int64(num * 1024), nil
	} else if strings.HasSuffix(value, "b") {
		numStr := strings.TrimSuffix(value, "b")
		return strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
	}

	// No unit provided, assume bytes
	return strconv.ParseInt(value, 10, 64)
}
