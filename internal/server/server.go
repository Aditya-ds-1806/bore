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
	"google.golang.org/protobuf/proto"
)

type BoreServer struct {
	wsConn       *websocket.Conn
	wsMutex      *sync.Mutex
	reqIdChanMap map[string]chan *borepb.Response
}

func (bs *BoreServer) StartBoreServer() error {
	router := chi.NewRouter()

	router.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		var upgrader = websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error from server:", err)
			http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
			return
		}

		bs.wsConn = conn
	})

	router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		requestId := uuid.New().String()
		bs.reqIdChanMap[requestId] = make(chan *borepb.Response)

		fmt.Println(r.Method, r.Host, r.URL.Path, "from", r.RemoteAddr)

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
			fmt.Println("Error reading request body:", err)
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}

		cookies := ""
		for _, cookie := range r.Cookies() {
			cookies += cookie.String() + "; "
		}

		headersParsed := make(map[string]string)
		headersParsed["X-Forwarded-For"] = r.RemoteAddr
		for headerName, headerValues := range r.Header {
			if !slices.Contains(hopByHopHeaders, headerName) {
				headersParsed[headerName] = strings.Join(headerValues, ",")
			}
		}

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
			fmt.Println("Failed to marshal request:", err)
			return
		}

		bs.wsMutex.Lock()
		err = bs.wsConn.WriteMessage(websocket.BinaryMessage, reqBytes)
		bs.wsMutex.Unlock()
		if err != nil {
			fmt.Println("Failed to write reqBytes to ws:", err)
			return
		}

		response := <-bs.reqIdChanMap[requestId]

		for headerName, headerValues := range response.Headers {
			w.Header().Add(headerName, headerValues)
		}

		w.WriteHeader(int(response.StatusCode))

		size, err := w.Write(response.Body)
		if err != nil {
			fmt.Println("Failed to send response:", err)
			return
		}

		fmt.Println(size, "bytes send to client")
	})

	go func() {
		for {
			response := &borepb.Response{}

			if bs.wsConn == nil {
				continue
			}

			_, res, err := bs.wsConn.ReadMessage()
			if err != nil {
				fmt.Println("Failed to read response from bore client:", err)
				return
			}

			err = proto.Unmarshal(res, response)
			if err != nil {
				fmt.Println("Failed to unmarshal response", err)
				return
			}

			bs.reqIdChanMap[response.Id] <- response
		}
	}()

	return http.ListenAndServe(":8080", router)
}

func NewBoreServer() *BoreServer {
	return &BoreServer{
		wsMutex:      &sync.Mutex{},
		reqIdChanMap: make(map[string]chan *borepb.Response),
	}
}
