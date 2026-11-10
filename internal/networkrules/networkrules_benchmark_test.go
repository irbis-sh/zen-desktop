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
	for _, rule := range syntheticRules() {
		if _, err := nr.ParseRule(rule, nil); err != nil {
			b.Fatal(err)
		}
	}
	// Match against compacted rules, as the app does after loading its lists.
	nr.Compact()
	return nr
}

// syntheticRules returns rules of the kinds the default lists have but EasyList
// and EasyPrivacy don't.
func syntheticRules() []string {
	// The default lists have about 1,800 generic rules (no URL pattern), nearly
	// all $removeparam. About 1,600 remove one parameter by name, a third of
	// them limited by $domain. Those average about five domains, and one in
	// twenty domains ends in ".*". About 160 are regexps, which are tried on
	// every query, most of them limited by $domain.
	var rules []string
	for i := range 1760 {
		if i%11 == 0 {
			// Four in five regexps are limited by $domain.
			rule := fmt.Sprintf("$removeparam=/^param%d_/", i)
			if i%55 != 0 {
				rule += fmt.Sprintf(",domain=a%[1]d.com|b%[1]d.com|c%[1]d.com|d%[1]d.com|e%[1]d.com", i)
			}
			rules = append(rules, rule)
			continue
		}
		rule := fmt.Sprintf("$removeparam=param%d", i)
		switch {
		case i%12 == 0:
			rule += fmt.Sprintf(",domain=site%d.*", i)
		case i%3 == 0:
			rule += fmt.Sprintf(",domain=a%[1]d.com|b%[1]d.com|c%[1]d.com|d%[1]d.com|e%[1]d.com", i)
		}
		rules = append(rules, rule)
	}
	// They also have a few rules with a bare "*" pattern, which match every URL.
	// These are the ones Zen parses.
	return append(rules,
		"*$ping,third-party",
		"*$third-party,domain=adblock.turtlecute.org",
		"*$script,third-party,domain=scnlog.me",
		"*$script,third-party,domain=ziperto.com",
		"*$script,third-party,domain=apne.co",
		"*$third-party,xmlhttprequest,domain=sanet.st",
	)
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
