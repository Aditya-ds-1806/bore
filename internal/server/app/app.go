package appregistry

import (
	borepb "bore/borepb"
	"context"
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
	logger             *zap.Logger
	appRegistry        *AppRegistry
	shutdownCtx        context.Context
	shutdown           context.CancelFunc
	once               sync.Once
}

func (app *App) handleMessagesFromUpstream() <-chan error {
	errCh := make(chan error, 1)

	for message := range app.fromUpstreamChan {
		mes, err := proto.Marshal(message)
		if err != nil {
			errCh <- fmt.Errorf("error marshalling message: %w", err)
			return errCh
		}

		switch message.Payload.(type) {
		case *borepb.Message_Request:
			app.WsMutex.Lock()
			err = app.WsConn.WriteMessage(websocket.BinaryMessage, mes)
			app.WsMutex.Unlock()
			if err != nil {
				errCh <- fmt.Errorf("error writing message to downstream WS connection: %w", err)
				return errCh
			}
		}
	}

	app.logger.Warn("fromUpstreamChan closed, stopping handleMessagesFromUpstream goroutine")

	return nil
}

func (app *App) handleMessagesFromDownstream() <-chan error {
	errCh := make(chan error, 1)

	for {
		var message borepb.Message

		_, mes, err := app.WsConn.ReadMessage()
		if err != nil {
			errCh <- fmt.Errorf("error reading message from downstream WS connection: %w", err)
			return errCh
		}

		err = proto.Unmarshal(mes, &message)
		if err != nil {
			errCh <- fmt.Errorf("error unmarshalling message from downstream WS connection: %w", err)
			return errCh
		}

		app.fromDownstreamChan <- &message
	}
}

func (app *App) keepDownstreamAlive() <-chan error {
	errCh := make(chan error, 1)

	pingInterval := time.Duration(10 * time.Second)
	ticker := time.NewTicker(pingInterval)

	defer ticker.Stop()

	for range ticker.C {
		app.WsMutex.Lock()
		err := app.WsConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
		app.WsMutex.Unlock()

		if err != nil {
			errCh <- fmt.Errorf("error sending ping message: %w", err)
			return errCh
		}
	}

	return errCh
}

func (app *App) WriteMessageToDownStream(message *borepb.Message) {
	app.fromUpstreamChan <- message
}

func (app *App) ReadMessagesFromDownstream() <-chan *borepb.Message {
	return app.fromDownstreamChan
}

func (app *App) Done() <-chan struct{} {
	return app.shutdownCtx.Done()
}

func (app *App) Destroy() {
	app.once.Do(func() {
		app.WsMutex.Lock()
		defer app.WsMutex.Unlock()

		app.shutdown()

		close(app.fromUpstreamChan)
		close(app.fromDownstreamChan)

		app.fromUpstreamChan = nil
		app.fromDownstreamChan = nil

		if app.WsConn != nil {
			app.WsConn.Close()
		}

		app.appRegistry.DeleteApp(app.AppId)
	})
}

func newApp(appCfg AppConfig) (*App, error) {
	appRegistry := NewAppRegistry()
	shutdownCtx, shutdown := context.WithCancel(context.Background())

	app := App{
		AppId:              appCfg.AppId,
		WsConn:             appCfg.DownstreamWSConn,
		WsMutex:            &sync.RWMutex{},
		logger:             appCfg.Logger,
		fromUpstreamChan:   make(chan *borepb.Message, 10),
		fromDownstreamChan: make(chan *borepb.Message, 10),
		appRegistry:        appRegistry,
		shutdownCtx:        shutdownCtx,
		shutdown:           shutdown,
	}

	err := appRegistry.addApp(&app)
	if err != nil {
		return nil, err
	}

	go func() {
		select {
		case err := <-app.keepDownstreamAlive():
			app.logger.Error("error in keepDownstreamAlive goroutine", zap.Error(err))
			app.Destroy()
		case <-app.shutdownCtx.Done():
			app.logger.Error("shutdown signal received, stopping keepDownstreamAlive goroutine")
		}
	}()

	go func() {
		select {
		case err := <-app.handleMessagesFromDownstream():
			app.logger.Error("error in handleMessagesFromDownstream goroutine", zap.Error(err))
			app.Destroy()
		case <-app.shutdownCtx.Done():
			app.logger.Error("shutdown signal received, stopping handleMessagesFromDownstream goroutine")
		}
	}()

	go func() {
		select {
		case err := <-app.handleMessagesFromUpstream():
			app.logger.Error("error in handleMessagesFromUpstream goroutine", zap.Error(err))
			app.Destroy()
		case <-app.shutdownCtx.Done():
			app.logger.Error("shutdown signal received, stopping handleMessagesFromUpstream goroutine")
		}
	}()

	return &app, nil
}
