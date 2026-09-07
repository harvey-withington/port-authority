package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"portauthority/core/insight"
)

// runInsights prints plain-language findings for the current topology.
func runInsights(args []string) error {
	fs := flag.NewFlagSet("insights", flag.ExitOnError)
	fixture := fs.String("fixture", "", "replay a saved snapshot instead of reading hardware")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := snapshot(*fixture)
	if err != nil {
		return err
	}
	insights := insight.Evaluate(t)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(insights)
	}
	if len(insights) == 0 {
		fmt.Println("No problems found. Everything is connected at the speed it should be.")
		return nil
	}
	for i, in := range insights {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("[%s] %s\n", strings.ToUpper(string(in.Severity)), in.Title)
		fmt.Printf("  %s\n", in.Explanation)
		fmt.Printf("  What to do: %s\n", in.Suggestion)
		if len(in.Evidence) > 0 {
			fmt.Printf("  Details (%.0f%% confident):\n", in.Confidence*100)
			for _, e := range in.Evidence {
				fmt.Printf("    - %s\n", e)
			}
		}
	}
	return nil
}
