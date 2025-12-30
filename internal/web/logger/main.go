package logger

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"resty.dev/v3"
)

type RequestID string

const RequestIDKey RequestID = "bore-request-id"

type Request struct {
	Method      string
	URLPath     string
	QueryParams map[string][]string
	Headers     map[string][]string
	Body        []byte
}

type Response struct {
	Headers    map[string][]string
	Body       []byte
	StatusCode int
}

type Log struct {
	RequestID string
	Request   Request
	Response  Response
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
	fmt.Println("Logging request:", requestID)

	request := Request{
		Method:      req.Method,
		URLPath:     req.URL,
		QueryParams: req.QueryParams,
		Headers:     req.Header,
	}

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body.(io.ReadCloser))
		if err != nil {
			fmt.Println("Error reading body:", err)
			return
		}

		request.Body = bodyBytes
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.logs[requestID] = &Log{
		RequestID: requestID,
		Request:   request,
	}
}

func (l *Logger) LogResponse(res *resty.Response) {
	requestID := res.Request.Context().Value(RequestIDKey).(string)
	fmt.Println("Logging response:", requestID)

	response := Response{
		Headers:    res.Header(),
		StatusCode: res.StatusCode(),
	}

	if res.Body != nil {
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			fmt.Println("Error reading response body:", err)
			return
		}

		res.Body.Close()
		res.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		response.Body = bodyBytes
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	if _, ok := l.logs[requestID]; !ok {
		fmt.Println("No request found for response logging")
		return
	}

	l.logs[requestID].Response = response
}
