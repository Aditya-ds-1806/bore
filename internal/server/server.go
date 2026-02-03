package server

import (
	borepb "bore/borepb"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	haikunator "github.com/atrox/haikunatorgo/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
)

const maxRetries int = 10

type App struct {
	wsConn  *websocket.Conn
	wsMutex *sync.Mutex
}

type BoreServer struct {
	logger       *zap.Logger
	reqIdChanMap map[string]chan *borepb.Response
	apps         map[string]App
	haikunator   *haikunator.Haikunator
	port         int
}

type BoreServerCfg struct {
	Port int
}

func (bs *BoreServer) generateAppId() string {
	return bs.haikunator.Haikunate()
}

func (bs *BoreServer) handleApp(appId string) {
	defer func() {
		delete(bs.apps, appId)
		bs.logger.Info("cleaned up resources for app", zap.String("app_id", appId))
	}()

	app, ok := bs.apps[appId]
	if !ok {
		bs.logger.Error("No App found!")
		return
	}

	if app.wsConn == nil {
		bs.logger.Info("no wsConn for app", zap.String("app_id", appId))
		return
	}

	go bs.ping(&app)

	for {
		response := &borepb.Response{}

		_, res, err := app.wsConn.ReadMessage()

		if websocket.IsUnexpectedCloseError(err) {
			bs.logger.Info("ws conn closed unexpectedly", zap.Error(err))
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

func (bs *BoreServer) ping(app *App) {
	pingInterval := time.Duration(10 * time.Second)
	ticker := time.NewTicker(pingInterval)

	defer ticker.Stop()

	for range ticker.C {
		app.wsMutex.Lock()
		err := app.wsConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
		app.wsMutex.Unlock()

		if err != nil {
			bs.logger.Error("failed to send ping!", zap.Error(err))
			return
		}
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

		appId := bs.generateAppId()

		conn, err := upgrader.Upgrade(w, r, http.Header{
			"X-Bore-App-ID": {appId},
		})

		if err != nil {
			bs.logger.Error("failed to upgrade connection to WS", zap.Error(err), zap.String("client_ip", clientIP))
			http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
			return
		}

		bs.logger.Info("connection upgraded to WS", zap.String("client_ip", clientIP))

		bs.apps[appId] = App{
			wsConn:  conn,
			wsMutex: &sync.Mutex{},
		}
		bs.logger.Info("registered app!", zap.String("app_id", appId))

		go bs.handleApp(appId)
	})

	router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		requestId := uuid.New().String()
		appId := strings.Split(r.Host, ".")[0]
		clientIP := r.Header.Get("X-Real-IP")

		defer func() {
			delete(bs.reqIdChanMap, requestId)
			bs.logger.Info("cleaned up resources for request", zap.String("req_id", requestId))
		}()

		reqLogger := bs.logger.With(
			zap.String("req_id", requestId),
			zap.String("client_ip", clientIP),
		)

		reqLogger.Info("new incoming request", zap.String("method", r.Method), zap.String("host", r.Host), zap.String("path", r.URL.Path), zap.String("app_id", appId))

		app, ok := bs.apps[appId]
		if !ok {
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

		app.wsMutex.Lock()
		err = app.wsConn.WriteMessage(websocket.BinaryMessage, reqBytes)
		app.wsMutex.Unlock()
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

	for range maxRetries {
		netListener, err := net.Listen("tcp", fmt.Sprintf(":%d", bs.port))
		if err == nil {
			bs.logger.Info(fmt.Sprintf("Bore server is running on http://localhost:%d/", bs.port))
			return http.Serve(netListener, router)
		}

		bs.port++
	}

	return fmt.Errorf("failed to start bore server after %d retries", maxRetries)
}

func NewBoreServer(boreCfg *BoreServerCfg) *BoreServer {
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

	h := haikunator.New()
	h.TokenLength = 5
	h.TokenChars = "abcdefghijklmnopqrstuvwxyz0123456789"

	return &BoreServer{
		reqIdChanMap: make(map[string]chan *borepb.Response),
		apps:         make(map[string]App),
		haikunator:   h,
		logger:       logger,
		port:         boreCfg.Port,
	}
}
