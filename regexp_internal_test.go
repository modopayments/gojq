package gojq

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func resetRegexpCache() {
	regexpCache.Lock()
	clear(regexpCache.m)
	regexpCache.Unlock()
}

func cachedRegexps() int {
	regexpCache.RLock()
	defer regexpCache.RUnlock()
	return len(regexpCache.m)
}

func TestCompileRegexp(t *testing.T) {
	resetRegexpCache()
	tests := []struct {
		name, re, flags string
		wantErr         string
	}{
		{"plain", "[0-9]+", "", ""},
		{"global flag", "[0-9]+", "g", ""},
		{"case insensitive", "[0-9]+", "i", ""},
		{"multiline", "^x$", "m", ""},
		{"named group", "(?<d>[0-9]+)", "", ""},
		{"invalid pattern", "(", "", "invalid regular expression"},
		{"unsupported flag", "x", "z", "unsupported regular expression flag"},
	}
	seen := map[*regexp.Regexp]string{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := compileRegexp(tt.re, tt.flags)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if prev, dup := seen[r]; dup {
				t.Errorf("shares a compiled regexp with %q", prev)
			}
			seen[r] = tt.name
			if again, _ := compileRegexp(tt.re, tt.flags); again != r {
				t.Error("second call did not hit the cache")
			}
		})
	}
	if n := cachedRegexps(); n != len(seen) {
		t.Errorf("%d cached, want %d", n, len(seen))
	}
}

func TestCompileRegexpCap(t *testing.T) {
	resetRegexpCache()
	first, err := compileRegexp("first", "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < regexpCacheMax; i++ {
		if _, err := compileRegexp(fmt.Sprintf("p%d", i), ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := cachedRegexps(); n > regexpCacheMax {
		t.Errorf("%d cached, cap is %d", n, regexpCacheMax)
	}
	if again, _ := compileRegexp("first", ""); again == first {
		t.Error("expected the cap to have evicted the first pattern")
	}
}

// run with -race
func TestCompileRegexpConcurrent(t *testing.T) {
	resetRegexpCache()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if _, err := compileRegexp(fmt.Sprintf("c%d", i%16), "g"); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}
