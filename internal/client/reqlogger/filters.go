package reqlogger

import (
	"fmt"
	"strconv"
	"strings"
)

type Filter struct {
	Field string
	Op    string
	Value string
}

/*
ParseQuery parses a filter query string into a list of filters
Query format:"field:value field:operator+value ..."

Example: `"method:GET path:/api status:>=200"`
*/
func ParseQuery(query string) ([]*Filter, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var filters []*Filter
	parts := strings.FieldsSeq(query)

	for part := range parts {
		if !strings.Contains(part, ":") {
			return nil, fmt.Errorf("invalid filter format: %s (expected field:value)", part)
		}

		keyValue := strings.SplitN(part, ":", 2)
		field := strings.TrimSpace(strings.ToLower(keyValue[0]))
		value := strings.TrimSpace(keyValue[1])

		switch field {
		case "method", "path", "status", "type", "content-type", "contenttype", "time", "size":
		default:
			return nil, fmt.Errorf("unknown filter field: %s", field)
		}

		if field == "content-type" || field == "contenttype" {
			field = "type"
		}

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

		switch field {
		case "time":
			parsedValue, err := parseTimeValue(value)
			if err != nil {
				return nil, fmt.Errorf("invalid time value: %s", value)
			}
			value = fmt.Sprintf("%d", parsedValue)

		case "size":
			parsedValue, err := parseSizeValue(value)
			if err != nil {
				return nil, fmt.Errorf("invalid size value: %s", value)
			}
			value = fmt.Sprintf("%d", parsedValue)

		case "status":
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

func MatchesFilter(log *Log, filter *Filter) bool {
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

func parseTimeValue(value string) (int64, error) {
	value = strings.TrimSpace(strings.ToLower(value))

	if strings.HasSuffix(value, "ms") {
		numStr := strings.TrimSuffix(value, "ms")
		return strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
	}

	if strings.HasSuffix(value, "s") {
		numStr := strings.TrimSuffix(value, "s")
		num, err := strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
		if err != nil {
			return 0, err
		}
		return num * 1000, nil
	}

	return strconv.ParseInt(value, 10, 64)
}

func parseSizeValue(value string) (int64, error) {
	value = strings.TrimSpace(strings.ToLower(value))

	if strings.HasSuffix(value, "gb") {
		numStr := strings.TrimSuffix(value, "gb")
		num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return int64(num * 1024 * 1024 * 1024), nil
	}

	if strings.HasSuffix(value, "mb") {
		numStr := strings.TrimSuffix(value, "mb")
		num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return int64(num * 1024 * 1024), nil
	}

	if strings.HasSuffix(value, "kb") {
		numStr := strings.TrimSuffix(value, "kb")
		num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err != nil {
			return 0, err
		}
		return int64(num * 1024), nil
	}

	if strings.HasSuffix(value, "b") {
		numStr := strings.TrimSuffix(value, "b")
		return strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
	}

	return strconv.ParseInt(value, 10, 64)
}
