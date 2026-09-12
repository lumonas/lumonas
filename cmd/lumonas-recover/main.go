package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lumonas/lumonas/internal/recovery"
)

func main() {
	bundlePath := flag.String("bundle", "", "path to an encrypted .mrb recovery bundle")
	keyPath := flag.String("key-file", "", "path to the independent recovery key")
	root := flag.String("root", "", "absolute offline filesystem root to restore into")
	apply := flag.Bool("apply", false, "apply the verified plan; without this flag only a plan is printed")
	flag.Parse()

	if *bundlePath == "" || *keyPath == "" {
		fatal("--bundle and --key-file are required")
	}
	bundle, err := os.ReadFile(*bundlePath)
	if err != nil {
		fatal("read bundle: %v", err)
	}
	key, err := os.ReadFile(*keyPath)
	if err != nil {
		fatal("read recovery key: %v", err)
	}
	if *apply {
		if *root == "" {
			fatal("--root is required with --apply")
		}
		result, err := recovery.Apply(bundle, key, recovery.ApplyOptions{Root: *root})
		if err != nil {
			fatal("apply recovery bundle: %v", err)
		}
		writeJSON(result)
		return
	}
	plan, err := recovery.Plan(bundle, key)
	if err != nil {
		fatal("verify recovery bundle: %v", err)
	}
	writeJSON(plan)
}

func writeJSON(value any) {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		fatal("write result: %v", err)
	}
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "lumonas-recover: "+format+"\n", args...)
	os.Exit(1)
}
