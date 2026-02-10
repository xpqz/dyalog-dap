package decode

import (
	"strconv"
	"strings"
)

func String(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func StringOrEmpty(v any) string {
	s, _ := String(v)
	return s
}

func NonEmptyTrimmedString(v any) (string, bool) {
	s, ok := String(v)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func StringFromMap(values map[string]any, key string) (string, bool) {
	v, ok := values[key]
	if !ok {
		return "", false
	}
	return String(v)
}

func StringOrEmptyFromMap(values map[string]any, key string) string {
	v, ok := values[key]
	if !ok {
		return ""
	}
	return StringOrEmpty(v)
}

func NonEmptyTrimmedStringFromMap(values map[string]any, key string) (string, bool) {
	v, ok := values[key]
	if !ok {
		return "", false
	}
	return NonEmptyTrimmedString(v)
}

func Int(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case uint:
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case uint64:
		return int(x), true
	case float32:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}

func IntOrZero(v any) int {
	n, ok := Int(v)
	if !ok {
		return 0
	}
	return n
}

func IntFromTextOrNumber(v any) (int, bool) {
	if n, ok := Int(v); ok {
		return n, true
	}
	s, ok := String(v)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func IntFromMap(values map[string]any, key string) (int, bool) {
	v, ok := values[key]
	if !ok {
		return 0, false
	}
	return Int(v)
}

func IntOrZeroFromMap(values map[string]any, key string) int {
	v, ok := values[key]
	if !ok {
		return 0
	}
	return IntOrZero(v)
}

func IntFromMapTextOrNumber(values map[string]any, key string) (int, bool) {
	v, ok := values[key]
	if !ok {
		return 0, false
	}
	return IntFromTextOrNumber(v)
}

func Bool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case int:
		return boolFrom01(int64(x))
	case int8:
		return boolFrom01(int64(x))
	case int16:
		return boolFrom01(int64(x))
	case int32:
		return boolFrom01(int64(x))
	case int64:
		return boolFrom01(x)
	case uint:
		return boolFrom01Uint(uint64(x))
	case uint8:
		return boolFrom01Uint(uint64(x))
	case uint16:
		return boolFrom01Uint(uint64(x))
	case uint32:
		return boolFrom01Uint(uint64(x))
	case uint64:
		return boolFrom01Uint(x)
	case float32:
		return boolFrom01Float(float64(x))
	case float64:
		return boolFrom01Float(x)
	default:
		return false, false
	}
}

func BoolOrFalse(v any) bool {
	b, ok := Bool(v)
	if !ok {
		return false
	}
	return b
}

func Slice(v any) []any {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	return items
}

func SliceFromMap(values map[string]any, key string) []any {
	v, ok := values[key]
	if !ok {
		return nil
	}
	return Slice(v)
}

func StringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func StringSliceFromMap(values map[string]any, key string) []string {
	v, ok := values[key]
	if !ok {
		return nil
	}
	return StringSlice(v)
}

func IntSlice(v any) []int {
	items := Slice(v)
	result := make([]int, 0, len(items))
	for _, item := range items {
		if n, ok := Int(item); ok {
			result = append(result, n)
		}
	}
	return result
}

func IntSliceFromMap(values map[string]any, key string) []int {
	v, ok := values[key]
	if !ok {
		return nil
	}
	return IntSlice(v)
}

func boolFrom01(v int64) (bool, bool) {
	if v == 0 {
		return false, true
	}
	if v == 1 {
		return true, true
	}
	return false, false
}

func boolFrom01Uint(v uint64) (bool, bool) {
	if v == 0 {
		return false, true
	}
	if v == 1 {
		return true, true
	}
	return false, false
}

func boolFrom01Float(v float64) (bool, bool) {
	if v == 0 {
		return false, true
	}
	if v == 1 {
		return true, true
	}
	return false, false
}
