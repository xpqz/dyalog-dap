package decode

import "testing"

func TestNonEmptyTrimmedStringFromMap(t *testing.T) {
	values := map[string]any{
		"a": "  hello  ",
		"b": "   ",
		"c": 42,
	}

	got, ok := NonEmptyTrimmedStringFromMap(values, "a")
	if !ok || got != "hello" {
		t.Fatalf("expected trimmed non-empty string, got %q ok=%v", got, ok)
	}
	if _, ok := NonEmptyTrimmedStringFromMap(values, "b"); ok {
		t.Fatal("expected whitespace-only value to be rejected")
	}
	if _, ok := NonEmptyTrimmedStringFromMap(values, "c"); ok {
		t.Fatal("expected non-string value to be rejected")
	}
}

func TestIntConversionVariants(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{name: "int", in: int(7), want: 7, ok: true},
		{name: "int64", in: int64(8), want: 8, ok: true},
		{name: "uint32", in: uint32(9), want: 9, ok: true},
		{name: "float64", in: float64(10), want: 10, ok: true},
		{name: "string rejected", in: "11", want: 0, ok: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Int(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("Int(%T) = %d ok=%v, want %d ok=%v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestIntFromTextOrNumber(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{name: "numeric", in: float64(15), want: 15, ok: true},
		{name: "text numeric", in: "16", want: 16, ok: true},
		{name: "text invalid", in: "16ms", want: 0, ok: false},
		{name: "text spaced invalid to preserve current behavior", in: " 16 ", want: 0, ok: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok := IntFromTextOrNumber(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("IntFromTextOrNumber(%v) = %d ok=%v, want %d ok=%v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestBoolConversion(t *testing.T) {
	trueValues := []any{true, 1, int8(1), uint64(1), float64(1)}
	for _, v := range trueValues {
		got, ok := Bool(v)
		if !ok || !got {
			t.Fatalf("expected Bool(%v) to be true/ok, got %v/%v", v, got, ok)
		}
	}

	falseValues := []any{false, 0, int32(0), uint(0), float32(0)}
	for _, v := range falseValues {
		got, ok := Bool(v)
		if !ok || got {
			t.Fatalf("expected Bool(%v) to be false/ok, got %v/%v", v, got, ok)
		}
	}

	if _, ok := Bool(2); ok {
		t.Fatal("expected Bool(2) to be invalid")
	}
}

func TestSliceConversions(t *testing.T) {
	values := map[string]any{
		"strings": []any{"a", 1, "b"},
		"ints":    []any{1, "x", int64(2), float64(3)},
	}

	stringSlice := StringSliceFromMap(values, "strings")
	if len(stringSlice) != 2 || stringSlice[0] != "a" || stringSlice[1] != "b" {
		t.Fatalf("unexpected string slice: %#v", stringSlice)
	}

	intSlice := IntSliceFromMap(values, "ints")
	if len(intSlice) != 3 || intSlice[0] != 1 || intSlice[1] != 2 || intSlice[2] != 3 {
		t.Fatalf("unexpected int slice: %#v", intSlice)
	}
}
