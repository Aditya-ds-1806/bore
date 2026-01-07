package server

import (
	borepb "bore/borepb"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
)

type BoreServer struct {
	wsMutex      *sync.Mutex
	logger       *zap.Logger
	reqIdChanMap map[string]chan *borepb.Response
	apps         map[string]*websocket.Conn
}

func (bs *BoreServer) generateAppId() string {
	return strings.ToLower(rand.Text())
}

func (bs *BoreServer) handleApp(appId string, wsConn *websocket.Conn) {
	defer func() { delete(bs.apps, appId) }()

	if wsConn == nil {
		bs.logger.Info("no wsConn for app", zap.String("appId", appId))
		return
	}

	for {
		response := &borepb.Response{}

		_, res, err := wsConn.ReadMessage()

		if websocket.IsUnexpectedCloseError(err) {
			bs.logger.Info("ws conn closed unexpectedly", zap.Error(err))
			bs.apps[appId] = nil
			return
		}

		if err != nil {
			bs.logger.Error("failed to read response from bore client", zap.Error(err))
			return
		}

		err = proto.Unmarshal(res, response)
		if err != nil {
			bs.logger.Error("Failed to unmarshal response", zap.Error(err))
			return
		}

		bs.reqIdChanMap[response.Id] <- response
	}
}

func (bs *BoreServer) StartBoreServer() error {
	router := chi.NewRouter()

	router.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		clientIP := r.Header.Get("X-Real-IP")
		bs.logger.Info("new bore client connection request", zap.String("client_ip", clientIP))

		var upgrader = websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			bs.logger.Error("failed to upgrade connection to WS", zap.Error(err), zap.String("client_ip", clientIP))
			http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
			return
		}

		bs.logger.Info("connection upgraded to WS", zap.String("client_ip", clientIP))

		appId := bs.generateAppId()
		bs.apps[appId] = conn

		bs.logger.Info("registered app!", zap.String("app_id", appId))

		go bs.handleApp(appId, conn)
	})

	router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		requestId := uuid.New().String()
		appId := strings.Split(r.Host, ".")[0]
		clientIP := r.Header.Get("X-Real-IP")

		reqLogger := bs.logger.With(
			zap.String("req_id", requestId),
			zap.String("client_ip", clientIP),
		)

		reqLogger.Info("new incoming request", zap.String("method", r.Method), zap.String("host", r.Host), zap.String("path", r.URL.Path), zap.String("appId", appId))

		if bs.apps[appId] == nil {
			reqLogger.Error("No app found!")
			http.Error(w, "No app found!", http.StatusBadRequest)
			return
		}

		bs.reqIdChanMap[requestId] = make(chan *borepb.Response)

		hopByHopHeaders := []string{
			"Connection",
			"Keep-Alive",
			"Proxy-Authenticate",
			"Proxy-Authorization",
			"TE",
			"Transfer-Encoding",
			"Upgrade",
			"Trailer",
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			reqLogger.Error("Error reading request body", zap.Error(err))
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}

		cookies := ""
		for _, cookie := range r.Cookies() {
			cookies += cookie.String() + "; "
		}

		reqLogger.Debug("finished parsing cookies", zap.Any("cookies", cookies))

		headersParsed := make(map[string]string)
		headersParsed["X-Forwarded-For"] = clientIP
		for headerName, headerValues := range r.Header {
			if !slices.Contains(hopByHopHeaders, headerName) {
				headersParsed[headerName] = strings.Join(headerValues, ",")
			}
		}

		reqLogger.Debug("finished parsing headers", zap.Any("headers", headersParsed))

		req := &borepb.Request{
			Id:        requestId,
			Method:    r.Method,
			Path:      r.RequestURI,
			Body:      bodyBytes,
			Cookies:   cookies,
			Headers:   headersParsed,
			Timestamp: time.Now().UnixMilli(),
		}

		reqBytes, err := proto.Marshal(req)
		if err != nil {
			reqLogger.Error("failed to marshal request", zap.Error(err))
			return
		}

		bs.wsMutex.Lock()
		err = bs.apps[appId].WriteMessage(websocket.BinaryMessage, reqBytes)
		bs.wsMutex.Unlock()
		if err != nil {
			reqLogger.Error("failed to write reqBytes to ws", zap.Error(err))
			return
		}

		response := <-bs.reqIdChanMap[requestId]
		reqLogger.Info("received response", zap.Int32("status_code", response.StatusCode), zap.Any("headers", response.Headers))

		for headerName, headerValues := range response.Headers {
			w.Header().Add(headerName, headerValues)
		}

		w.WriteHeader(int(response.StatusCode))

		size, err := w.Write(response.Body)
		if err != nil {
			reqLogger.Error("failed to forward response to bore client", zap.Error(err))
			return
		}

		reqLogger.Info("response forwarded to bore client", zap.Int("res_size", size))
	})

	return http.ListenAndServe(":8080", router)
}

func NewBoreServer() *BoreServer {
	cfg := zap.NewProductionConfig()

	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.OutputPaths = []string{"logs/bore.log", "stdout"}
	cfg.ErrorOutputPaths = []string{"logs/bore.log", "stdout"}

	logger, err := cfg.Build()

	if err != nil {
		fmt.Println("Failed to initialize logger")
		panic(err)
	}

	return &BoreServer{
		wsMutex:      &sync.Mutex{},
		reqIdChanMap: make(map[string]chan *borepb.Response),
		apps:         make(map[string]*websocket.Conn),
		logger:       logger,
	}
}
