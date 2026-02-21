package app

import (
	borepb "bore/borepb"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type AppConfig struct {
	AppId            string
	DownstreamWSConn *websocket.Conn
	Logger           *zap.Logger
}

type App struct {
	AppId              string
	WsConn             *websocket.Conn      // WS conn to bore client (downstream)
	WsMutex            *sync.RWMutex        // Mutex to synchronize writes to WS conn
	fromUpstreamChan   chan *borepb.Message // Channel to receive messages from upstream (nginx)
	fromDownstreamChan chan *borepb.Message // Channel to receive messages from downstream (bore client)
	errCh              chan error           // Channel to receive errors from goroutines
	logger             *zap.Logger
}

func (app *App) handleMessagesFromUpstream() error {
	for message := range app.fromUpstreamChan {
		mes, err := proto.Marshal(message)
		if err != nil {
			return err
		}

		switch message.Payload.(type) {
		case *borepb.Message_Request:
			app.WsMutex.Lock()
			err = app.WsConn.WriteMessage(websocket.BinaryMessage, mes)
			app.WsMutex.Unlock()
			if err != nil {
				return err
			}
		}
	}

	app.logger.Warn("fromUpstreamChan closed, stopping handleMessagesFromUpstream goroutine")

	return nil
}

func (app *App) handleMessagesFromDownstream() error {
	for {
		var message borepb.Message

		_, mes, err := app.WsConn.ReadMessage()
		if err != nil {
			return err
		}

		err = proto.Unmarshal(mes, &message)
		if err != nil {
			return err
		}

		select {
		case app.fromDownstreamChan <- &message:
		default:
			err := fmt.Errorf("timeout sending message to fromDownstreamChan")
			fmt.Println(err)
		}
	}
}

func (app *App) keepDownstreamAlive() error {
	pingInterval := time.Duration(10 * time.Second)
	ticker := time.NewTicker(pingInterval)

	defer ticker.Stop()

	for range ticker.C {
		app.WsMutex.Lock()
		err := app.WsConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
		app.WsMutex.Unlock()

		if err != nil {
			return err
		}
	}

	return nil
}

func (app *App) WriteMessageToDownStream(message *borepb.Message) {
	if app.fromUpstreamChan == nil {
		app.errCh <- fmt.Errorf("upstream channel is nil")
		return
	}

	app.fromUpstreamChan <- message
}

func (app *App) ReadMessagesFromDownstream() <-chan *borepb.Message {
	return app.fromDownstreamChan
}

func (app *App) destroy() {
	app.WsMutex.Lock()
	defer app.WsMutex.Unlock()

	close(app.errCh)
	close(app.fromUpstreamChan)
	close(app.fromDownstreamChan)

	app.errCh = nil
	app.fromUpstreamChan = nil
	app.fromDownstreamChan = nil

	if app.WsConn != nil {
		app.WsConn.Close()
	}

	DeleteApp(app.AppId)
}

func NewApp(appCfg AppConfig) (*App, error) {
	app := App{
		AppId:              appCfg.AppId,
		WsConn:             appCfg.DownstreamWSConn,
		WsMutex:            &sync.RWMutex{},
		logger:             appCfg.Logger,
		fromUpstreamChan:   make(chan *borepb.Message, 10),
		fromDownstreamChan: make(chan *borepb.Message, 10),
		errCh:              make(chan error, 1),
	}

	err := addApp(&app)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := <-app.errCh; err != nil {
			app.logger.Error("error in app goroutine", zap.Error(err))
			app.destroy()
		}
	}()

	go func() {
		app.errCh <- app.keepDownstreamAlive()
	}()

	go func() {
		app.errCh <- app.handleMessagesFromDownstream()
	}()

	go func() {
		app.errCh <- app.handleMessagesFromUpstream()
	}()

	return &app, nil
}
