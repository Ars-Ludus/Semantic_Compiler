package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	semindex "semcom_embed"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
		runBuild(os.Args[2:])
	case "query":
		runQuery(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	dsn := fs.String("dsn", "postgres://ars@localhost:5432/memory_db", "PostgreSQL DSN")
	out := fs.String("out", "index.bin", "output file path")
	fs.Parse(args)

	if err := semindex.Build(*dsn, *out); err != nil {
		fmt.Fprintf(os.Stderr, "build error: %v\n", err)
		os.Exit(1)
	}
}

func runQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	idxPath := fs.String("index", "index.bin", "index file path")
	t2 := fs.Float64("t2", 0.25, "L2 match ratio threshold")
	t1 := fs.Float64("t1", 0.2, "L1 match ratio threshold")
	t0 := fs.Float64("t0", 0.15, "L0 match ratio threshold")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "query: provide query text as argument")
		os.Exit(1)
	}
	text := strings.Join(fs.Args(), " ")

	loadStart := time.Now()
	idx, err := semindex.Load(*idxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load error: %v\n", err)
		os.Exit(1)
	}
	loadTime := time.Since(loadStart)

	th := semindex.Thresholds{L2: *t2, L1: *t1, L0: *t0}
	queryStart := time.Now()
	stats := idx.Query(text, th)
	queryTime := time.Since(queryStart)
	if len(stats.TokenIDs) == 0 {
		fmt.Fprintln(os.Stderr, "no results")
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "query tokens: %d | l3: %d | l2: %d | l1: %d | l0: %d | tokens: %d | load: %s | query: %s\n",
		stats.QueryTokens, stats.L3, stats.L2, stats.L1, len(stats.L0IDs), len(stats.TokenIDs), loadTime.Round(time.Microsecond), queryTime.Round(time.Microsecond))

	parts := make([]string, len(stats.TokenIDs))
	for i, t := range stats.TokenIDs {
		parts[i] = strconv.Itoa(int(t))
	}
	fmt.Println(strings.Join(parts, " "))
}

func usage() {
	fmt.Fprintln(os.Stderr, `semcom - hierarchical roaring bitmap semantic index

Usage:
  semcom build [flags]          build index from PostgreSQL
  semcom query [flags] <text>   query the index

Build flags:
  --dsn   PostgreSQL DSN (default: postgres://ars@localhost:5432/memory_db)
  --out   output file (default: index.bin)

Query flags:
  --index  index file (default: index.bin)
  --t2     L2 match ratio threshold (default: 0.25)
  --t1     L1 match ratio threshold (default: 0.2)
  --t0     L0 match ratio threshold (default: 0.15)`)
}
