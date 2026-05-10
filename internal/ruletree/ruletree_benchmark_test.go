/*
Benchmarks for building and querying the rule tree.

Run benchmarks with:

	// Run all benchmarks
	go test -bench=. ./internal/ruletree

	// Run BenchmarkMatch and export a memory profile
	go test -bench=Match$ -memprofile=mem.out ./internal/ruletree

	// Run BenchmarkLoadTree and export a CPU profile
	go test -bench=LoadTree -cpuprofile=cpu.out ./internal/ruletree

Inspect profile:

	go tool pprof -lines -focus=FindMatchingRulesReq mem.out

pprof tips:
  - -lines: show line-level metric attribution
  - -focus=FindMatchingRulesReq: restrict output to FindMatchingRulesReq; filters out setup/teardown noise
  - -ignore=runtime: hide nodes matching "runtime" (includes GC)
  - top: show top entries (usually somewhat hard to make sense of)
  - list <func>: show annotated source for the given function
  - web: generate an SVG call graph and open in browser
*/
package ruletree_test

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"runtime"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/ruletree"
)

const baseSeed = 42

var (
	rnd         = rand.New(rand.NewSource(baseSeed)) // #nosec G404 -- Not used for cryptographic purposes.
	filterLists = []string{"testdata/easylist.txt", "testdata/easyprivacy.txt"}
)

func BenchmarkLoadTree(b *testing.B) {
	rawLists := make([][]byte, 0, len(filterLists))
	var totalBytes int64
	for _, filename := range filterLists {
		data, err := os.ReadFile(filename)
		if err != nil {
			b.Fatalf("read %s: %v", filename, err)
		}
		totalBytes += int64(len(data))
		rawLists = append(rawLists, data)
	}
	b.SetBytes(totalBytes)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	var lines int
	var trees []*ruletree.Tree[string]
	for b.Loop() {
		tree := ruletree.New[string]()
		for _, data := range rawLists {
			scanner := bufio.NewScanner(bytes.NewReader(data))

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				tree.Insert(line, line)
				lines++
			}

			if err := scanner.Err(); err != nil {
				b.Fatalf("scan: %v", err)
			}
		}
		trees = append(trees, tree)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(trees)

	heapAlloc := float64(after.HeapInuse - before.HeapInuse)
	b.ReportMetric(heapAlloc/(1024*1024), "MB_allocs")
	b.ReportMetric(heapAlloc/float64(lines), "B_allocs/line")

	b.ReportAllocs()
}

func BenchmarkMatch(b *testing.B) {
	tree, err := loadTree()
	if err != nil {
		b.Fatalf("load tree: %v", err)
	}

	urls, avgBytes, err := loadURLs()
	if err != nil {
		b.Fatalf("load urls: %v", err)
	}
	b.SetBytes(int64(avgBytes))

	var i int
	for b.Loop() {
		u := urls[i%len(urls)]
		tree.Get(u)
		i++
	}

	b.ReportAllocs()
}

func BenchmarkMatchParallel(b *testing.B) {
	tree, err := loadTree()
	if err != nil {
		b.Fatalf("load tree: %v", err)
	}

	urls, avgBytes, err := loadURLs()
	if err != nil {
		b.Fatalf("load urls: %v", err)
	}
	b.SetBytes(int64(avgBytes))

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			u := urls[i%len(urls)]
			tree.Get(u)
			i++
		}
	})

	b.ReportAllocs()
}

func loadTree() (*ruletree.Tree[string], error) {
	tree := ruletree.New[string]()

	for _, filename := range filterLists {
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read %s: %v", filename, err)
		}

		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			tree.Insert(line, line)
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan %s: %v", filename, err)
		}
	}
	return tree, nil
}

func loadURLs() ([]string, int, error) {
	const filename = "testdata/urls.txt"

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %v", filename, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))

	var urls []string
	var totalURLBytes int
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if _, err := url.Parse(line); err != nil {
			return nil, 0, fmt.Errorf("invalid url %q: %v", line, err)
		}

		urls = append(urls, line)
		totalURLBytes += len(line)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan %s: %v", filename, err)
	}

	avg := totalURLBytes / len(urls)

	rnd.Shuffle(len(urls), func(i, j int) {
		urls[i], urls[j] = urls[j], urls[i]
	})

	return urls, avg, nil
}
