package client

import (
	borepb "bore/borepb"
	"bore/internal/client/reqlogger"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"resty.dev/v3"
)

var BoreServerHost string
var WSScheme string

type BoreClientConfig struct {
	UpstreamURL   string
	Logger        *reqlogger.Logger
	AllowExternal bool
}

type BoreClient struct {
	resty         *resty.Client
	wsConn        *websocket.Conn
	wsMutex       *sync.Mutex
	Logger        *reqlogger.Logger
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
	conn, res, err := dialer.Dial(wsConnStr, nil)

	if err == nil {
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
	}

	return err
}

func (bc *BoreClient) HandleWSMessages() error {
	defer bc.resty.Close()

	for {
		_, message, err := bc.wsConn.ReadMessage()
		if err != nil {
			return err
		}

		var request borepb.Request

		err = proto.Unmarshal(message, &request)
		if err != nil {
			return err
		}

		cookies, _ := http.ParseCookie(request.Cookies)

		ctx := context.WithValue(context.TODO(), reqlogger.RequestIDKey, request.Id)

		req := bc.resty.
			NewRequest().
			SetContext(ctx).
			SetMethod(request.Method).
			SetURL(request.Path).
			SetBody(request.Body).
			SetCookies(cookies).
			SetHeaders(request.Headers)

		bc.Logger.LogRequest(req)

		res, err := req.Send()
		if err != nil {
			return err
		}

		bc.Logger.LogResponse(res)

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
			return err
		}

		bc.wsMutex.Lock()
		err = bc.wsConn.WriteMessage(websocket.BinaryMessage, resBytes)
		bc.wsMutex.Unlock()
		if err != nil {
			return err
		}
	}
}

func (bc *BoreClient) RegisterApp() error {
	url, err := url.ParseRequestURI(bc.UpstreamURL)
	if err != nil {
		return err
	}

	isRemoteUpstream := url.Hostname() != "localhost" && !strings.HasPrefix(url.Hostname(), "127.0.0.1")

	if isRemoteUpstream && !bc.allowExternal {
		return fmt.Errorf("Refusing to proxy non-localhost targets by default. Use --allow-external to override.")
	}

	err = bc.NewWSConnection()
	if err != nil {
		return err
	}

	err = bc.HandleWSMessages()

	return err
}

func NewBoreClient(cfg *BoreClientConfig) *BoreClient {
	resty := resty.New().SetBaseURL(cfg.UpstreamURL)

	return &BoreClient{
		resty:         resty,
		UpstreamURL:   cfg.UpstreamURL,
		Logger:        cfg.Logger,
		wsMutex:       &sync.Mutex{},
		Ready:         make(chan struct{}),
		allowExternal: cfg.AllowExternal,
	}
}
