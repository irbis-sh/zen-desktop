package filter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/irbis-sh/process"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
	"github.com/irbis-sh/zen-desktop/internal/redacted"
)

// filterActionObserver observes filter events.
type filterActionObserver interface {
	OnFilterBlock(method, url, referer string, rules []rule.Rule, pid process.PID)
	OnFilterRedirect(method, url, to, referer string, rules []rule.Rule, pid process.PID)
	OnFilterModify(method, url, referer string, rules []rule.Rule, pid process.PID)
}

type filterListStore interface {
	Get(url string) (io.ReadCloser, error)
}

type networkRules interface {
	ParseRule(rule string, filterName *string) (isException bool, err error)
	ModifyReq(req *http.Request) (appliedRules []rule.Rule, shouldBlock bool, redirectURL string)
	ModifyRes(req *http.Request, res *http.Response) ([]rule.Rule, error)
	CreateBlockResponse(req *http.Request) *http.Response
	CreateRedirectResponse(req *http.Request, to string) *http.Response
	CreateBlockPageResponse(req *http.Request, appliedRules []rule.Rule, whitelistPort int) (*http.Response, error)
	Compact()
}

// documentInjector handles non-network rules and HTML injection.
type documentInjector interface {
	AddRule(rule string, filterListTrusted bool) (handled bool, err error)
	Inject(*http.Request, *http.Response) error
}

type whitelistSrv interface {
	GetPort() int
}

// Filter is capable of parsing Adblock-style filter lists and hosts rules and matching URLs against them.
//
// Safe for concurrent use.
type Filter struct {
	networkRules    networkRules
	injector        documentInjector
	filterListStore filterListStore
	actionObserver  filterActionObserver
	whitelistSrv    whitelistSrv
}

var (
	// ignoreLineRegex matches comments and [Adblock Plus 2.0]-style headers.
	ignoreLineRegex = regexp.MustCompile(`^(?:!|\[|#[^#%@$])`)
)

// NewFilter creates and initializes a new filter.
func NewFilter(networkRules networkRules, injector documentInjector, filterListStore filterListStore, actionObserver filterActionObserver, whitelistSrv whitelistSrv) (*Filter, error) {
	if actionObserver == nil {
		return nil, errors.New("actionObserver is nil")
	}
	if networkRules == nil {
		return nil, errors.New("networkRules is nil")
	}
	if injector == nil {
		return nil, errors.New("injector is nil")
	}
	if filterListStore == nil {
		return nil, errors.New("filterListStore is nil")
	}
	if whitelistSrv == nil {
		return nil, errors.New("whitelistSrv is nil")
	}

	f := &Filter{
		networkRules:    networkRules,
		injector:        injector,
		actionObserver:  actionObserver,
		whitelistSrv:    whitelistSrv,
		filterListStore: filterListStore,
	}

	return f, nil
}

const includeMaxDepth = 20

// AddURL fetches a filter list from a URL, expands !#include directives, and adds rules to the filter.
func (f *Filter) AddURL(listURL string, listName string, listTrusted bool) error {
	if listURL == "" {
		return errors.New("url is empty")
	}

	var ruleCount, exceptionCount int
	var countsMu sync.Mutex

	addRuleLine := func(line string) {
		if len(line) == 0 || ignoreLineRegex.MatchString(line) {
			return
		}
		if isException, err := f.addRule(line, &listName, listTrusted); err != nil { // nolint:revive
			// log.Printf("error adding rule: %v", err)
		} else {
			countsMu.Lock()
			if isException {
				exceptionCount++
			} else {
				ruleCount++
			}
			countsMu.Unlock()
		}
	}

	visited := make(map[string]struct{})
	var visitedMu sync.Mutex

	var wg sync.WaitGroup
	var parseURL func(currentURL string, depth int)

	parseURL = func(currentURL string, depth int) {
		defer wg.Done()
		if depth > includeMaxDepth {
			log.Printf("filter: max depth %d exceeded when adding %q", includeMaxDepth, currentURL)
			return
		}

		base, err := url.Parse(currentURL)
		if err != nil {
			log.Printf("filter: error parsing url %q: %v", currentURL, err)
			return
		}

		visitedMu.Lock()
		if _, ok := visited[currentURL]; ok {
			visitedMu.Unlock()
			log.Printf("filter: duplicate include %q skipped", currentURL)
			return
		}
		visited[currentURL] = struct{}{}
		visitedMu.Unlock()

		contents, err := f.filterListStore.Get(currentURL)
		if err != nil {
			log.Printf("failed to get filter list %q from store: %v", currentURL, err)
			return
		}
		defer contents.Close()

		scanner := bufio.NewScanner(contents)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if after, ok := strings.CutPrefix(line, "!#include"); ok {
				includeURL, err := resolveInclude(base, after)
				if err != nil {
					log.Printf("filter: error resolving include: %v", err)
					continue
				}

				wg.Add(1)
				go parseURL(includeURL, depth+1)
				continue
			}

			addRuleLine(line)
		}
		if err := scanner.Err(); err != nil {
			log.Printf("filter: error scanning %q: %v", currentURL, err)
		}
	}

	wg.Add(1)
	go parseURL(listURL, 0)
	wg.Wait()

	log.Printf("filter: added %d rules, %d exceptions from %s", ruleCount, exceptionCount, listName)
	return nil
}

// AddReader parses the rules from the given reader and adds them to the filter.
func (f *Filter) AddReader(listRules io.Reader, listName string, listTrusted bool) error {
	var ruleCount, exceptionCount int
	scanner := bufio.NewScanner(listRules)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || ignoreLineRegex.MatchString(line) {
			continue
		}

		if isException, err := f.addRule(line, &listName, listTrusted); err != nil { // nolint:revive
			// log.Printf("error adding rule: %v", err)
		} else if isException {
			exceptionCount++
		} else {
			ruleCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	log.Printf("filter: added %d rules, %d exceptions from %s", ruleCount, exceptionCount, listName)
	return nil
}

// addRule adds a new rule to the filter.
func (f *Filter) addRule(rule string, filterListName *string, filterListTrusted bool) (isException bool, err error) {
	if handled, err := f.injector.AddRule(rule, filterListTrusted); err != nil {
		return false, err
	} else if handled {
		return false, nil
	}

	isExceptionRule, err := f.networkRules.ParseRule(rule, filterListName)
	if err != nil {
		return false, fmt.Errorf("parse network rule: %w", err)
	}
	return isExceptionRule, nil
}

// HandleRequest handles the given request by matching it against the filter rules.
// If the request should be blocked, it returns a response that blocks the request. If the request should be modified, it modifies it in-place.
func (f *Filter) HandleRequest(req *http.Request, pid process.PID) (*http.Response, error) {
	initialURL := req.URL.String()

	appliedRules, shouldBlock, redirectURL := f.networkRules.ModifyReq(req)
	if shouldBlock {
		f.actionObserver.OnFilterBlock(req.Method, initialURL, req.Header.Get("Referer"), appliedRules, pid)

		if isUserNavigation(req) {
			port := f.whitelistSrv.GetPort()
			if port <= 0 {
				log.Printf("whitelist server not ready, falling back to simple block response for %q", redacted.Redacted(req.URL))
				return f.networkRules.CreateBlockResponse(req), nil
			}

			res, err := f.networkRules.CreateBlockPageResponse(req, appliedRules, f.whitelistSrv.GetPort())
			if err != nil {
				return nil, fmt.Errorf("create block page response: %v", err)
			}
			return res, nil
		}
		return f.networkRules.CreateBlockResponse(req), nil
	}

	if redirectURL != "" {
		f.actionObserver.OnFilterRedirect(req.Method, initialURL, redirectURL, req.Header.Get("Referer"), appliedRules, pid)
		return f.networkRules.CreateRedirectResponse(req, redirectURL), nil
	}

	if len(appliedRules) > 0 {
		f.actionObserver.OnFilterModify(req.Method, initialURL, req.Header.Get("Referer"), appliedRules, pid)
	}

	return nil, nil
}

// Finalize optimizes internal data structures after all filter lists have been loaded.
// This method should be called once after all AddURL/AddReader calls are complete and before
// the filter starts handling requests. Calling Finalize is not required for correctness,
// but improves memory usage and lookup performance.
func (f *Filter) Finalize() {
	f.networkRules.Compact()
}

// HandleResponse handles the given response by matching it against the filter rules.
// If the response should be modified, it modifies it in-place.
//
// As of April 2024, there are no response-only rules that can block or redirect responses.
// For that reason, this method does not return a blocking or redirecting response itself.
func (f *Filter) HandleResponse(req *http.Request, res *http.Response, pid process.PID) error {
	if isDocumentNavigation(req, res) {
		if err := f.injector.Inject(req, res); err != nil {
			// This injection error is recoverable, so we log it and continue processing the response.
			log.Printf("error injecting assets for %q: %v", redacted.Redacted(req.URL), err)
		}
	}

	appliedRules, err := f.networkRules.ModifyRes(req, res)
	if err != nil {
		return fmt.Errorf("apply network rules: %v", err)
	}
	if len(appliedRules) > 0 {
		f.actionObserver.OnFilterModify(req.Method, req.URL.String(), req.Header.Get("Referer"), appliedRules, pid)
	}

	return nil
}

func isDocumentNavigation(req *http.Request, res *http.Response) bool {
	// Reference: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Sec-Fetch-Dest#directives
	// Note: Although not explicitly stated in the spec, Fetch Metadata Request Headers are only included in requests sent to HTTPS endpoints.
	if req.URL.Scheme == "https" {
		secFetchDest := req.Header.Get("Sec-Fetch-Dest")
		if secFetchDest != "document" && secFetchDest != "iframe" {
			return false
		}
	}

	contentType := res.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	if mediaType != "text/html" {
		return false
	}

	return true
}

func isUserNavigation(req *http.Request) bool {
	dest := req.Header.Get("Sec-Fetch-Dest")
	mode := req.Header.Get("Sec-Fetch-Mode")
	user := req.Header.Get("Sec-Fetch-User")

	if dest == "document" && (mode == "navigate" || user == "?1") {
		return true
	}
	return false
}
