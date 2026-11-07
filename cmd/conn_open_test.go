package main

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"omni-cli/internal/config"
	"omni-cli/internal/conn"
	"omni-cli/internal/sshtunnel/sshtest"
)

// TestOpenConnDirect 回归：无 ssh 参数时直连，返回原参数且 closeFn 为空操作。
func TestOpenConnDirect(t *testing.T) {
	entry := &config.Entry{
		Type:   conn.ConnTypeTCP,
		Params: map[string]any{"host": "127.0.0.1", "port": 3306},
	}
	got, closeFn, err := openConn(context.Background(), entry)
	if err != nil {
		t.Fatalf("openConn(直连) 意外错误: %v", err)
	}
	if got.Host != "127.0.0.1" || got.Port != 3306 {
		t.Errorf("直连参数 = %+v, 期望原样返回 host=127.0.0.1 port=3306", got)
	}
	if err := closeFn(); err != nil {
		t.Errorf("直连 closeFn 应为空操作，实际错误: %v", err)
	}
}

// TestOpenConnTunnel 回归：带 ssh 参数时经跳板机转发，返回本地地址供 db/redis 建连。
func TestOpenConnTunnel(t *testing.T) {
	srv := sshtest.StartSSHServer(t, "jump", "jump-pass")
	targetHost, targetPort := sshtest.StartEchoTarget(t)
	sshHost, sshPort := srv.Addr()

	entry := &config.Entry{
		Type: conn.ConnTypeTCP,
		Params: map[string]any{
			"host":         targetHost,
			"port":         targetPort,
			"ssh_host":     sshHost,
			"ssh_port":     sshPort,
			"ssh_user":     "jump",
			"ssh_password": "jump-pass",
		},
	}
	got, closeFn, err := openConn(context.Background(), entry)
	if err != nil {
		t.Fatalf("openConn(隧道) 意外错误: %v", err)
	}
	defer closeFn()
	if got.Host != "127.0.0.1" {
		t.Errorf("隧道模式 host = %q, 期望 127.0.0.1（本地转发地址）", got.Host)
	}

	c, err := net.DialTimeout("tcp", net.JoinHostPort(got.Host, strconv.Itoa(got.Port)), 5*time.Second)
	if err != nil {
		t.Fatalf("连接本地转发端口: %v", err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatalf("写入: %v", err)
	}
	buf := make([]byte, 4)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatalf("读取回显: %v", err)
	}
	if string(buf) != "ping" {
		t.Errorf("回显 = %q, 期望 %q", buf, "ping")
	}
	want := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
	if got := srv.LastTarget(); got != want {
		t.Errorf("direct-tcpip 目标 = %q, 期望 %q", got, want)
	}
}

// TestOpenConnPropagatesResolveError 回归：连接参数非法时报错并带字段名。
func TestOpenConnPropagatesResolveError(t *testing.T) {
	entry := &config.Entry{
		Type:   conn.ConnTypeTCP,
		Params: map[string]any{"host": "127.0.0.1", "port": "notanumber"},
	}
	_, _, err := openConn(context.Background(), entry)
	if err == nil {
		t.Fatal("期望解析错误，实际成功")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("错误 %q 应包含 \"port\"", err.Error())
	}
}
