package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestVersionOutput 表驱动：--version flag 与 version 子命令输出一致的版本信息，
// 且都读取包变量 version（等价 -ldflags "-X main.version=..." 注入）。
func TestVersionOutput(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		version string
	}{
		{name: "flag 默认版本", args: []string{"--version"}, version: "dev"},
		{name: "子命令 默认版本", args: []string{"version"}, version: "dev"},
		{name: "flag 注入版本", args: []string{"--version"}, version: "v1.2.3-test"},
		{name: "子命令 注入版本", args: []string{"version"}, version: "v1.2.3-test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVersion := version
			version = tt.version
			t.Cleanup(func() { version = oldVersion })

			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetErr(&buf)
			rootCmd.SetArgs(tt.args)
			t.Cleanup(func() {
				rootCmd.SetOut(os.Stdout)
				rootCmd.SetErr(os.Stderr)
				rootCmd.SetArgs(nil)
			})

			if err := Execute(); err != nil {
				t.Fatalf("Execute() 意外错误: %v", err)
			}
			got := buf.String()
			want := "omni-cli version " + tt.version + "\n"
			if !strings.Contains(got, "version") {
				t.Errorf("输出 %q 不含 \"version\"", got)
			}
			if !strings.Contains(got, tt.version) {
				t.Errorf("输出 %q 不含版本值 %q", got, tt.version)
			}
			if got != want {
				t.Errorf("输出 = %q, 期望 %q", got, want)
			}
		})
	}
}
