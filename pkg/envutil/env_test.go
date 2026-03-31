package envutil_test

import (
	"os"
	"testing"
	"time"

	"github.com/littleSand/adama/pkg/envutil"
)

func TestCSV(t *testing.T) {
	key := "ADAMA_TEST_ENVUTIL_CSV"
	t.Setenv(key, " a, b ,, c ")

	got := envutil.CSV(key, []string{"fallback"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len(csv) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("csv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIntAndDurationFallback(t *testing.T) {
	intKey := "ADAMA_TEST_ENVUTIL_INT"
	durationKey := "ADAMA_TEST_ENVUTIL_DURATION"
	t.Setenv(intKey, "not-a-number")
	t.Setenv(durationKey, "invalid")

	if got := envutil.Int(intKey, 12); got != 12 {
		t.Fatalf("Int fallback = %d, want 12", got)
	}
	if got := envutil.Duration(durationKey, 3*time.Second); got != 3*time.Second {
		t.Fatalf("Duration fallback = %s, want 3s", got)
	}
}

func TestGetFallbackWhenUnset(t *testing.T) {
	key := "ADAMA_TEST_ENVUTIL_GET"
	_ = os.Unsetenv(key)

	if got := envutil.Get(key, "fallback"); got != "fallback" {
		t.Fatalf("Get fallback = %q, want fallback", got)
	}
}
