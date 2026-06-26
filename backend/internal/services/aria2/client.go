package aria2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client aria2 JSON-RPC 客户端（仅支持 HTTP JSON-RPC）
type Client struct {
	rpcURL  string
	secret  string
	httpURL string
}

// Config aria2 配置
type Config struct {
	RPCEndpoint  string // 例如: http://localhost:6800/jsonrpc
	Secret       string
	HTTPEndpoint string // 文件下载地址，例如: http://localhost:6800
}

// NewClient 创建 aria2 RPC 客户端
func NewClient(cfg Config) (*Client, error) {
	if cfg.RPCEndpoint == "" {
		return nil, fmt.Errorf("aria2 RPC 端点不能为空")
	}
	return &Client{
		rpcURL:  cfg.RPCEndpoint,
		secret:  cfg.Secret,
		httpURL: cfg.HTTPEndpoint,
	}, nil
}

// Ping 测试 aria2 连接
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.GetGlobalStat(ctx)
	return err
}

// AddURI 添加下载任务
func (c *Client) AddURI(ctx context.Context, uris []string, options map[string]interface{}) (string, error) {
	args := []interface{}{"token:" + c.secret, uris}
	if options != nil {
		args = append(args, options)
	} else {
		args = append(args, map[string]interface{}{})
	}
	var gid string
	err := c.call(ctx, "aria2.addUri", args, &gid)
	return gid, err
}

// TellStatus 查询任务状态
func (c *Client) TellStatus(ctx context.Context, gid string, keys ...string) (map[string]interface{}, error) {
	args := []interface{}{"token:" + c.secret, gid}
	if len(keys) > 0 {
		args = append(args, keys)
	}
	var status map[string]interface{}
	err := c.call(ctx, "aria2.tellStatus", args, &status)
	return status, err
}

// Remove 删除任务
func (c *Client) Remove(ctx context.Context, gid string) (string, error) {
	args := []interface{}{"token:" + c.secret, gid}
	var result string
	err := c.call(ctx, "aria2.remove", args, &result)
	return result, err
}

// GetGlobalStat 获取全局状态
func (c *Client) GetGlobalStat(ctx context.Context) (map[string]interface{}, error) {
	args := []interface{}{"token:" + c.secret}
	var stat map[string]interface{}
	err := c.call(ctx, "aria2.getGlobalStat", args, &stat)
	return stat, err
}

// HTTPEndpoint 返回 HTTP 文件服务地址
func (c *Client) HTTPEndpoint() string {
	return c.httpURL
}

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params interface{}, reply interface{}) error {
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if !strings.HasPrefix(c.rpcURL, "http") {
		// 如果不是 HTTP URL，走 WebSocket（暂不支持）
		return fmt.Errorf("暂不支持非 HTTP 方式的 aria2 连接")
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("aria2 RPC 错误 [%d]: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	if reply != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, reply); err != nil {
			return fmt.Errorf("解析结果失败: %w", err)
		}
	}

	return nil
}
