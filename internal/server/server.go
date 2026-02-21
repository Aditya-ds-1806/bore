package server

import (
	borepb "bore/borepb"
	"bore/internal/logger"
	appregistry "bore/internal/server/app"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const maxRetries int = 10

type BoreServer struct {
	logger               *zap.Logger
	port                 int
	mu                   sync.Mutex
	messageSubscriptions map[string]chan *borepb.Message
	appRegistry          *appregistry.AppRegistry
}

type BoreServerCfg struct {
	Port    int
	LogFile string
	Version string
}

func (bs *BoreServer) newSubCh(messageId string) chan *borepb.Message {
	ch := make(chan *borepb.Message, 1)
	bs.mu.Lock()
	bs.messageSubscriptions[messageId] = ch
	bs.mu.Unlock()

	return ch
}

func (bs *BoreServer) GetSubCh(messageId string) (chan *borepb.Message, bool) {
	bs.mu.Lock()
	ch, ok := bs.messageSubscriptions[messageId]
	bs.mu.Unlock()

	return ch, ok
}

func (bs *BoreServer) RegisterApp(appId string, conn *websocket.Conn) (*appregistry.App, error) {
	logger := bs.logger.With(zap.String("app_id", appId))

	appCfg := appregistry.AppConfig{
		AppId:            appId,
		Logger:           logger,
		DownstreamWSConn: conn,
	}

	app, err := bs.appRegistry.NewApp(appCfg)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case msg := <-app.ReadMessagesFromDownstream():
				msgId := msg.GetMessageId()
				if subChan, ok := bs.GetSubCh(msgId); ok {
					subChan <- msg
					close(subChan)
				} else {
					logger.Warn("no subscription channel found for message", zap.String("message_id", msgId))
				}
			case <-app.Done():
				logger.Info("app shutdown signal received, stopping ReadMessagesFromDownstream goroutine")
				return
			}
		}
	}()

	logger.Info("registered app!", zap.String("app_id", appId))

	return app, nil
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

		appId := bs.appRegistry.NewAppId()

		conn, err := upgrader.Upgrade(w, r, http.Header{
			"X-Bore-App-ID": {appId},
		})

		if err != nil {
			bs.logger.Error("failed to upgrade connection to WS", zap.Error(err), zap.String("client_ip", clientIP))
			http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
			return
		}

		bs.logger.Info("connection upgraded to WS", zap.String("client_ip", clientIP))

		if _, err = bs.RegisterApp(appId, conn); err != nil {
			bs.logger.Error("failed to register app", zap.Error(err), zap.String("app_id", appId))
			http.Error(w, "Failed to register app", http.StatusInternalServerError)
			return
		}
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

		app, ok := bs.appRegistry.GetApp(appId)
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

		subCh := bs.newSubCh(messageId)

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

	return &BoreServer{
		logger:               logger,
		port:                 boreCfg.Port,
		mu:                   sync.Mutex{},
		messageSubscriptions: make(map[string]chan *borepb.Message),
		appRegistry:          appregistry.NewAppRegistry(),
	}
}
