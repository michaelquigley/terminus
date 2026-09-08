package report

import (
	"encoding/json"
	"reflect"

	"github.com/michaelquigley/df/dd"
)

// Codec is the shared dd binding options for project data. it carries the
// raw-reviewer-JSON converter so the reviewer's raw output — a json.RawMessage
// field on the result DTOs — passes through unbound as its original nested
// value rather than being rendered as a byte array. disk persistence and the
// MCP adapter both bind and unbind through this so they can never diverge on
// raw reviewer content.
var Codec = &dd.Options{
	Converters: map[reflect.Type]dd.Converter{
		reflect.TypeOf(json.RawMessage{}): rawMessageConverter{},
	},
}

// rawMessageConverter passes a json.RawMessage field through dd as embedded
// JSON; dd's generic unbind would otherwise render the underlying []byte as a
// number array, and binding would need the reverse.
type rawMessageConverter struct{}

func (rawMessageConverter) ToRaw(value interface{}) (interface{}, error) {
	rm, ok := value.(json.RawMessage)
	if !ok || len(rm) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(rm, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func (rawMessageConverter) FromRaw(raw interface{}) (interface{}, error) {
	if raw == nil {
		return json.RawMessage(nil), nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// Unbind converts v into a dd map using the shared codec, ready to cross the
// MCP transport as a plain map.
func Unbind(source any) (map[string]any, error) {
	return dd.Unbind(source, Codec)
}

// UnbindJSON serializes v to JSON using the shared codec, the bytes written to
// disk and emitted as MCP text content.
func UnbindJSON(source any) ([]byte, error) {
	return dd.UnbindJSON(source, Codec)
}

// BindJSON binds raw JSON into target using the shared codec, the inverse of
// UnbindJSON used to read persisted records back.
func BindJSON(target any, raw []byte) error {
	return dd.BindJSON(target, raw, Codec)
}
