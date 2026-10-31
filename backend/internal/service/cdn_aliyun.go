package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type aliyunCDN struct {
	key, secret, endpoint string
	client                *http.Client
}
type cdnError string

var cdnRate struct {
	sync.Mutex
	next time.Time
}

func (e cdnError) Error() string { return string(e) }
func cdnErrorCode(err error) string {
	var e cdnError
	if errors.As(err, &e) {
		return string(e)
	}
	return "ProviderUnavailable"
}
func newAliyunCDN(key, secret string) *aliyunCDN {
	return &aliyunCDN{key: key, secret: secret, endpoint: "https://cdn.aliyuncs.com/", client: &http.Client{Timeout: 20 * time.Second}}
}
func rpcEscape(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
func (a *aliyunCDN) call(ctx context.Context, action string, values url.Values, result any) error {
	cdnRate.Lock()
	wait := time.Until(cdnRate.next)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			cdnRate.Unlock()
			return ctx.Err()
		case <-timer.C:
		}
	}
	cdnRate.next = time.Now().Add(250 * time.Millisecond)
	cdnRate.Unlock()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	values.Set("Action", action)
	values.Set("Version", "2018-05-10")
	values.Set("Format", "JSON")
	values.Set("AccessKeyId", a.key)
	values.Set("SignatureMethod", "HMAC-SHA1")
	values.Set("SignatureVersion", "1.0")
	values.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	values.Set("SignatureNonce", hex.EncodeToString(nonce))
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, rpcEscape(k)+"="+rpcEscape(values.Get(k)))
	}
	mac := hmac.New(sha1.New, []byte(a.secret+"&"))
	mac.Write([]byte("POST&%2F&" + rpcEscape(strings.Join(parts, "&"))))
	values.Set("Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := a.client.Do(req)
	if err != nil {
		return cdnError("ProviderUnavailable")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return cdnError("ProviderUnavailable")
	}
	var failure struct{ Code string }
	_ = json.Unmarshal(data, &failure)
	if failure.Code != "" {
		if len(failure.Code) > 128 {
			return cdnError("ProviderError")
		}
		return cdnError(failure.Code)
	}
	if res.StatusCode != 200 {
		return cdnError("HTTP" + strconv.Itoa(res.StatusCode))
	}
	if json.Unmarshal(data, result) != nil {
		return cdnError("InvalidResponse")
	}
	return nil
}
func (a *aliyunCDN) refresh(ctx context.Context, urls []string) (string, error) {
	var result struct {
		RefreshTaskID string `json:"RefreshTaskId"`
	}
	err := a.call(ctx, "RefreshObjectCaches", url.Values{"ObjectPath": {strings.Join(urls, "\n")}, "ObjectType": {"File"}}, &result)
	if err != nil {
		return "", err
	}
	if result.RefreshTaskID == "" {
		return "", cdnError("InvalidResponse")
	}
	return result.RefreshTaskID, nil
}
func (a *aliyunCDN) complete(ctx context.Context, taskID string, urls []string) (bool, error) {
	expected := map[string]bool{}
	for _, u := range urls {
		expected[u] = false
	}
	for _, id := range strings.Split(taskID, ",") {
		for page := 1; page <= 20; page++ {
			var result struct {
				TotalCount int
				Tasks      struct {
					CDNTask []struct{ Status, ObjectPath string } `json:"CDNTask"`
				}
			}
			host, _ := url.Parse(urls[0])
			if err := a.call(ctx, "DescribeRefreshTasks", url.Values{"TaskId": {id}, "DomainName": {host.Hostname()}, "ObjectType": {"file"}, "PageSize": {"100"}, "PageNumber": {strconv.Itoa(page)}}, &result); err != nil {
				return false, err
			}
			for _, t := range result.Tasks.CDNTask {
				if _, relevant := expected[t.ObjectPath]; !relevant {
					continue
				}
				switch t.Status {
				case "Failed", "Timeout", "Canceled":
					return false, cdnError("RefreshFailed")
				case "Complete":
					if _, ok := expected[t.ObjectPath]; ok {
						expected[t.ObjectPath] = true
					}
				}
			}
			if page*100 >= result.TotalCount {
				break
			}
		}
	}
	for _, done := range expected {
		if !done {
			return false, nil
		}
	}
	return true, nil
}
