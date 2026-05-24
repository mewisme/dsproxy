package jsoncanon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// Marshal produces compact JSON with sorted object keys (", " separators).
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeValue(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeValue(buf *bytes.Buffer, v any) error {
	switch value := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if value {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case float64:
		buf.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	case json.Number:
		buf.WriteString(string(value))
	case int:
		buf.WriteString(fmt.Sprintf("%d", value))
	case int64:
		buf.WriteString(fmt.Sprintf("%d", value))
	case string:
		enc, err := json.Marshal(value)
		if err != nil {
			return err
		}
		buf.Write(enc)
	case []any:
		buf.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyEnc, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(keyEnc)
			buf.WriteByte(':')
			if err := writeValue(buf, value[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		return writeValue(buf, decoded)
	}
	return nil
}
