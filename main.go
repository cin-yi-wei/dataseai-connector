package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	token := flag.String("token", "", "agent token (env: DATASEAI_TOKEN)")
	server := flag.String("server", "wss://dataseai.app/agent", "broker URL")
	flag.Parse()

	if *token == "" {
		if t := os.Getenv("DATASEAI_TOKEN"); t != "" {
			*token = t
		}
	}

	fmt.Printf("dataseai-connector %s (commit=%s, date=%s)\n", version, commit, date)
	fmt.Printf("system: %s/%s, go=%s\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Printf("server: %s\n", *server)
	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: --token required (or DATASEAI_TOKEN env)")
		os.Exit(1)
	}
	fmt.Printf("token:  %s...%s\n", (*token)[:min(6, len(*token))], (*token)[max(0, len(*token)-4):])
	fmt.Println("(would connect to broker now; sandbox stub exits)")
}

func min(a, b int) int { if a < b { return a }; return b }
func max(a, b int) int { if a > b { return a }; return b }
