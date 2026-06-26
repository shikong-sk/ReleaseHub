package filter

import (
	"path"
	"regexp"
	"strings"
)

type Mode string

const (
	ModeGlob  Mode = "glob"
	ModeRegex Mode = "regex"
)

type Matcher struct {
	mode    Mode
	include []string
	exclude []string
}

func NewMatcher(mode string, includePatterns string, excludePatterns string) (*Matcher, error) {
	matcher := &Matcher{
		mode:    Mode(strings.ToLower(strings.TrimSpace(mode))),
		include: splitPatterns(includePatterns),
		exclude: splitPatterns(excludePatterns),
	}
	if matcher.mode == "" {
		matcher.mode = ModeGlob
	}
	if matcher.mode != ModeGlob && matcher.mode != ModeRegex {
		matcher.mode = ModeGlob
	}

	if matcher.mode == ModeRegex {
		for _, pattern := range append(matcher.include, matcher.exclude...) {
			if _, err := regexp.Compile(pattern); err != nil {
				return nil, err
			}
		}
	}

	return matcher, nil
}

func (m *Matcher) Match(name string) (bool, error) {
	// include 为空表示先纳入全部资产，再由 exclude 排除。
	included := len(m.include) == 0
	for _, pattern := range m.include {
		matched, err := m.matchPattern(pattern, name)
		if err != nil {
			return false, err
		}
		if matched {
			included = true
			break
		}
	}
	if !included {
		return false, nil
	}

	for _, pattern := range m.exclude {
		matched, err := m.matchPattern(pattern, name)
		if err != nil {
			return false, err
		}
		if matched {
			return false, nil
		}
	}

	return true, nil
}

func (m *Matcher) matchPattern(pattern string, name string) (bool, error) {
	if m.mode == ModeRegex {
		return regexp.MatchString(pattern, name)
	}

	return path.Match(pattern, name)
}

func splitPatterns(patterns string) []string {
	fields := strings.FieldsFunc(patterns, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})

	result := make([]string, 0, len(fields))
	for _, field := range fields {
		pattern := strings.TrimSpace(field)
		if pattern != "" {
			result = append(result, pattern)
		}
	}

	return result
}
