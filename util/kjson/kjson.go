package kjson

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Marshal marshals json without escaping html
func Marshal(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	j := json.NewEncoder(&b)
	j.SetEscapeHTML(false)
	if err := j.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Unmarshal unmarshals json with the option UseNumber
func Unmarshal(data []byte, v interface{}) error {
	if !json.Valid(data) {
		return errors.New("Invalid json")
	}
	j := json.NewDecoder(bytes.NewReader(data))
	j.UseNumber()
	if err := j.Decode(v); err != nil {
		return err
	}
	return nil
}
