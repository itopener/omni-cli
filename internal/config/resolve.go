package config

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveEntry 在条目列表中按 key/env 解析唯一条目。
//
// key 与 env 任一为空时视为「缺省」：过滤后若只剩唯一候选则直接返回；
// 多个候选则报错并列出所有候选项；无候选也报错并列出可选 key/env。
func ResolveEntry(entries []Entry, key, env string) (*Entry, error) {
	if key != "" {
		var idx []int
		for i := range entries {
			if entries[i].Key == key {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			return nil, notFoundErr(entries, key, env)
		}
		if env != "" {
			for _, i := range idx {
				if entries[i].Env == env {
					return &entries[i], nil
				}
			}
			return nil, notFoundErr(entries, key, env)
		}
		if len(idx) == 1 {
			return &entries[idx[0]], nil
		}
		return nil, ambiguousErr(entries, idx, key, "")
	}

	if env != "" {
		var idx []int
		for i := range entries {
			if entries[i].Env == env {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			return nil, notFoundErr(entries, key, env)
		}
		if len(idx) == 1 {
			return &entries[idx[0]], nil
		}
		return nil, ambiguousErr(entries, idx, "", env)
	}

	if len(entries) == 1 {
		return &entries[0], nil
	}
	idx := make([]int, len(entries))
	for i := range idx {
		idx[i] = i
	}
	return nil, ambiguousErr(entries, idx, "", "")
}

// ambiguousErr 构造候选多项时的错误，列出可区分维度的候选项。
func ambiguousErr(entries []Entry, idx []int, key, env string) error {
	switch {
	case key != "": // env 缺省：列出候选 env 名
		envs := make([]string, 0, len(idx))
		for _, i := range idx {
			envs = append(envs, entries[i].Env)
		}
		return fmt.Errorf("key %q 匹配多个 env，请指定 env。候选: %s",
			key, strings.Join(distinctSorted(envs), ", "))
	case env != "": // key 缺省：列出候选 key 名
		keys := make([]string, 0, len(idx))
		for _, i := range idx {
			keys = append(keys, entries[i].Key)
		}
		return fmt.Errorf("env %q 匹配多个 key，请指定 key。候选: %s",
			env, strings.Join(distinctSorted(keys), ", "))
	default: // key/env 均缺省：列出 key/env 组合
		pairs := make([]string, 0, len(idx))
		for _, i := range idx {
			pairs = append(pairs, entries[i].Key+"/"+entries[i].Env)
		}
		return fmt.Errorf("未指定 key/env，匹配多个条目。候选: %s",
			strings.Join(distinctSorted(pairs), ", "))
	}
}

// notFoundErr 构造无匹配时的错误，列出可选 key/env。
func notFoundErr(entries []Entry, key, env string) error {
	keys := make([]string, 0, len(entries))
	envs := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.Key)
		envs = append(envs, e.Env)
	}
	return fmt.Errorf("未找到 key=%q env=%q 的配置条目。可用 key: %s；可用 env: %s",
		key, env,
		strings.Join(distinctSorted(keys), ", "),
		strings.Join(distinctSorted(envs), ", "))
}

// distinctSorted 去重并排序，保证错误信息里的候选项列表稳定。
func distinctSorted(vals []string) []string {
	seen := make(map[string]bool, len(vals))
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
