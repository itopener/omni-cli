// Package output 把结构化结果渲染为对齐表格或缩进 JSON。
//
// 表格用于 db 查询结果与 list 清单，JSON 用于机器可读输出。表格按列对齐，
// nil 渲染为 NULL，[]byte 直接转字符串（database/sql 扫描的字节列）。
package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itopener/omni-cli/internal/db"
)

// colSep 是表格列间的分隔符（两个空格，保证对齐且可读）。
const colSep = "  "

// Table 把 db.Result 渲染为对齐表格：首行列名，之后每行一条记录。
// 列宽取该列最宽单元格，各列用两个空格分隔；无列时返回空串。
func Table(r *db.Result) string {
	if r == nil || len(r.Columns) == 0 {
		return ""
	}
	widths := make([]int, len(r.Columns))
	for i, c := range r.Columns {
		widths[i] = len(c)
	}
	rows := make([][]string, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = make([]string, len(r.Columns))
		for j := 0; j < len(r.Columns) && j < len(row); j++ {
			s := cellString(row[j])
			rows[i][j] = s
			if len(s) > widths[j] {
				widths[j] = len(s)
			}
		}
	}
	var b strings.Builder
	writeRow(&b, r.Columns, widths)
	for _, row := range rows {
		writeRow(&b, row, widths)
	}
	return strings.TrimRight(b.String(), "\n")
}

// JSON 把 v 渲染为两空格缩进的 JSON。v 无法序列化时返回 "null"。
func JSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "null"
	}
	return string(data)
}

// cellString 把单元格值转为字符串：nil → NULL，[]byte → string，其余用 %v。
func cellString(v any) string {
	if v == nil {
		return "NULL"
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

// writeRow 把一行按列宽右对齐不足补空格写入 b，列间用两个空格分隔。
func writeRow(b *strings.Builder, cells []string, widths []int) {
	for i, c := range cells {
		if i > 0 {
			b.WriteString(colSep)
		}
		b.WriteString(c)
		if len(c) < widths[i] {
			b.WriteString(strings.Repeat(" ", widths[i]-len(c)))
		}
	}
	b.WriteString("\n")
}
