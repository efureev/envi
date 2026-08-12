package bind

// Tests in the package itself, for helpers and guards that cannot be reached
// through the exported API. Everything a consumer can reach is tested from
// bind_test.

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestJoinKey(t *testing.T) {
	t.Parallel()

	tests := []struct{ prefix, key, want string }{
		{"", "PORT", "PORT"},
		{"APP", "PORT", "APP_PORT"},
		{"APP", "", "APP"},
		{"", "", ""},
	}
	for _, tc := range tests {
		if got := joinKey(tc.prefix, tc.key); got != tc.want {
			t.Errorf("joinKey(%q, %q) = %q, want %q", tc.prefix, tc.key, got, tc.want)
		}
	}
}

func TestCamelToSnake(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"AppName":    "App_Name",
		"DBHost":     "DB_Host",
		"HTTPPort":   "HTTP_Port",
		"ID":         "ID",
		"A":          "A",
		"":           "",
		"Port2Value": "Port2_Value",
		"lowercase":  "lowercase",
	}
	for in, want := range tests {
		if got := camelToSnake(in); got != want {
			t.Errorf("camelToSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestItoa(t *testing.T) {
	t.Parallel()

	tests := map[int]string{0: "0", 1: "1", 9: "9", 10: "10", 4096: "4096"}
	for in, want := range tests {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

// setViaText refuses a value it cannot take the address of, rather than
// panicking inside reflect. Struct fields are always addressable, so this
// guard is unreachable through the exported API — which is exactly why it is
// worth pinning here.
func TestSetViaTextRejectsUnaddressableValue(t *testing.T) {
	t.Parallel()

	unaddressable := reflect.ValueOf(time.Time{})
	if unaddressable.CanAddr() {
		t.Fatal("precondition: the value must not be addressable")
	}

	err := setViaText(unaddressable, "2026-08-12T10:00:00Z")
	if !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("error = %v, want ErrUnsupportedType", err)
	}
}

// The plan cache must hand back the same plan for a repeated type rather than
// rebuilding it, which is what keeps binding allocation-free after warm-up.
func TestPlanCacheReturnsTheSameInstance(t *testing.T) {
	t.Parallel()

	type cached struct {
		Port int `env:"PORT"`
	}
	typ := reflect.TypeFor[cached]()

	first, err := planFor(typ, defaultTagName)
	if err != nil {
		t.Fatal(err)
	}
	second, err := planFor(typ, defaultTagName)
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Error("planFor rebuilt the plan instead of serving it from cache")
	}
}
