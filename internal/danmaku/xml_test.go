package danmaku

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"testing"
)

func TestDecodeXML(t *testing.T) {
	want := []byte(`<?xml version="1.0" encoding="UTF-8"?><i><d p="1,1,25,16777215">hello</d></i>`)
	for name, create := range map[string]func(io.Writer) io.WriteCloser{
		"deflate": func(w io.Writer) io.WriteCloser {
			writer, _ := flate.NewWriter(w, flate.DefaultCompression)
			return writer
		},
		"gzip": func(w io.Writer) io.WriteCloser { return gzip.NewWriter(w) },
		"zlib": func(w io.Writer) io.WriteCloser { return zlib.NewWriter(w) },
	} {
		t.Run(name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := create(&buffer)
			writer.Write(want)
			writer.Close()
			got, err := DecodeXML(buffer.Bytes())
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("decode failed: %v", err)
			}
		})
	}
	if got, err := DecodeXML(want); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("plain XML failed: %v", err)
	}
	for _, invalid := range []string{"", "not xml", "<html>blocked</html>", "<i><d>broken"} {
		if _, err := DecodeXML([]byte(invalid)); err == nil {
			t.Fatalf("accepted invalid XML: %q", invalid)
		}
	}
}
