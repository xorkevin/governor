package kjson

import (
	"bytes"
	"encoding/json"
)

// Marshal marshals json without escaping html
func Marshal(v interface{}) ([]byte, error) {
	b := bytes.Buffer{}
	j := json.NewEncoder(&b)
	j.SetEscapeHTML(false)
	if err := j.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Unmarshal is [encoding/json.Unmarshal]
func Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
