package server

import (
	borepb "bore/borepb"
	"bore/internal/logger"
	"bore/internal/server/app"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	haikunator "github.com/atrox/haikunatorgo/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const maxRetries int = 10

type BoreServer struct {
	logger               *zap.Logger
	reqIdResponseMap     map[string]chan *borepb.Response
	appRegistry          *app.AppRegistry
	haikunator           *haikunator.Haikunator
	port                 int
	messageSubscriptions map[string]chan *borepb.Message
}

type BoreServerCfg struct {
	Port    int
	LogFile string
	Version string
}

func (bs *BoreServer) generateAppId() string {
	return bs.haikunator.Haikunate()
}

func (bs *BoreServer) StartBoreServer() error {
	router := chi.NewRouter()

	router.Get("/register", func(w http.ResponseWriter, r *http.Request) {
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

		appCfg := app.AppConfig{
			AppId:            appId,
			DownstreamWSConn: conn,
			Logger:           bs.logger.With(zap.String("app_id", appId)),
		}

		app, err := app.NewApp(appCfg)
		if err != nil {
			bs.logger.Error("failed to register app", zap.Error(err), zap.String("app_id", appId))
			http.Error(w, "Failed to register app", http.StatusInternalServerError)
			return
		}

		go func() {
			for msg := range app.ReadMessagesFromDownstream() {
				msgId := msg.GetMessageId()
				if subChan, ok := bs.messageSubscriptions[msgId]; ok {
					subChan <- msg
					close(subChan)
				}
			}
		}()

		bs.logger.Info("registered app!", zap.String("app_id", appId))
	})

	router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		messageId := uuid.New().String()
		appId := strings.Split(r.Host, ".")[0]
		clientIP := r.Header.Get("X-Real-IP")

		defer func() {
			delete(bs.messageSubscriptions, messageId)
		}()

		reqLogger := bs.logger.With(
			zap.String("req_id", messageId),
			zap.String("client_ip", clientIP),
			zap.String("app_id", appId),
		)

		reqLogger.Info("new incoming request", zap.String("method", r.Method), zap.String("host", r.Host), zap.String("path", r.URL.Path))

		app, ok := app.GetApp(appId)
		if !ok {
			reqLogger.Error("No app found!")
			http.Error(w, "No app found!", http.StatusBadRequest)
			return
		}

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

		request := borepb.Request{
			Method:    r.Method,
			Path:      r.RequestURI,
			Body:      bodyBytes,
			Cookies:   cookies,
			Headers:   headersParsed,
			Timestamp: time.Now().UnixMilli(),
		}

		message := borepb.Message{
			MessageId: messageId,
			Payload:   &borepb.Message_Request{Request: &request},
		}

		subCh := make(chan *borepb.Message, 1)
		bs.messageSubscriptions[messageId] = subCh
		app.WriteMessageToDownStream(&message)
		res := <-subCh

		response := res.GetResponse()
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
	cfg := logger.
		NewLoggerCfg().
		WithLogFilePath(boreCfg.LogFile).
		WithDevMode(strings.Contains(boreCfg.Version, "dev"))

	logger, err := logger.NewLogger(cfg)

	if err != nil {
		fmt.Println("Failed to initialize logger")
		panic(err)
	}

	h := haikunator.New()
	h.TokenLength = 5
	h.TokenChars = "abcdefghijklmnopqrstuvwxyz0123456789"

	return &BoreServer{
		reqIdResponseMap:     make(map[string]chan *borepb.Response),
		appRegistry:          app.Registry,
		haikunator:           h,
		logger:               logger,
		port:                 boreCfg.Port,
		messageSubscriptions: make(map[string]chan *borepb.Message),
	}
}
