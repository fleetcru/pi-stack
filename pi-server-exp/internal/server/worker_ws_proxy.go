package server

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

func (s *Server) proxyWorkerWebSocket(w http.ResponseWriter, r *http.Request, worker Worker, remotePath string) {
	remoteURL, err := workerWebSocketURL(worker.URL, remotePath)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	header := http.Header{}
	if string(worker.Token) != "" {
		header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	remote, _, err := websocket.DefaultDialer.DialContext(r.Context(), remoteURL, header)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	client, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = remote.Close()
		return
	}
	var once sync.Once
	closeBoth := func() { once.Do(func() { _ = client.Close(); _ = remote.Close() }) }
	go proxyWS(client, remote, closeBoth)
	proxyWS(remote, client, closeBoth)
}

func proxyWS(dst, src *websocket.Conn, done func()) {
	defer done()
	for {
		mt, msg, err := src.ReadMessage()
		if err != nil {
			return
		}
		if err := dst.WriteMessage(mt, msg); err != nil {
			return
		}
	}
}

func workerWebSocketURL(baseURL, remotePath string) (string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	u.Path = remotePath
	return u.String(), nil
}
