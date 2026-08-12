package envi_test

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	envi "github.com/efureev/envi/v2"
)

// Editing a document leaves everything it does not touch exactly as it was.
func Example() {
	const src = `###   ---[ Application ]---   ###
# Human readable name
APP_NAME="My App"
APP_DEBUG=false
`

	env, err := envi.ParseString(src)
	if err != nil {
		log.Fatal(err)
	}

	env.Set("APP_DEBUG", "true")

	fmt.Print(env)
	// Output:
	// ###   ---[ Application ]---   ###
	// # Human readable name
	// APP_NAME="My App"
	// APP_DEBUG=true
}

// Keys are normalised, so how a caller spells one does not matter.
func ExampleEnv_Lookup() {
	env, err := envi.ParseString("APP_PORT=8080\n")
	if err != nil {
		log.Fatal(err)
	}

	port, ok := env.Lookup("app-port")
	fmt.Println(port, ok)

	_, ok = env.Lookup("MISSING")
	fmt.Println(ok)
	// Output:
	// 8080 true
	// false
}

// Reading a document preserves its order; sorting is something you ask for.
func ExampleEnv_All() {
	env, err := envi.ParseString("ZULU=1\nALPHA=2\n")
	if err != nil {
		log.Fatal(err)
	}

	for key, value := range env.All() {
		fmt.Println(key, value)
	}

	env.SortByKey()
	fmt.Println("--")
	for key := range env.All() {
		fmt.Println(key)
	}
	// Output:
	// ZULU 1
	// ALPHA 2
	// --
	// ALPHA
	// ZULU
}

// A shadow is a commented-out alternative kept beside the live value.
func ExampleRow_Shadows() {
	const src = `# for docker
# REDIS_HOST=redis
REDIS_HOST=127.0.0.1
`
	env, err := envi.ParseString(src)
	if err != nil {
		log.Fatal(err)
	}

	row := env.Get("REDIS_HOST")
	fmt.Println("value:", row.Value())
	for shadow := range row.Shadows() {
		fmt.Println("shadow:", shadow)
	}
	// Output:
	// value: 127.0.0.1
	// shadow: redis
}

// A malformed line reports where it went wrong.
func ExampleSyntaxError() {
	_, err := envi.ParseString("GOOD=1\nnot an assignment\n")

	var syntaxErr *envi.SyntaxError
	if errors.As(err, &syntaxErr) {
		fmt.Println("line:", syntaxErr.Line)
		fmt.Println("problem:", syntaxErr.Msg)
	}
	// Output:
	// line: 2
	// problem: expected '=' after key
}

// Options belong to the operation, not to the package, so two callers never
// disturb each other.
func ExampleNewEncoder() {
	env, err := envi.ParseString("A=plain\nB=1\n")
	if err != nil {
		log.Fatal(err)
	}

	var out strings.Builder
	if err := envi.NewEncoder(&out, envi.WithQuoting(envi.QuoteAlways)).Encode(env); err != nil {
		log.Fatal(err)
	}
	fmt.Print(out.String())
	// Output:
	// A="plain"
	// B=1
}

// Comments and shadows can be left out of the output without changing the
// document itself.
func ExampleWithComments() {
	const src = `# why this exists
# A=alternative
A=live
`
	env, err := envi.ParseString(src)
	if err != nil {
		log.Fatal(err)
	}

	var out strings.Builder
	err = envi.NewEncoder(&out, envi.WithComments(false), envi.WithShadows(false)).Encode(env)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out.String())
	// Output:
	// A=live
}

// Save writes atomically: the file is complete or untouched.
func ExampleSave() {
	dir, err := os.MkdirTemp("", "envi")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Errors are printed rather than fatal, so the deferred cleanup still runs
	// — and an unexpected one fails the example through its output.
	env := envi.New(envi.NewRow("APP_NAME", "example"))
	path := filepath.Join(dir, ".env")
	if err := envi.Save(env, path); err != nil {
		fmt.Println("save:", err)
		return
	}

	written, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	fmt.Print(string(written))
	// Output: APP_NAME=example
}
