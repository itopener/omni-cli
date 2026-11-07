// Package sshtest 提供进程内 ssh 跳板机与回显目标，供 sshtunnel 与 cmd 层测试复用。
//
// 仅测试代码导入，不属于产品面。
package sshtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// directTCPPayload 是 direct-tcpip 通道请求的载荷（x/crypto/ssh 约定）。
// 字段集必须与协议消息完全一致，ssh.Unmarshal 拒绝多余字节。
type directTCPPayload struct {
	Addr              string
	Port              uint32
	OriginatorAddress string
	OriginatorPort    uint32
}

// Server 是进程内 ssh 跳板机：密码认证 + direct-tcpip 转发到真实目标。
type Server struct {
	listener net.Listener

	mu         sync.Mutex
	lastTarget string
}

// StartSSHServer 启动只接受 user/pass 认证、支持 direct-tcpip 转发的 ssh 服务端，
// 返回监听地址；tb 结束时自动关闭。
func StartSSHServer(tb testing.TB, user, pass string) *Server {
	tb.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatalf("生成 ed25519 密钥: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		tb.Fatalf("构造 ssh signer: %v", err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if meta.User() == user && string(password) == pass {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("认证失败")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("监听 ssh 端口: %v", err)
	}
	s := &Server{listener: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serveConn(c, cfg)
		}
	}()
	tb.Cleanup(func() { ln.Close() })
	return s
}

// Addr 返回服务端监听的 host 与 port。
func (s *Server) Addr() (host string, port int) {
	return splitAddr(s.listener.Addr().String())
}

// LastTarget 返回最近一次 direct-tcpip 请求的目标 host:port；未有请求时返回空串。
func (s *Server) LastTarget() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastTarget
}

// serveConn 完成握手并转发 direct-tcpip 通道。
func (s *Server) serveConn(c net.Conn, cfg *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() != "direct-tcpip" {
			_ = newCh.Reject(ssh.UnknownChannelType, "仅支持 direct-tcpip")
			continue
		}
		var payload directTCPPayload
		if err := ssh.Unmarshal(newCh.ExtraData(), &payload); err != nil {
			_ = newCh.Reject(ssh.Prohibited, "载荷解析失败")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		target := net.JoinHostPort(payload.Addr, strconv.Itoa(int(payload.Port)))
		s.mu.Lock()
		s.lastTarget = target
		s.mu.Unlock()
		go func(ch ssh.Channel, chReqs <-chan *ssh.Request, target string) {
			defer ch.Close()
			go ssh.DiscardRequests(chReqs)
			tc, err := net.Dial("tcp", target)
			if err != nil {
				return
			}
			defer tc.Close()
			done := make(chan struct{}, 2)
			go func() { io.Copy(tc, ch); done <- struct{}{} }()
			go func() { io.Copy(ch, tc); done <- struct{}{} }()
			<-done
		}(ch, chReqs, target)
	}
}

// StartEchoTarget 启动回显目标服务（写什么读回什么），返回地址；tb 结束时自动关闭。
func StartEchoTarget(tb testing.TB) (host string, port int) {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("监听目标端口: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(c)
		}
	}()
	tb.Cleanup(func() { ln.Close() })
	return splitAddr(ln.Addr().String())
}

// splitAddr 拆 host:port 为 host 与 int port。
func splitAddr(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}
