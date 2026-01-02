package client

import (
	borepb "bore/borepb"
	"bore/internal/web/logger"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"resty.dev/v3"
)

type BoreClientConfig struct {
	UpstreamURL string
	Logger      *logger.Logger
}

type BoreClient struct {
	resty       *resty.Client
	wsConn      *websocket.Conn
	wsMutex     *sync.Mutex
	Logger      *logger.Logger
	UpstreamURL string
}

func (bc *BoreClient) NewWSConnection() error {
	var dialer = websocket.Dialer{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	conn, _, err := dialer.Dial("ws://localhost:8080/ws", nil)
	bc.wsConn = conn

	return err
}

func (bc *BoreClient) HandleWSMessages() {
	for {
		messageType, message, err := bc.wsConn.ReadMessage()
		if err != nil {
			fmt.Println("Error from client:", messageType, err)
			break
		}

		var request borepb.Request

		err = proto.Unmarshal(message, &request)
		if err != nil {
			fmt.Println("Failed to unmarshal request:", err)
			return
		}

		cookies, err := http.ParseCookie(request.Cookies)
		if err != nil {
			fmt.Println("Failed to parse cookies:", err)
		}

		req := bc.resty.
			NewRequest().
			SetMethod(request.Method).
			SetURL(request.Path).
			SetBody(request.Body).
			SetCookies(cookies).
			SetHeaders(request.Headers)

		res, err := req.Send()
		if err != nil {
			fmt.Println("Error fetching data:", err)
			return
		}

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
			fmt.Println("Failed to marshal response:", err)
			return
		}

		bc.wsMutex.Lock()
		err = bc.wsConn.WriteMessage(websocket.BinaryMessage, resBytes)
		bc.wsMutex.Unlock()
		if err != nil {
			fmt.Println("Failed to write resBytes to ws:", err)
			return
		}
	}
}

func (bc *BoreClient) StartBoreClient() error {
	bc.resty = resty.New().SetBaseURL(bc.UpstreamURL)
	defer bc.resty.Close()

	err := bc.NewWSConnection()
	if err != nil {
		return err
	}

	bc.HandleWSMessages()

	return nil
}

func NewBoreClient(cfg *BoreClientConfig) *BoreClient {
	return &BoreClient{
		UpstreamURL: cfg.UpstreamURL,
		Logger:      cfg.Logger,
		wsMutex:     &sync.Mutex{},
	}
}
