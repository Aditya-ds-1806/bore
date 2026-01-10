package logger

import (
	borepb "bore/borepb"
	"bytes"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	"resty.dev/v3"
)

type RequestID string

const RequestIDKey RequestID = "bore-request-id"

type Log struct {
	RequestID string
	Request   *borepb.Request
	Response  *borepb.Response
	Duration  int64
}

type Logger struct {
	mutex sync.Mutex
	logs  map[string]*Log
}

func NewLogger() *Logger {
	return &Logger{
		logs: make(map[string]*Log),
	}
}

func (l *Logger) LogRequest(req *resty.Request) {
	requestID := req.Context().Value(RequestIDKey).(string)
	// fmt.Println("Logging request:", requestID)

	request := borepb.Request{
		Method:  req.Method,
		Path:    req.URL,
		Headers: l.flattenHeaders(req.Header),
		Body:    req.Body.([]byte),
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.logs[requestID] = &Log{
		RequestID: requestID,
		Request:   &request,
		Response:  &borepb.Response{},
	}
}

func (l *Logger) LogResponse(res *resty.Response) {
	requestID := res.Request.Context().Value(RequestIDKey).(string)
	// fmt.Println("Logging response:", requestID)

	requestTimestamp := res.Request.Time.UnixMilli()
	responseTimestamp := res.ReceivedAt().UnixMilli()

	response := borepb.Response{
		Headers:    l.flattenHeaders(res.Header()),
		StatusCode: int32(res.StatusCode()),
		Timestamp:  responseTimestamp,
	}

	if res.Body != nil {
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			// fmt.Println("Error reading response body:", err)
			return
		}

		res.Body.Close()
		res.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		response.Body = bodyBytes
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	if _, ok := l.logs[requestID]; !ok {
		// fmt.Println("No request found for response logging")
		return
	}

	l.logs[requestID].Request.Timestamp = requestTimestamp
	l.logs[requestID].Response = &response
	l.logs[requestID].Duration = responseTimestamp - requestTimestamp
}

func (l *Logger) GetLogs() []*Log {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	var allLogs []*Log
	for _, log := range l.logs {
		allLogs = append(allLogs, log)
	}

	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Request.Timestamp > allLogs[j].Request.Timestamp
	})

	return allLogs
}

func (l *Logger) flattenHeaders(headers http.Header) map[string]string {
	headersMap := make(map[string]string)

	for headerName, headerValues := range headers {
		headersMap[headerName] = strings.Join(headerValues, ",")
	}

	return headersMap
}
