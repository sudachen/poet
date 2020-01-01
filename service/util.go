package service

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/spacemeshos/poet/shared"
	"io/ioutil"
	"os"
)

// ⚠️ Pass the interface by reference
func BytesToInterface(buf []byte, i interface{}) error {
	dec := gob.NewDecoder(bytes.NewReader(buf)) // Will read from network.
	return dec.Decode(i)
}

// ⚠️ Pass the interface by reference
func InterfaceToBytes(i interface{}) ([]byte, error) {
	var w bytes.Buffer
	enc := gob.NewEncoder(&w)
	err := enc.Encode(i)
	return w.Bytes(), err
}

func persist(filename string, v interface{}) error {
	w, err := InterfaceToBytes(v)
	if err != nil {
		return fmt.Errorf("serialization failure: %v", err)
	}

	err = ioutil.WriteFile(filename, w, shared.OwnerReadWrite)
	if err != nil {
		return fmt.Errorf("write to disk failure: %v", err)
	}

	return nil

}

func load(filename string, v interface{}) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file is missing: %v", filename)
		}

		return fmt.Errorf("failed to read file: %v", err)
	}

	err = BytesToInterface(data, v)
	if err != nil {
		return fmt.Errorf("failed to deserialize: %v", err)
	}

	return nil
}
