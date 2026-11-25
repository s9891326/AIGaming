package ws

import (
	"fmt"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	gamev1 "aigaming/server/internal/pb/game/v1"
)

// Format 是一條連線使用的訊息編碼，由連線網址的 ?format= 決定。
type Format string

const (
	// FormatProtobuf：binary frame + protobuf（預設，正式前端使用）。
	FormatProtobuf Format = "protobuf"
	// FormatJSON：text frame + protojson（除錯用，方便 Postman 等工具直接打 JSON）。
	FormatJSON Format = "json"
)

// ParseFormat 解析 ?format= 的值；空字串視為 protobuf。
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case "", FormatProtobuf:
		return FormatProtobuf, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unsupported format %q (want protobuf or json)", s)
	}
}

var (
	// 欄位名稱用 lowerCamelCase（與前端 protobuf-es 的 toJson 相同），並輸出預設值，
	// 例如座標 0、seq 0 也會出現，人工閱讀時比較不會誤會「欄位不見了」。
	jsonMarshal   = protojson.MarshalOptions{EmitUnpopulated: true}
	jsonUnmarshal = protojson.UnmarshalOptions{}
)

// frameType 回傳這種編碼使用的 WebSocket frame 類型。
func (f Format) frameType() int {
	if f == FormatJSON {
		return websocket.TextMessage
	}
	return websocket.BinaryMessage
}

func (f Format) encode(msg *gamev1.ServerMessage) ([]byte, error) {
	if f == FormatJSON {
		return jsonMarshal.Marshal(msg)
	}
	return proto.Marshal(msg)
}

func (f Format) decode(data []byte, msg *gamev1.ClientMessage) error {
	if f == FormatJSON {
		return jsonUnmarshal.Unmarshal(data, msg)
	}
	return proto.Unmarshal(data, msg)
}
