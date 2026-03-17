package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultRegistryPath = "/home/polis/tools/gate/registry.yaml"

type registry struct {
	Entries []registryEntry `yaml:"entries"`
}

type registryEntry struct {
	Name           string   `yaml:"name"`
	Bins           []string `yaml:"bins"`
	Source         string   `yaml:"source"`
	Targets        []string `yaml:"targets"`
	VerifyCommands []string `yaml:"verify_commands"`
}

type catalogResult struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Details []string `json:"details,omitempty"`
}

func runCatalogCheck(ctx context.Context, args []string) int {
	registryPath := defaultRegistryPath
	var jsonOutput bool

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--registry":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--registry requires a value")
				return 1
			}
			registryPath = args[i]
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
				return 1
			}
		}
		i++
	}

	data, err := os.ReadFile(registryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read registry: %v\n", err)
		return 1
	}

	var reg registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse registry: %v\n", err)
		return 1
	}

	results := make([]catalogResult, 0, len(reg.Entries))
	allPass := true

	for _, e := range reg.Entries {
		r := checkEntry(ctx, e)
		if r.Status != "PASS" {
			allPass = false
		}
		results = append(results, r)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
	} else {
		printCatalogTable(results)
	}

	// Create beads for BROKEN entries if br is available
	if !allPass {
		createBrokenBeads(results)
	}

	if allPass {
		return 0
	}
	return 1
}

func checkEntry(ctx context.Context, e registryEntry) catalogResult {
	r := catalogResult{Name: e.Name, Status: "PASS"}

	// Check bins on PATH
	for _, bin := range e.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			r.Status = "BROKEN"
			r.Details = append(r.Details, fmt.Sprintf("bin %q not on PATH", bin))
		}
	}

	// If already BROKEN from missing bins, skip verify_commands
	if r.Status != "BROKEN" {
		for _, cmd := range e.VerifyCommands {
			vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			c := exec.CommandContext(vctx, "sh", "-c", cmd)
			if err := c.Run(); err != nil {
				r.Status = "BROKEN"
				r.Details = append(r.Details, fmt.Sprintf("verify %q failed: %v", cmd, err))
			}
			cancel()
		}
	}

	// Check source exists
	if e.Source != "" {
		if _, err := os.Stat(e.Source); err != nil {
			if r.Status != "BROKEN" {
				r.Status = "STALE"
			}
			r.Details = append(r.Details, fmt.Sprintf("source missing: %s", e.Source))
		}
	}

	// Check targets exist and are non-empty
	for _, t := range e.Targets {
		fi, err := os.Stat(t)
		if err != nil {
			if r.Status != "BROKEN" {
				r.Status = "STALE"
			}
			r.Details = append(r.Details, fmt.Sprintf("target missing: %s", t))
		} else if fi.Size() == 0 {
			if r.Status != "BROKEN" {
				r.Status = "STALE"
			}
			r.Details = append(r.Details, fmt.Sprintf("target empty: %s", t))
		}
	}

	return r
}

func printCatalogTable(results []catalogResult) {
	// Find max name length
	maxName := 4
	for _, r := range results {
		if len(r.Name) > maxName {
			maxName = len(r.Name)
		}
	}

	fmt.Printf("\n%-*s  STATUS   DETAILS\n", maxName, "NAME")
	fmt.Printf("%s  %s  %s\n", strings.Repeat("─", maxName), "──────", strings.Repeat("─", 40))

	for _, r := range results {
		color := "\033[32m" // green
		if r.Status == "STALE" {
			color = "\033[33m" // yellow
		} else if r.Status == "BROKEN" {
			color = "\033[31m" // red
		}
		detail := strings.Join(r.Details, "; ")
		fmt.Printf("%-*s  %s%-6s\033[0m  %s\n", maxName, r.Name, color, r.Status, detail)
	}
	fmt.Println()
}

func createBrokenBeads(results []catalogResult) {
	if _, err := exec.LookPath("br"); err != nil {
		return
	}
	for _, r := range results {
		if r.Status != "BROKEN" {
			continue
		}
		detail := strings.Join(r.Details, "; ")
		title := fmt.Sprintf("catalog-check: %s is BROKEN — %s", r.Name, detail)
		cmd := exec.Command("br", "create", "--type", "gate", "--title", title, "--label", "catalog-check")
		cmd.Run() // best-effort
	}
}
