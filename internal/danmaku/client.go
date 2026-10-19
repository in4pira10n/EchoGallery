package danmaku

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

type SearchResult struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Owner       string `json:"owner"`
	OwnerAvatar string `json:"owner_avatar,omitempty"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
}

var searchTitleTags = regexp.MustCompile(`<[^>]*>`)

func (c *Client) Search(ctx context.Context, keyword string) ([]SearchResult, error) {
	params := url.Values{"search_type": {"video"}, "keyword": {keyword}, "page": {"1"}, "page_size": {"10"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/x/web-interface/search/type?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://search.bilibili.com/")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bilibili search HTTP %s", resp.Status)
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Result []struct {
				BVID     string `json:"bvid"`
				Title    string `json:"title"`
				Author   string `json:"author"`
				UPic     string `json:"upic"`
				Pic      string `json:"pic"`
				Duration string `json:"duration"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("Bilibili search API %d", payload.Code)
	}
	items := make([]SearchResult, 0, 10)
	for _, item := range payload.Data.Result {
		bvid, _, err := ParseBVID(item.BVID)
		if err != nil {
			continue
		}
		items = append(items, SearchResult{BVID: bvid, Title: html.UnescapeString(searchTitleTags.ReplaceAllString(item.Title, "")), Owner: item.Author, OwnerAvatar: normalizeHTTPSImageURL(item.UPic), Cover: normalizeHTTPSImageURL(item.Pic), Duration: item.Duration})
		if len(items) == 10 {
			break
		}
	}
	return items, nil
}

func normalizeHTTPSImageURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "//") {
		value = "https:" + value
	} else if strings.HasPrefix(strings.ToLower(value), "http://") {
		value = "https://" + value[len("http://"):]
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

func NewClient(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{HTTPClient: client, BaseURL: "https://api.bilibili.com"}
}

type viewResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		BVID  string `json:"bvid"`
		Title string `json:"title"`
		Cover string `json:"pic"`
		Owner struct {
			Name string `json:"name"`
			Face string `json:"face"`
		} `json:"owner"`
		Pages []struct {
			Page     int    `json:"page"`
			CID      int64  `json:"cid"`
			Part     string `json:"part"`
			Duration int    `json:"duration"`
		} `json:"pages"`
	} `json:"data"`
}

func (c *Client) Resolve(ctx context.Context, bvid string) (Video, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/x/web-interface/view?bvid="+bvid, nil)
	if err != nil {
		return Video{}, err
	}
	req.Header.Set("User-Agent", "EchoGallery/3.0")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Video{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Video{}, fmt.Errorf("Bilibili returned HTTP %s", resp.Status)
	}
	var result viewResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Video{}, err
	}
	if result.Code != 0 {
		return Video{}, fmt.Errorf("Bilibili API %d: %s", result.Code, result.Message)
	}
	video := Video{BVID: result.Data.BVID, Title: result.Data.Title, Owner: result.Data.Owner.Name, OwnerAvatar: normalizeHTTPSImageURL(result.Data.Owner.Face), Cover: normalizeHTTPSImageURL(result.Data.Cover), URL: "https://www.bilibili.com/video/" + bvid}
	for _, page := range result.Data.Pages {
		video.Pages = append(video.Pages, Page{Page: page.Page, CID: page.CID, Part: page.Part, Duration: page.Duration, File: "P" + strconv.Itoa(page.Page) + ".xml", Status: "pending"})
	}
	if len(video.Pages) == 0 {
		return Video{}, fmt.Errorf("video has no pages")
	}
	return video, nil
}

func (c *Client) FetchXML(ctx context.Context, cid int64) ([]byte, error) {
	url := "https://comment.bilibili.com/" + strconv.FormatInt(cid, 10) + ".xml"
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "EchoGallery/3.0")
		resp, err := c.HTTPClient.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
			resp.Body.Close()
			if readErr == nil && len(data) > 0 {
				decoded, decodeErr := DecodeXML(data)
				if decodeErr == nil {
					return decoded, nil
				}
				readErr = decodeErr
			}
			last = readErr
		} else {
			if err != nil {
				last = err
			} else {
				last = fmt.Errorf("Bilibili returned HTTP %s", resp.Status)
				resp.Body.Close()
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
		}
	}
	if last == nil {
		last = fmt.Errorf("Bilibili returned an empty danmaku response")
	}
	return nil, last
}
