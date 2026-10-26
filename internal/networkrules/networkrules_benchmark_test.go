/*
Benchmarks for matching requests and responses against EasyList and EasyPrivacy.

Run benchmarks with:

	go test -run '^$' -bench . ./internal/networkrules

They reuse the lists and URLs in internal/ruletree/testdata.
*/

package networkrules_test

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/networkrules"
)

func BenchmarkModifyReq(b *testing.B) {
	nr := loadRules(b)
	urls := loadURLs(b)
	b.ReportAllocs()

	var i int
	for b.Loop() {
		nr.ModifyReq(newRequest(urls[i%len(urls)]))
		i++
	}
}

func BenchmarkModifyRes(b *testing.B) {
	nr := loadRules(b)
	urls := loadURLs(b)
	b.ReportAllocs()

	var i int
	for b.Loop() {
		res := &http.Response{Header: http.Header{"Content-Type": []string{"text/javascript"}}}
		if _, err := nr.ModifyRes(newRequest(urls[i%len(urls)]), res); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func loadRules(b *testing.B) *networkrules.NetworkRules {
	b.Helper()

	nr := networkrules.New()
	for _, filename := range []string{"../ruletree/testdata/easylist.txt", "../ruletree/testdata/easyprivacy.txt"} {
		data, err := os.ReadFile(filename)
		if err != nil {
			b.Fatal(err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			if line := scanner.Text(); line != "" {
				// Skip rules with unsupported modifiers, as the filter does.
				_, _ = nr.ParseRule(line, nil)
			}
		}
		if err := scanner.Err(); err != nil {
			b.Fatal(err)
		}
	}
	// The default lists have about 2,400 generic rules (no URL pattern), mostly
	// $removeparam, a third of them limited by $domain. The lists above have
	// none, so add a similar set.
	for i := range 2400 {
		rule := fmt.Sprintf("$removeparam=param%d", i)
		if i%3 == 0 {
			rule += fmt.Sprintf(",domain=site%d.com", i)
		}
		if _, err := nr.ParseRule(rule, nil); err != nil {
			b.Fatal(err)
		}
	}
	// Match against compacted rules, as the app does after loading its lists.
	nr.Compact()
	return nr
}

func loadURLs(b *testing.B) []*url.URL {
	b.Helper()

	data, err := os.ReadFile("../ruletree/testdata/urls.txt")
	if err != nil {
		b.Fatal(err)
	}
	var urls []*url.URL
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			u, err := url.Parse(line)
			if err != nil {
				b.Fatal(err)
			}
			urls = append(urls, u)
		}
	}
	if err := scanner.Err(); err != nil {
		b.Fatal(err)
	}
	return urls
}

// newRequest builds a fresh request for each iteration because rules can
// rewrite the URL and headers. The Referer makes $domain conditions do their
// usual work.
func newRequest(u *url.URL) *http.Request {
	reqURL := *u
	return &http.Request{
		Method: http.MethodGet,
		URL:    &reqURL,
		Header: http.Header{
			"Referer":        []string{"https://example.com/"},
			"Sec-Fetch-Dest": []string{"script"},
		},
	}
}
