package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func decodeJSONBody[T any](body []byte, dst *T) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return map[string]json.RawMessage{}, nil
	}

	if err := json.Unmarshal(trimmed, dst); err != nil {
		return nil, err
	}

	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, err
	}

	return fields, nil
}

func parseOptionalUintField(fields map[string]json.RawMessage, key string) (bool, *uint, error) {
	raw, ok := fields[key]
	if !ok {
		return false, nil, nil
	}

	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return true, nil, nil
	}

	var value uint
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return true, nil, fmt.Errorf("%s must be an integer or null", key)
	}
	if value == 0 {
		return true, nil, fmt.Errorf("%s must be greater than 0", key)
	}

	return true, &value, nil
}
