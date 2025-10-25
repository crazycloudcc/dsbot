package nets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	DefaultTimeout  = 60 * time.Second // 默认超时，单位秒
	DefaultProxyURL = ""               // 默认代理(如果需要网络代理，可以在这里设置代理URL，比如"http://127.0.0.1:7890")
)

var (
	DefaultHeadersGet = map[string]string{
		"Accept":     "application/json",
		"User-Agent": "Go-http-client/1.1",
	}
	DefaultHeadersPost = map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"User-Agent":   "Go-http-client/1.1",
	}
)

type HttpClient struct {
	httpTimeout  time.Duration
	httpProxyURL string
	http         *http.Client
}

func NewHttpClient(timeout time.Duration, httpProxyURL string) (*HttpClient, error) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: timeout,
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if httpProxyURL != "" {
		proxyURL, err := url.Parse(httpProxyURL)
		if err != nil {
			fmt.Println("解析代理URL失败:", err)
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	c := &HttpClient{
		httpTimeout:  timeout,
		httpProxyURL: httpProxyURL,
		http:         &http.Client{Transport: transport, Timeout: timeout},
	}

	fmt.Println("创建HTTP客户端: timeout =", c.httpTimeout, "proxy =", c.httpProxyURL)

	return c, nil
}

func (c *HttpClient) SetTimeout(timeout int) {
	c.httpTimeout = time.Duration(timeout) * time.Second
	c.http.Timeout = c.httpTimeout
}

func (c *HttpClient) QueryGet(url string, headers map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// fmt.Printf("HTTP GET URL: %s\n", url)

	resp, err := c.http.Do(req)
	if err != nil {
		fmt.Println("请求错误:", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return body, nil
}

// HttpPost sends a POST request to the specified URL.
func (c *HttpClient) QueryPost(url string, headers map[string]string, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	// 设置请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		fmt.Println("请求错误:", err)
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return responseBody, nil
}

// 发送POST请求，data为map数据
func (c *HttpClient) QueryPostEx(url string, headers map[string]string, data map[string]interface{}) ([]byte, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
	}
	return c.QueryPost(url, headers, bytes)
}
