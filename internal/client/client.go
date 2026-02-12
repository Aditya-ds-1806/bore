package client

import (
	borepb "bore/borepb"
	"bore/internal/logger"
	"bore/internal/traffik"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"resty.dev/v3"
)

var BoreServerHost string
var WSScheme string

type BoreClientConfig struct {
	UpstreamURL   string
	Traffik       *traffik.Logger
	AllowExternal bool
	DebugMode     bool
	Version       string
	NoTui         bool
}

type BoreClient struct {
	resty         *resty.Client
	wsConn        *websocket.Conn
	wsMutex       *sync.Mutex
	debugMode     bool
	logger        *zap.Logger
	Traffik       *traffik.Logger
	AppId         string
	AppURL        string
	UpstreamURL   string
	Ready         chan struct{}
	allowExternal bool
}

func (bc *BoreClient) NewWSConnection() error {
	var dialer = websocket.Dialer{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	wsConnStr := fmt.Sprintf("%s://%s/ws", WSScheme, BoreServerHost)
	bc.logger.Debug("attempting websocket connection", zap.String("url", wsConnStr))
	conn, res, err := dialer.Dial(wsConnStr, nil)

	if err != nil {
		bc.logger.Error("failed to establish websocket connection", zap.Error(err), zap.String("url", wsConnStr))
		return err
	}

	conn.SetPingHandler(func(appData string) error {
		bc.logger.Debug("received ping from server, sending pong", zap.String("appData", appData))
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})

	conn.SetCloseHandler(func(code int, text string) error {
		bc.logger.Warn("websocket connection closed by server", zap.Int("code", code), zap.String("text", text))
		return conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, ""))
	})

	var domain string = BoreServerHost

	appId := res.Header.Get("X-Bore-App-ID")
	parts := strings.Split(BoreServerHost, ".")

	if len(parts) > 2 {
		domain = strings.Join(parts[1:], ".")
	}

	bc.wsConn = conn
	bc.AppId = appId
	bc.AppURL = fmt.Sprintf("https://%s.%s", appId, domain)
	bc.Ready <- struct{}{}
	close(bc.Ready)

	bc.logger = bc.logger.With(zap.String("appId", appId))
	bc.logger.Info("websocket connection established", zap.String("appURL", bc.AppURL))

	return nil
}

func (bc *BoreClient) HandleWSMessages() error {
	defer bc.resty.Close()

	bc.logger.Info("starting to handle websocket messages")
	for {
		_, message, err := bc.wsConn.ReadMessage()
		if err != nil {
			bc.logger.Error("error reading websocket message", zap.Error(err))
			return err
		}

		var request borepb.Request

		err = proto.Unmarshal(message, &request)
		if err != nil {
			bc.logger.Error("failed to unmarshal protobuf message", zap.Error(err))
			return err
		}

		bc.logger.Debug("received request", zap.String("reqId", request.Id), zap.String("method", request.Method), zap.String("path", request.Path))

		cookies, _ := http.ParseCookie(request.Cookies)

		ctx := context.WithValue(context.TODO(), traffik.RequestIDKey, request.Id)

		req := bc.resty.
			NewRequest().
			SetContext(ctx).
			SetMethod(request.Method).
			SetURL(request.Path).
			SetBody(request.Body).
			SetCookies(cookies).
			SetHeaders(request.Headers)

		bc.Traffik.LogRequest(req)

		res, err := req.Send()
		if err != nil {
			bc.logger.Error("failed to send request", zap.String("reqId", request.Id), zap.Error(err))
			return err
		}

		bc.logger.Debug("response received", zap.String("reqId", request.Id), zap.Int("statusCode", res.StatusCode()))
		bc.Traffik.LogResponse(res)

		response := borepb.Response{
			Id:         request.Id,
			StatusCode: int32(res.StatusCode()),
			Body:       res.Bytes(),
			Timestamp:  res.ReceivedAt().UnixMilli(),
			Headers:    make(map[string]string),
		}

		for headerName, headerValues := range res.Header() {
			response.Headers[headerName] = strings.Join(headerValues, ",")
		}

		resBytes, err := proto.Marshal(&response)
		if err != nil {
			bc.logger.Error("failed to marshal response", zap.String("reqId", request.Id), zap.Error(err))
			return err
		}

		bc.wsMutex.Lock()
		err = bc.wsConn.WriteMessage(websocket.BinaryMessage, resBytes)
		bc.wsMutex.Unlock()
		if err != nil {
			bc.logger.Error("failed to write response to websocket", zap.String("reqId", request.Id), zap.Error(err))
			return err
		}
		bc.logger.Debug("response sent", zap.String("reqId", request.Id))
	}
}

func (bc *BoreClient) RegisterApp() error {
	bc.logger.Info("registering application")
	url, err := url.ParseRequestURI(bc.UpstreamURL)
	if err != nil {
		bc.logger.Error("failed to parse upstream url", zap.String("url", bc.UpstreamURL), zap.Error(err))
		return err
	}

	isRemoteUpstream := url.Hostname() != "localhost" && !strings.HasPrefix(url.Hostname(), "127.0.0.1")

	if isRemoteUpstream && !bc.allowExternal {
		err := fmt.Errorf("refusing to proxy non-localhost targets by default. Use --allow-external to override.")
		bc.logger.Error("remote upstream rejected", zap.String("host", url.Hostname()), zap.Error(err))
		return err
	}

	err = bc.NewWSConnection()
	if err != nil {
		bc.logger.Error("failed to establish websocket connection during registration", zap.Error(err))
		return err
	}

	err = bc.HandleWSMessages()
	if err != nil {
		bc.logger.Error("error handling websocket messages", zap.Error(err))
	}

	return err
}

func NewBoreClient(boreClientCfg *BoreClientConfig) *BoreClient {
	resty := resty.New().SetBaseURL(boreClientCfg.UpstreamURL)
	logFilePath := "./logs/bore-client.log"

	cfg := logger.
		NewLoggerCfg().
		WithLogFilePath(logFilePath).
		WithStdout(boreClientCfg.NoTui).
		WithLoggingEnabled(boreClientCfg.DebugMode).
		WithDevMode(strings.Contains(boreClientCfg.Version, "dev"))

	logger, err := logger.NewLogger(cfg)

	if err != nil {
		fmt.Println("Failed to create logger")
		panic(err)
	}

	logger.Info("bore client initialized", zap.String("upstreamURL", boreClientCfg.UpstreamURL), zap.Bool("debugMode", boreClientCfg.DebugMode), zap.Bool("allowExternal", boreClientCfg.AllowExternal))

	return &BoreClient{
		resty:         resty,
		UpstreamURL:   boreClientCfg.UpstreamURL,
		debugMode:     boreClientCfg.DebugMode,
		logger:        logger,
		Traffik:       boreClientCfg.Traffik,
		wsMutex:       &sync.Mutex{},
		Ready:         make(chan struct{}),
		allowExternal: boreClientCfg.AllowExternal,
	}
}
