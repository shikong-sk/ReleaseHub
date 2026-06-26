package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"
)

// EmailNotifier 通过 SMTP 发送邮件通知
type EmailNotifier struct {
	from     string
	to       string
	host     string
	port     string
	username string
	password string
	useTLS   bool
}

// NewEmailNotifier 从 serverURL (格式: smtp://host:port) 和 token (格式: user:pass:from:to) 创建
func NewEmailNotifier(serverURL string, token string) (*EmailNotifier, error) {
	n := &EmailNotifier{}
	// 解析 serverURL: smtp://host:port 或 host:port
	addr := serverURL
	if strings.HasPrefix(addr, "smtp://") {
		addr = strings.TrimPrefix(addr, "smtp://")
		n.useTLS = true
	}
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("邮件服务器地址格式应为 host:port")
	}
	n.host = parts[0]
	n.port = parts[1]

	// 解析 token: username:password:from:to
	credParts := strings.SplitN(token, ":", 4)
	if len(credParts) < 4 {
		return nil, fmt.Errorf("邮件认证格式应为 username:password:from:to")
	}
	n.username = credParts[0]
	n.password = credParts[1]
	n.from = credParts[2]
	n.to = credParts[3]

	if _, err := mail.ParseAddress(n.from); err != nil {
		return nil, fmt.Errorf("发件人地址无效: %w", err)
	}
	return n, nil
}

func (e *EmailNotifier) Send(ctx context.Context, title string, message string) error {
	addr := e.host + ":" + e.port
	auth := smtp.PlainAuth("", e.username, e.password, e.host)

	subject := "ReleaseHub: " + title
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", e.from, e.to, subject, message)

	if e.useTLS {
		return e.sendTLS(addr, auth, body)
	}
	return smtp.SendMail(addr, auth, e.from, []string{e.to}, []byte(body))
}

func (e *EmailNotifier) sendTLS(addr string, auth smtp.Auth, body string) error {
	tlsConfig := &tls.Config{ServerName: e.host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS 连接邮件服务器失败: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, e.host)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}
	if err := client.Mail(e.from); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	if err := client.Rcpt(e.to); err != nil {
		return fmt.Errorf("设置收件人失败: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("发送邮件数据失败: %w", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("关闭邮件写入失败: %w", err)
	}
	return client.Quit()
}
