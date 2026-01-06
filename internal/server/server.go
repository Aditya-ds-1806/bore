package server

import (
	borepb "bore/borepb"
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
	wsConn       *websocket.Conn
	wsMutex      *sync.Mutex
	logger       *zap.Logger
	reqIdChanMap map[string]chan *borepb.Response
}

func (bs *BoreServer) StartBoreServer() error {
	router := chi.NewRouter()

	router.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		bs.logger.Info("new bore client connection request", zap.String("client_ip", r.RemoteAddr))

		var upgrader = websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			bs.logger.Error("failed to upgrade connection to WS", zap.Error(err), zap.String("client_ip", r.RemoteAddr))
			http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
			return
		}

		bs.wsConn = conn
		bs.logger.Info("connection upgraded to WS", zap.String("client_ip", r.RemoteAddr))
	})

	router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		requestId := uuid.New().String()

		reqLogger := bs.logger.With(
			zap.String("req_id", requestId),
			zap.String("client_ip", r.RemoteAddr),
		)

		reqLogger.Info("new incoming request", zap.String("method", r.Method), zap.String("host", r.Host), zap.String("path", r.URL.Path))

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
		headersParsed["X-Forwarded-For"] = r.RemoteAddr
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
		err = bs.wsConn.WriteMessage(websocket.BinaryMessage, reqBytes)
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

	go func() {
		bs.logger.Info("starting goroutine to read ws messages")

		for {
			response := &borepb.Response{}

			if bs.wsConn == nil {
				bs.logger.Debug("Waiting for bore client ws connection to be established")
				continue
			}

			_, res, err := bs.wsConn.ReadMessage()
			if err != nil {
				bs.logger.Error("failed to read response from bore client", zap.Error(err))
				continue
			}

			err = proto.Unmarshal(res, response)
			if err != nil {
				bs.logger.Error("Failed to unmarshal response", zap.Error(err))
				continue
			}

			bs.reqIdChanMap[response.Id] <- response
		}
	}()

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
		logger:       logger,
	}
}
