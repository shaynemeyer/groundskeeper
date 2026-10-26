// Command record saves what go-ios reads from the connected iPhone as a test
// fixture, after the fixture scrubber has removed device and personal data.
// It prints counts only.
//
//	go run ./internal/iphone/record [out.json]
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/shaynemeyer/groundskeeper/internal/fixtures"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

const defaultOut = "internal/iphone/testdata/phone.json"

func main() {
	out := defaultOut
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	raw, err := iphone.Read()
	if err != nil {
		log.Fatal(err)
	}
	b, err := scrub(raw)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(out, b, 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("recorded %d apps, %d shared files (%d apps skipped) to %s\n", len(raw.Apps), len(raw.Files), len(raw.Skipped), out)
}

// scrub turns raw into plain JSON values, scrubs them, and returns indented JSON.
func scrub(raw iphone.Raw) ([]byte, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	clean, err := fixtures.New().Scrub(v)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(clean, "", "  ")
}
