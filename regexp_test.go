package gojq_test

import (
	"testing"

	"github.com/itchyny/gojq"
)

// Sample run, median of 3, Apple M3 Max, 2026.11.09, before and after the compiled-regexp cache:
//
//	func              ns/op before    after  speedup   allocs
//	test                      6847      613    11.2x   193->6
//	test_flags                2428      399     6.1x    76->5
//	match                     2261     1156     2.0x    51->20
//	match_g                   3354     3011     1.1x    76->66
//	capture                   2659     1466     1.8x    58->26
//	scan                      4358     4071     1.1x    80->70
//	split                     1359      985     1.4x    37->26
//	splits                    2356     2007     1.2x    65->54
//	sub                       2387     2113     1.1x    52->41
//	gsub                     25249    25363     1.0x   494->485
//	match_g_complex           4034     2196     1.8x    93->46
//	scan_complex              4420     1835     2.4x    91->25
//	sub_complex               6248     3254     1.9x   162->73
//	gsub_complex              8644     5836     1.5x   168->105
func BenchmarkRegexp(b *testing.B) {
	const input = "2026-11-08T10:00:05.123-07:00"
	for _, bc := range []struct{ name, query string }{
		{"test", `test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$")`},
		{"test_flags", `test("^[0-9]{4}-[0-9]{2}-[0-9]{2}t"; "i")`},
		{"match", `match("([0-9]{2}):([0-9]{2})")`},
		{"match_g", `match("[0-9]+"; "g")`},
		{"capture", `capture("(?<h>[0-9]{2}):(?<m>[0-9]{2})")`},
		{"scan", `scan("[0-9]+")`},
		{"split", `split("[-T:.]"; null)`},
		{"splits", `[splits("[-T:.]")]`},
		{"sub", `sub("T"; " ")`},
		{"gsub", `gsub("[0-9]"; "#")`},
		// patterns that cost something to compile
		{"match_g_complex", `match("(?<h>[0-9]{2}):(?<m>[0-9]{2})(?::(?<s>[0-9]{2}))?"; "g")`},
		{"scan_complex", `scan("[0-9]{2}:[0-9]{2}(?::[0-9]{2})?|[+-][0-9]{2}:[0-9]{2}$")`},
		{"sub_complex", `sub("^(?<y>[0-9]{4})-(?<mo>[0-9]{2})-(?<d>[0-9]{2})T"; "\(.mo)/\(.d)/\(.y) ")`},
		{"gsub_complex", `gsub("(?<h>[0-9]{2}):(?<m>[0-9]{2})|(?<tz>[+-][0-9]{2}:[0-9]{2})$"; "<\(.h // .tz)>")`},
	} {
		query, err := gojq.Parse(bc.query)
		if err != nil {
			b.Fatal(err)
		}
		code, err := gojq.Compile(query)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				iter := code.Run(input)
				for {
					v, ok := iter.Next()
					if !ok {
						break
					}
					if err, ok := v.(error); ok {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
