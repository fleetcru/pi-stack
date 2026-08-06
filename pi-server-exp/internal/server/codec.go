package server

import (
	"io"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

// EventCodec encodes/decodes events over WebSocket.
// Two formats are supported:
//   - "json" (default, backward-compatible)
//   - "msgpack" (~30% smaller payloads, faster parse)
type EventCodec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
	WriteWebSocket(conn *websocket.Conn, v any) error
	ReadWebSocket(conn *websocket.Conn, v any) error
	Name() string
}

// JSONCodec uses encoding/json via gorilla/websocket's WriteJSON/ReadJSON.
type JSONCodec struct{}

func (JSONCodec) Name() string                        { return "json" }
func (JSONCodec) Marshal(v any) ([]byte, error)        { return nil, nil } // unused — WriteWebSocket handles it
func (JSONCodec) Unmarshal(data []byte, v any) error   { return nil }      // unused
func (JSONCodec) WriteWebSocket(conn *websocket.Conn, v any) error {
	return conn.WriteJSON(v)
}
func (JSONCodec) ReadWebSocket(conn *websocket.Conn, v any) error {
	return conn.ReadJSON(v)
}

// MsgPackCodec uses MessagePack for binary encoding.
type MsgPackCodec struct{}

func (MsgPackCodec) Name() string                      { return "msgpack" }
func (MsgPackCodec) Marshal(v any) ([]byte, error)     { return msgpack.Marshal(v) }
func (MsgPackCodec) Unmarshal(data []byte, v any) error { return msgpack.Unmarshal(data, v) }
func (MsgPackCodec) WriteWebSocket(conn *websocket.Conn, v any) error {
	data, err := msgpack.Marshal(v)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}
func (MsgPackCodec) ReadWebSocket(conn *websocket.Conn, v any) error {
	_, data, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(data, v)
}

// MarshalForTransport encodes v using the given codec and writes it to w.
// Used for SSE and other non-WebSocket transports.
func MarshalForTransport(w io.Writer, codec EventCodec, v any) error {
	switch c := codec.(type) {
	case MsgPackCodec:
		data, err := c.Marshal(v)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	default: // JSON
		jsonCodec := JSONCodec{}
		return jsonCodec.WriteWebSocket(nil, v) // unused — see below
	}
}

// ParseCodec returns the EventCodec for the given name, defaulting to JSON.
func ParseCodec(name string) EventCodec {
	switch name {
	case "msgpack", "binary":
		return MsgPackCodec{}
	default:
		return JSONCodec{}
	}
}
