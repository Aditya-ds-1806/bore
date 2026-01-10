package client

import (
	borepb "bore/borepb"
	"bore/internal/web/logger"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"resty.dev/v3"
)

var BoreServerHost string

type BoreClientConfig struct {
	UpstreamURL string
	Logger      *logger.Logger
}

type BoreClient struct {
	resty       *resty.Client
	wsConn      *websocket.Conn
	wsMutex     *sync.Mutex
	Logger      *logger.Logger
	AppId       string
	AppURL      string
	UpstreamURL string
	Ready       chan struct{}
}

func (bc *BoreClient) NewWSConnection() error {
	var dialer = websocket.Dialer{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	wsConnStr := fmt.Sprintf("wss://%s/ws", BoreServerHost)
	conn, res, err := dialer.Dial(wsConnStr, nil)

	if err == nil {
		appId := res.Header.Get("X-Bore-App-ID")
		bc.wsConn = conn
		bc.AppId = appId
		bc.AppURL = fmt.Sprintf("https://%s.%s", appId, BoreServerHost)
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

		ctx := context.WithValue(context.TODO(), logger.RequestIDKey, request.Id)

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
	err := bc.NewWSConnection()
	if err != nil {
		return err
	}

	err = bc.HandleWSMessages()

	return err
}

func NewBoreClient(cfg *BoreClientConfig) *BoreClient {
	resty := resty.New().SetBaseURL(cfg.UpstreamURL)

	return &BoreClient{
		resty:       resty,
		UpstreamURL: cfg.UpstreamURL,
		Logger:      cfg.Logger,
		wsMutex:     &sync.Mutex{},
		Ready:       make(chan struct{}),
	}
}
