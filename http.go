package cex

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// 全局复用 Transport（连接池核心），避免每次创建新连接
var sharedTransport = &http.Transport{
	MaxIdleConns:        32, // 最大空闲连接数（默认2）
	MaxIdleConnsPerHost: 8,
	IdleConnTimeout:     90 * time.Second, // 空闲连接超时时间（默认无）
	MaxConnsPerHost:     0,                // 每个主机最大并发连接数
	TLSHandshakeTimeout: 5 * time.Second,  // TLS握手超时
	DisableCompression:  false,            // 启用gzip压缩（节省带宽）
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,  // 拨号超时
		KeepAlive: 30 * time.Second, // TCP保活时间
	}).DialContext,
}
var sharedClient = &http.Client{
	Transport: sharedTransport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 { // 限制重定向次数（默认10次，避免无限重定向）
			return errors.New("too many redirects (max 5)")
		}
		return nil
	},
}

type Http struct {
	client *http.Client
}

func NewClientWithLocalIP(localIP string) (*http.Client, error) {
	client := sharedClient
	if localIP != "" {
		ipAddr := net.ParseIP(localIP)
		if ipAddr == nil {
			return nil, errors.New("invalid local ip address")
		}
		localAddr := &net.TCPAddr{IP: ipAddr, Port: 0}
		tr := &http.Transport{
			MaxIdleConns:        4,                // 全局最大空闲连接
			MaxIdleConnsPerHost: 2,                // 单个域名最大空闲连接
			MaxConnsPerHost:     0,                // 单域名最大并发连接，0=无限制，少量场景无所谓
			IdleConnTimeout:     30 * time.Second, // 空闲连接快速回收，避免长占句柄
			TLSHandshakeTimeout: 3 * time.Second,
			DisableCompression:  false, // 开启gzip省流量

			DialContext: (&net.Dialer{
				Timeout:   3 * time.Second,  // 拨号超时
				KeepAlive: 15 * time.Second, // TCP保活
				LocalAddr: localAddr,        // 绑定出口IP
			}).DialContext,
		}

		client = &http.Client{
			Transport: tr,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects, max 5")
				}
				return nil
			},
		}
	}
	return client, nil
}
func (h *Http) Get(link string, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodGet, link, nil, timeout, headers)
}
func (h *Http) Post(link string, pl []byte, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodPost, link, pl, timeout, headers)
}
func (h *Http) Delete(link string, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodDelete, link, nil, timeout, headers)
}
func (h *Http) Put(link string, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	return h.doRequest(http.MethodPut, link, nil, timeout, headers)
}
func (h *Http) doRequest(method, link string, pl []byte, timeout time.Duration,
	headers map[string]string) (int, []byte, error) {
	buffer := bytes.NewBuffer(pl)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, link, buffer)
	if err != nil {
		return 0, nil, errors.New("create request failed: " +
			link + ", err: " + err.Error())
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close() // 忽略关闭错误（核心是确保关闭）
		}
	}()
	if err != nil {
		var timeoutErr error
		if uErr, ok := err.(*url.Error); ok {
			if netErr, ok := uErr.Err.(net.Error); ok {
				if netErr.Timeout() {
					timeoutErr = errors.New("request timeout: " + link +
						", timeout: " + timeout.String())
				} else if netErr.Temporary() {
					timeoutErr = errors.New("temporary error (network issue): " +
						link + ", err: " + netErr.Error())
				}
			}
		}
		if timeoutErr != nil {
			return 0, nil, timeoutErr
		}
		return 0, nil, errors.New("request failed: " + link + ", err: " + err.Error())
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, errors.New("read response body failed: " +
			link + ", err: " + err.Error())
	}

	return resp.StatusCode, body, nil
}
