package api

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"strings"

	"echogallery/internal/service"
)

func writeZipResponse(ctx context.Context, w io.Writer, entries []service.DownloadEntry) error {
	zw := zip.NewWriter(w)

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			_ = zw.Close()
			return err
		}
		writer, err := zw.Create(entry.FileName)
		if err != nil {
			_ = zw.Close()
			return err
		}
		file, err := os.Open(entry.Path)
		if err != nil {
			_ = zw.Close()
			return err
		}
		_, copyErr := copyWithContext(ctx, writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			_ = zw.Close()
			return copyErr
		}
		if closeErr != nil {
			_ = zw.Close()
			return closeErr
		}
	}

	return zw.Close()
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 128*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			if err := ctx.Err(); err != nil {
				return written, err
			}
			nw, ew := dst.Write(buf[:nr])
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}

func sanitizeZipName(name string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "", "<", "-", ">", "-", "|", "-")
	cleaned := strings.TrimSpace(replacer.Replace(name))
	if cleaned == "" {
		cleaned = "album"
	}
	return cleaned + ".zip"
}
