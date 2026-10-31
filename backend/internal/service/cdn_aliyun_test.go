package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

func TestCDNRPCSigningAndCompletion(t *testing.T) {
	state := "Refreshing"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.RawQuery != "" {
			t.Error("credentials must be in POST body")
		}
		_ = r.ParseForm()
		q := r.Form
		signature := q.Get("Signature")
		q.Del("Signature")
		keys := []string{}
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, k := range keys {
			parts = append(parts, rpcEscape(k)+"="+rpcEscape(q.Get(k)))
		}
		mac := hmac.New(sha1.New, []byte("secret&"))
		mac.Write([]byte("POST&%2F&" + rpcEscape(strings.Join(parts, "&"))))
		if signature != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid RPC signature")
		}
		switch q.Get("Action") {
		case "RefreshObjectCaches":
			if q.Get("ObjectType") != "File" || !strings.Contains(q.Get("ObjectPath"), "\n") {
				t.Error("expected exact URL batch")
			}
			_, _ = w.Write([]byte(`{"RefreshTaskId":"123"}`))
		case "DescribeRefreshTasks":
			if q.Get("DomainName") != "cdn.example.com" || q.Get("TaskId") != "123" {
				t.Error("query must be domain scoped")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Tasks": map[string]any{"CDNTask": []any{map[string]string{"ObjectPath": "https://cdn.example.com/i/a.png", "Status": state}, map[string]string{"ObjectPath": "https://cdn.example.com/t/a.png", "Status": state}}}})
		}
	}))
	defer server.Close()
	c := newAliyunCDN("key", "secret")
	c.endpoint = server.URL
	urls := []string{"https://cdn.example.com/i/a.png", "https://cdn.example.com/t/a.png"}
	id, err := c.refresh(context.Background(), urls)
	if err != nil || id != "123" {
		t.Fatalf("refresh: %v", err)
	}
	done, err := c.complete(context.Background(), id, urls)
	if done || err != nil {
		t.Fatal("pending refresh declared complete")
	}
	state = "Complete"
	done, err = c.complete(context.Background(), id, urls)
	if !done || err != nil {
		t.Fatalf("completion: %v", err)
	}
	state = "Failed"
	_, err = c.complete(context.Background(), id, urls)
	if cdnErrorCode(err) != "RefreshFailed" {
		t.Fatal("failed refresh must be retried")
	}
}

func TestCDNProviderErrorsAreSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"Code":"Forbidden.RAM","Message":"sensitive provider diagnostic"}`))
	}))
	defer server.Close()
	c := newAliyunCDN("key", "secret")
	c.endpoint = server.URL
	err := c.call(context.Background(), "DescribeRefreshTasks", url.Values{}, &struct{}{})
	if err == nil || err.Error() != "Forbidden.RAM" {
		t.Fatal("only provider error code may be retained")
	}
}
