// Package redis 用原生 go-redis 驱动执行 redis 命令。
//
// 输入为 conn.TCPParams（type=tcp 的解析结果），构建 redis.Options 后
// 经 go-redis 的 Do 执行任意命令。密码只进入 redis.Options，绝不打印或落日志。
package redis

import (
	"context"
	"fmt"
	"net"
	"strconv"

	goredis "github.com/redis/go-redis/v9"

	"omni-cli/internal/conn"
)

// Options 把直连参数转为 go-redis 的 Options。
//
// 单次命令池大小为 1（PoolSize=1），不为一次查询开一池连接。
func Options(p *conn.TCPParams) (*goredis.Options, error) {
	if p == nil {
		return nil, fmt.Errorf("连接参数为空")
	}
	return &goredis.Options{
		Addr:     net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Password: p.Password,
		DB:       p.DB,
		PoolSize: 1,
	}, nil
}

// Exec 用 args 执行一条 redis 命令，返回命令结果。
//
// 每次调用新建 client，defer Close 回收连接；执行统一走传入的 ctx
// （调用方用 context.WithTimeout 控制超时）。
func Exec(ctx context.Context, p *conn.TCPParams, args []string) (any, error) {
	opts, err := Options(p)
	if err != nil {
		return nil, err
	}
	client := goredis.NewClient(opts)
	defer client.Close()

	cmdArgs := make([]any, 0, len(args))
	for _, a := range args {
		cmdArgs = append(cmdArgs, a)
	}
	val, err := client.Do(ctx, cmdArgs...).Result()
	if err != nil {
		return nil, fmt.Errorf("执行 redis 命令失败: %w", err)
	}
	return val, nil
}
