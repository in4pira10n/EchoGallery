package danmaku

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/xml"
	"fmt"
	"io"
)

const maxXMLSize = 32 << 20

// Bilibili sometimes returns raw DEFLATE even for URLs ending in .xml.
// Decode on read as well, so earlier downloads remain usable.
func DecodeXML(data []byte) ([]byte, error) {
	validate := func(value []byte) bool {
		if len(value) == 0 || len(value) > maxXMLSize {
			return false
		}
		decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(value, []byte{0xef, 0xbb, 0xbf})))
		root := false
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return root
			}
			if err != nil {
				return false
			}
			if start, ok := token.(xml.StartElement); ok && !root {
				if start.Name.Local != "i" {
					return false
				}
				root = true
			}
		}
	}
	if validate(data) {
		return data, nil
	}
	for _, open := range []func(io.Reader) (io.ReadCloser, error){
		func(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) },
		zlib.NewReader,
		func(r io.Reader) (io.ReadCloser, error) { return flate.NewReader(r), nil },
	} {
		reader, err := open(bytes.NewReader(data))
		if err != nil {
			continue
		}
		decoded, readErr := io.ReadAll(io.LimitReader(reader, maxXMLSize+1))
		reader.Close()
		if readErr == nil && validate(decoded) {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("invalid or oversized danmaku XML")
}
