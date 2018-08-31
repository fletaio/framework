package config

import (
	"bytes"
	"io"
	"os"

	toml "github.com/pelletier/go-toml"
)

// LoadFile TODO
func LoadFile(path string, v interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return LoadReader(file, v)
}

// LoadString TODO
func LoadString(data string, v interface{}) error {
	return LoadReader(bytes.NewReader([]byte(data)), v)
}

// LoadReader TODO
func LoadReader(r io.Reader, v interface{}) error {
	dec := toml.NewDecoder(r)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
