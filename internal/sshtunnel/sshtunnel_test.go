package sshtunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"omni-cli/internal/conn"
	"omni-cli/internal/sshtunnel/sshtest"
)

// tunnelParams 构造带 ssh 隧道的连接参数。
func tunnelParams(sshHost string, sshPort int, targetHost string, targetPort int) *conn.TCPParams {
	return &conn.TCPParams{
		Host:        targetHost,
		Port:        targetPort,
		SSHHost:     sshHost,
		SSHPort:     sshPort,
		SSHUser:     "jump",
		SSHPassword: "jump-pass",
	}
}

func TestOpenForwardsThroughTunnel(t *testing.T) {
	srv := sshtest.StartSSHServer(t, "jump", "jump-pass")
	targetHost, targetPort := sshtest.StartEchoTarget(t)
	sshHost, sshPort := srv.Addr()

	local, closeFn, err := Open(context.Background(), tunnelParams(sshHost, sshPort, targetHost, targetPort))
	if err != nil {
		t.Fatalf("Open 意外错误: %v", err)
	}
	defer closeFn()

	if local.SSHHost != "" {
		t.Errorf("返回参数应清除 ssh 字段，实际 SSHHost=%q", local.SSHHost)
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(local.Host, strconv.Itoa(local.Port)), 5*time.Second)
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
	got := srv.LastTarget()
	want := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
	if got != want {
		t.Errorf("direct-tcpip 目标 = %q, 期望 %q", got, want)
	}
}

func TestOpenDefaultsPort22(t *testing.T) {
	p := tunnelParams("j.example.com", 0, "10.0.0.7", 3306)
	if got := sshAddr(p); got != "j.example.com:22" {
		t.Errorf("sshAddr = %q, 期望 j.example.com:22", got)
	}
}

func TestOpenMissingSSHParams(t *testing.T) {
	tests := []struct {
		name   string
		params *conn.TCPParams
		want   string
	}{
		{
			name:   "缺 ssh_host",
			params: &conn.TCPParams{SSHUser: "u", SSHPassword: "p"},
			want:   "ssh_host",
		},
		{
			name:   "缺 ssh_user",
			params: &conn.TCPParams{SSHHost: "j", SSHPassword: "p"},
			want:   "ssh_user",
		},
		{
			name:   "缺 ssh_password",
			params: &conn.TCPParams{SSHHost: "j", SSHUser: "u"},
			want:   "ssh_password",
		},
		{
			name:   "缺目标 host",
			params: &conn.TCPParams{Port: 3306, SSHHost: "j", SSHUser: "u", SSHPassword: "p"},
			want:   "host",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Open(context.Background(), tt.params)
			if err == nil {
				t.Fatal("期望参数缺失报错，实际成功")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("错误 %q 应包含 %q", err.Error(), tt.want)
			}
		})
	}
}

func TestOpenAuthFailure(t *testing.T) {
	srv := sshtest.StartSSHServer(t, "jump", "jump-pass")
	targetHost, targetPort := sshtest.StartEchoTarget(t)
	sshHost, sshPort := srv.Addr()

	p := tunnelParams(sshHost, sshPort, targetHost, targetPort)
	p.SSHPassword = "wrong-pass"
	_, _, err := Open(context.Background(), p)
	if err == nil {
		t.Fatal("期望认证失败报错，实际成功")
	}
	if !strings.Contains(err.Error(), "认证") {
		t.Errorf("错误 %q 应包含 \"认证\"", err.Error())
	}
	if strings.Contains(err.Error(), "wrong-pass") {
		t.Errorf("错误信息不得泄漏密码，实际 %q", err.Error())
	}
}

func TestCloseStopsForwarding(t *testing.T) {
	srv := sshtest.StartSSHServer(t, "jump", "jump-pass")
	targetHost, targetPort := sshtest.StartEchoTarget(t)
	sshHost, sshPort := srv.Addr()

	local, closeFn, err := Open(context.Background(), tunnelParams(sshHost, sshPort, targetHost, targetPort))
	if err != nil {
		t.Fatalf("Open 意外错误: %v", err)
	}
	addr := net.JoinHostPort(local.Host, strconv.Itoa(local.Port))
	if err := closeFn(); err != nil {
		t.Fatalf("closeFn 意外错误: %v", err)
	}
	_, err = net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		t.Fatal("close 后本地转发端口应拒绝连接，实际成功")
	}
	if !errors.Is(closeFn(), errTunnelClosed) {
		t.Errorf("重复 close 应返回 errTunnelClosed，实际 %v", closeFn())
	}
}
