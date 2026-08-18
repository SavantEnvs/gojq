// mayhem/kat — known-answer-test probe for mayhem/test.sh.
//
// WHY A SEPARATE BINARY (SPEC §6.3 anti-reward-hacking):
// `go test` links a STATIC binary, so the verify-repo sabotage check (which
// LD_PRELOADs a shim whose constructor calls _exit(0) for non-system
// executables) cannot neuter it — a suite that only runs `go test` is
// therefore immune to the sabotage check and does NOT prove the oracle is
// behavioral. This probe is built with cgo (see cgo_dynamic.go) so it is
// DYNAMICALLY linked: the shim reaches it, the process becomes an instant
// no-op, it prints nothing, and test.sh's exact string assertions below fail.
// That is what makes the oracle sabotage-detecting.
//
// It is also a real KAT, not a liveness check: it parses+compiles+runs three
// fixed jq PROGRAMS over three fixed JSON fixtures through gojq's public
// library API (the same API the fuzz harnesses drive) and asserts the exact
// computed results. A patch that stubs the parser/interpreter to dodge a
// crash cannot reproduce these values.
//
// Prints three lines, which test.sh matches EXACTLY:
//
//	KAT_MAP_FIELD=<result of  [.[]|.a]  over [{"a":1},{"a":2}]>
//	KAT_TOSTRING=<result of   .a|tostring  over {"a":5}>
//	KAT_MAP_ARITH=<result of  map(.*2)  over [1,2,3]>
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/itchyny/gojq"
)

// runOne parses+compiles src, runs it over input, and returns the JSON
// encoding of its single output value. Any parse/compile/runtime error, or
// producing zero/multiple outputs, is a hard failure — this probe expects an
// exact, single, deterministic answer for each fixed case.
func runOne(label, src string, input any) string {
	q, err := gojq.Parse(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kat: %s: parse %q: %v\n", label, src, err)
		os.Exit(1)
	}
	code, err := gojq.Compile(q)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kat: %s: compile %q: %v\n", label, src, err)
		os.Exit(1)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		fmt.Fprintf(os.Stderr, "kat: %s: %q produced no output\n", label, src)
		os.Exit(1)
	}
	if runErr, isErr := v.(error); isErr {
		fmt.Fprintf(os.Stderr, "kat: %s: %q runtime error: %v\n", label, src, runErr)
		os.Exit(1)
	}
	if _, ok := iter.Next(); ok {
		fmt.Fprintf(os.Stderr, "kat: %s: %q produced more than one output\n", label, src)
		os.Exit(1)
	}
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kat: %s: marshal result: %v\n", label, err)
		os.Exit(1)
	}
	return string(b)
}

func decode(label, jsonText string) any {
	var v any
	if err := json.Unmarshal([]byte(jsonText), &v); err != nil {
		fmt.Fprintf(os.Stderr, "kat: %s: fixture decode: %v\n", label, err)
		os.Exit(1)
	}
	return v
}

func main() {
	// 1) array/object field extraction — the everyday jq idiom.
	docs := decode("MAP_FIELD", `[{"a":1},{"a":2}]`)
	fmt.Printf("KAT_MAP_FIELD=%s\n", runOne("MAP_FIELD", "[.[]|.a]", docs))

	// 2) string function — tostring, over a number field.
	single := decode("TOSTRING", `{"a":5}`)
	fmt.Printf("KAT_TOSTRING=%s\n", runOne("TOSTRING", ".a|tostring", single))

	// 3) arithmetic across an array — map(.*2).
	nums := decode("MAP_ARITH", `[1,2,3]`)
	fmt.Printf("KAT_MAP_ARITH=%s\n", runOne("MAP_ARITH", "map(.*2)", nums))
}
