package filter

import "testing"

func TestMatcherGlobIncludeExclude(t *testing.T) {
	t.Parallel()

	matcher, err := NewMatcher("glob", "*linux*amd64*", "*debug*")
	if err != nil {
		t.Fatalf("创建 matcher 失败: %v", err)
	}

	tests := []struct {
		name string
		want bool
	}{
		{name: "app-linux-amd64.tar.gz", want: true},
		{name: "app-linux-amd64-debug.tar.gz", want: false},
		{name: "app-darwin-arm64.tar.gz", want: false},
	}

	for _, tt := range tests {
		got, err := matcher.Match(tt.name)
		if err != nil {
			t.Fatalf("匹配失败: %v", err)
		}
		if got != tt.want {
			t.Fatalf("%s 期望 %v，实际 %v", tt.name, tt.want, got)
		}
	}
}

func TestMatcherRegexIncludeExclude(t *testing.T) {
	t.Parallel()

	matcher, err := NewMatcher("regex", `.*linux.*amd64.*`, `.*debug.*`)
	if err != nil {
		t.Fatalf("创建 matcher 失败: %v", err)
	}

	got, err := matcher.Match("tool-linux-amd64.zip")
	if err != nil {
		t.Fatalf("匹配失败: %v", err)
	}
	if !got {
		t.Fatal("期望 linux amd64 资产被包含")
	}
}

func TestMatcherInvalidRegex(t *testing.T) {
	t.Parallel()

	if _, err := NewMatcher("regex", `(`, ""); err == nil {
		t.Fatal("期望无效正则返回错误")
	}
}
