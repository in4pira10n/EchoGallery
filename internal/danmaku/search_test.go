package danmaku

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchVideoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/search/type" || r.URL.Query().Get("keyword") != "测试 & 视频" || r.URL.Query().Get("search_type") != "video" {
			t.Errorf("unexpected search request: %s", r.URL)
		}
		fmt.Fprint(w, `{"code":0,"data":{"result":[{"bvid":"BV1SopR6wE2x","title":"<em>测试</em> &amp; 视频","author":"UP","upic":"//example.com/face.jpg","pic":"//example.com/cover.jpg","duration":"08:30"},{"bvid":"invalid"}]}}`)
	}))
	defer server.Close()
	client := NewClient(server.Client())
	client.BaseURL = server.URL
	items, err := client.Search(context.Background(), "测试 & 视频")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "测试 & 视频" || items[0].OwnerAvatar != "https://example.com/face.jpg" || items[0].Cover != "https://example.com/cover.jpg" || items[0].Duration != "08:30" {
		t.Fatalf("unexpected results: %+v", items)
	}
}

func TestResolveIncludesUploaderAvatar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/view" || r.URL.Query().Get("bvid") != "BV1SopR6wE2x" {
			t.Errorf("unexpected resolve request: %s", r.URL)
		}
		fmt.Fprint(w, `{"code":0,"data":{"bvid":"BV1SopR6wE2x","title":"video","pic":"//example.com/cover.jpg","owner":{"name":"UP","face":"//example.com/face.jpg"},"pages":[{"page":1,"cid":123,"part":"P1","duration":60}]}}`)
	}))
	defer server.Close()
	client := NewClient(server.Client())
	client.BaseURL = server.URL
	video, err := client.Resolve(context.Background(), "BV1SopR6wE2x")
	if err != nil {
		t.Fatal(err)
	}
	if video.Owner != "UP" || video.OwnerAvatar != "https://example.com/face.jpg" {
		t.Fatalf("uploader details were not resolved: %+v", video)
	}
}

func TestSearchRejectsUpstreamErrors(t *testing.T) {
	for _, body := range []string{`{"code":-412}`, `<html>Access restricted</html>`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			client := NewClient(server.Client())
			client.BaseURL = server.URL
			if _, err := client.Search(context.Background(), "test"); err == nil {
				t.Fatal("expected upstream error")
			}
		})
	}
}
