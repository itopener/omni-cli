// Package sshtunnel 把 tcp 连接的可选 ssh 隧道能力落地：
//
// params 含 ssh_host 时，先与跳板机建 ssh 连接（密码认证），再在本地
// 127.0.0.1 随机端口起转发监听；db / redis 对返回的本地地址建连，流量经
// 隧道直达目标 host:port。跳板机密码只进 ssh.ClientConfig（内存），不打印。
//
// v1 不校验跳板机主机密钥（InsecureIgnoreHostKey），已知限制见 docs/design.md。
package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"omni-cli/internal/conn"
)

// errTunnelClosed 由重复 Close 返回，调用方可用 errors.Is 判断。
var errTunnelClosed = errors.New("ssh 隧道已关闭")

// authFailureMarker 是 x/crypto/ssh 密码认证失败的错误特征。
const authFailureMarker = "unable to authenticate"

// tunnel 持有转发监听与 ssh 客户端，Close 幂等。
type tunnel struct {
	listener net.Listener
	client   *ssh.Client
	once     sync.Once
	closeErr error
}

// Open 校验 ssh 参数，建立到跳板机的 ssh 连接并在本地起转发监听。
//
// 返回的 TCPParams 是原参数的副本，host/port 已替换为本地转发地址、ssh 字段
// 已清空，可直接交给 db / redis 建连；closeFn 关闭监听与 ssh 连接，幂等，
// 重复调用返回 errTunnelClosed。ssh 拨号与握手受 ctx 控制。
func Open(ctx context.Context, p *conn.TCPParams) (*conn.TCPParams, func() error, error) {
	if p == nil {
		return nil, nil, fmt.Errorf("连接参数为空")
	}
	if p.SSHHost == "" {
		return nil, nil, fmt.Errorf("ssh 隧道缺少 ssh_host（跳板机地址）")
	}
	if p.SSHUser == "" {
		return nil, nil, fmt.Errorf("ssh 隧道缺少 ssh_user（跳板机用户名）")
	}
	if p.SSHPassword == "" {
		return nil, nil, fmt.Errorf("ssh 隧道缺少 ssh_password（跳板机密码）")
	}
	if p.Host == "" {
		return nil, nil, fmt.Errorf("ssh 隧道缺少 host（跳板机视角的目标地址）")
	}

	cfg := &ssh.ClientConfig{
		User: p.SSHUser,
		Auth: []ssh.AuthMethod{ssh.Password(p.SSHPassword)},
		// v1 不做主机密钥校验（TOFU / known_hosts 留待扩展）。
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         handshakeTimeout(ctx),
	}

	addr := sshAddr(p)
	d := net.Dialer{}
	nc, err := d.DialContext(ctx, networkTCP, addr)
	if err != nil {
		return nil, nil, fmt.Errorf("连接跳板机 %s 失败: %w", addr, err)
	}
	sconn, chans, reqs, err := ssh.NewClientConn(nc, addr, cfg)
	if err != nil {
		nc.Close()
		if strings.Contains(err.Error(), authFailureMarker) {
			return nil, nil, fmt.Errorf("跳板机 %s ssh 认证失败（检查 ssh_user / ssh_password）: %w", addr, err)
		}
		return nil, nil, fmt.Errorf("跳板机 %s ssh 握手失败: %w", addr, err)
	}
	client := ssh.NewClient(sconn, chans, reqs)

	ln, err := net.Listen(networkTCP, "127.0.0.1:0")
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("监听本地转发端口失败: %w", err)
	}

	t := &tunnel{listener: ln, client: client}
	go t.serve(net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))

	local := *p
	local.SSHHost, local.SSHPort, local.SSHUser, local.SSHPassword = "", 0, "", ""
	local.Host, local.Port = splitAddr(ln.Addr().String())
	return &local, t.Close, nil
}

// networkTCP 是 net 拨号/监听统一使用的网络名。
const networkTCP = "tcp"

// sshAddr 返回跳板机地址；ssh_port 省略（0）时用默认端口 22。
func sshAddr(p *conn.TCPParams) string {
	port := p.SSHPort
	if port == 0 {
		port = conn.DefaultSSHPort
	}
	return net.JoinHostPort(p.SSHHost, strconv.Itoa(port))
}

// handshakeTimeout 从 ctx 截止时间推导 ssh 握手超时；无截止时间则不限。
func handshakeTimeout(ctx context.Context) time.Duration {
	dl, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	return time.Until(dl)
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

// serve 接受本地连接并逐个经隧道转发；监听关闭后退出。
func (t *tunnel) serve(target string) {
	for {
		c, err := t.listener.Accept()
		if err != nil {
			return
		}
		go t.forward(c, target)
	}
}

// forward 把一条本地连接与经隧道到 target 的连接双向对拷。
func (t *tunnel) forward(c net.Conn, target string) {
	defer c.Close()
	rc, err := t.client.Dial(networkTCP, target)
	if err != nil {
		return
	}
	defer rc.Close()
	done := make(chan struct{}, 2)
	go func() { io.Copy(rc, c); done <- struct{}{} }()
	go func() { io.Copy(c, rc); done <- struct{}{} }()
	<-done
}

// Close 关闭本地监听与 ssh 连接，幂等；重复调用返回 errTunnelClosed。
func (t *tunnel) Close() error {
	first := false
	t.once.Do(func() {
		first = true
		t.closeErr = errors.Join(t.listener.Close(), t.client.Close())
	})
	if !first {
		return errTunnelClosed
	}
	return t.closeErr
}
