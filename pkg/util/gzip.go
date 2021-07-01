package util

import (
	"bytes"
	"compress/gzip"
	"fmt"
)

func GZipCompress(input []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if n, err := zw.Write(input); err != nil {
		return nil, err
	} else if n != len(input) {
		return nil, fmt.Errorf("wrong num bytes")
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
