package filter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/irbis-sh/zen-desktop/internal/filterliststore"
	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
	"github.com/irbis-sh/zen-desktop/internal/process"
)

func TestAddURLThreadsCtxAndModeToEveryFetch(t *testing.T) {
	t.Parallel()

	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/list.txt":  {content: "||ads.example.com^\n! comment\n!#include extra.txt\n"},
		"https://example.com/extra.txt": {content: "||track.example.com^\n"},
	}}
	f, rules := newTestFilter(t, store)

	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	outcome := f.AddURL(ctx, "https://example.com/list.txt", "test", true, filterliststore.ModePreferCache)

	if outcome != (Outcome{}) {
		t.Errorf("expected zero outcome, got %+v", outcome)
	}
	assertRules(t, rules.got(), "||ads.example.com^", "||track.example.com^")
	// The include's fetch must observe the same ctx and mode as the root's:
	// stage 6's per-pass mode ladder and deadline depend on it.
	for _, url := range []string{"https://example.com/list.txt", "https://example.com/extra.txt"} {
		rec := store.fetched(url)
		if rec.mode != filterliststore.ModePreferCache {
			t.Errorf("expected ModePreferCache threaded to Get(%q), got %v", url, rec.mode)
		}
		if rec.ctxVal != "marker" {
			t.Errorf("expected the caller's ctx threaded to Get(%q), got value %v", url, rec.ctxVal)
		}
	}
}

func TestMidBodyTruncationMarksTruncated(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		want    []string
	}{
		// The scanner hands over the buffered partial line before surfacing
		// the error, so a fragment of a rule does get applied - part of why
		// Truncated must discard the whole structure, not just this list.
		{"MidLine", "||a.example.com^\n||partial", []string{"||a.example.com^", "||partial"}},
		{"AtLineBoundary", "||a.example.com^\n", []string{"||a.example.com^"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			readErr := errors.New("connection reset")
			store := &fakeStore{entries: map[string]listEntry{
				"https://example.com/list.txt": {content: c.content, readErr: readErr},
			}}
			f, rules := newTestFilter(t, store)

			outcome := f.AddURL(context.Background(), "https://example.com/list.txt", "test", true, filterliststore.ModeDefault)

			if !outcome.Truncated {
				t.Error("expected Truncated")
			}
			if outcome.Failed {
				t.Error("unexpected Failed")
			}
			if !errors.Is(outcome.Err, readErr) {
				t.Errorf("expected Err to wrap the read error, got %v", outcome.Err)
			}
			assertRules(t, rules.got(), c.want...)
		})
	}
}

func TestOversizedLineIsNotTruncation(t *testing.T) {
	t.Parallel()

	content := "||before.example.com^\n" + strings.Repeat("a", maxRuleLength+1) + "\n||after.example.com^\n"
	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/list.txt": {content: content},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/list.txt", "test", true, filterliststore.ModeDefault)

	if outcome != (Outcome{}) {
		t.Errorf("expected zero outcome for a parser-side error, got %+v", outcome)
	}
	// bufio.ErrTooLong aborts the scan, so rules past the oversized line are
	// lost; refetching would lose them identically, hence no taint.
	assertRules(t, rules.got(), "||before.example.com^")
}

func TestLongLineWithinLimitParses(t *testing.T) {
	t.Parallel()

	// Over bufio.Scanner's 64 KiB default, under maxRuleLength: parses only
	// because AddURL raises the scanner's limit.
	longRule := "||" + strings.Repeat("a", 100*1024) + ".com^"
	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/list.txt": {content: longRule + "\n"},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/list.txt", "test", true, filterliststore.ModeDefault)

	if outcome != (Outcome{}) {
		t.Errorf("expected zero outcome, got %+v", outcome)
	}
	assertRules(t, rules.got(), longRule)
}

func TestOversizedLineDrainsStreamToEOF(t *testing.T) {
	t.Parallel()

	content := "||before.example.com^\n" + strings.Repeat("a", maxRuleLength+1) + "\n||after.example.com^\n"
	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/list.txt": {content: content},
	}}
	f, _ := newTestFilter(t, store)

	f.AddURL(context.Background(), "https://example.com/list.txt", "test", true, filterliststore.ModeDefault)

	// The store caches a download only at a verified EOF: returning at the
	// oversized line would leave the list uncacheable, refetched in full at
	// every startup with no offline copy.
	if !store.reachedEOF("https://example.com/list.txt") {
		t.Error("expected the stream to be drained to EOF")
	}
}

func TestEmptyBodyMarksFailed(t *testing.T) {
	t.Parallel()

	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/list.txt": {readErr: filterliststore.ErrEmptyBody},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/list.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.Failed {
		t.Error("expected Failed")
	}
	// Deterministic and rule-free: a rebuild would refetch the same emptiness,
	// so it must not read as truncation.
	if outcome.Truncated {
		t.Error("unexpected Truncated")
	}
	if !errors.Is(outcome.Err, filterliststore.ErrEmptyBody) {
		t.Errorf("expected Err to wrap ErrEmptyBody, got %v", outcome.Err)
	}
	assertRules(t, rules.got())
}

func TestListSizeCapIsNotTruncation(t *testing.T) {
	t.Parallel()

	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/list.txt": {content: "||before.example.com^\n", readErr: filterliststore.ErrListTooLarge},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/list.txt", "test", true, filterliststore.ModeDefault)

	// Like an oversized line: the rules up to the cap were applied and a
	// refetch would break at the same byte, so no taint.
	if outcome != (Outcome{}) {
		t.Errorf("expected zero outcome for the deterministic size cap, got %+v", outcome)
	}
	assertRules(t, rules.got(), "||before.example.com^")
}

func TestRootFetchFailureMarksFailed(t *testing.T) {
	t.Parallel()

	getErr := errors.New("no network, no cache")
	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/list.txt": {getErr: getErr},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/list.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.Failed {
		t.Error("expected Failed")
	}
	if outcome.Truncated {
		t.Error("unexpected Truncated")
	}
	if !errors.Is(outcome.Err, getErr) {
		t.Errorf("expected Err to wrap the fetch error, got %v", outcome.Err)
	}
	assertRules(t, rules.got())
}

func TestEmptyURLFails(t *testing.T) {
	t.Parallel()

	f, _ := newTestFilter(t, &fakeStore{})

	outcome := f.AddURL(context.Background(), "", "test", true, filterliststore.ModeDefault)

	if !outcome.Failed || outcome.Err == nil {
		t.Errorf("expected Failed with an error, got %+v", outcome)
	}
}

func TestUnparseableURLMarksFailed(t *testing.T) {
	t.Parallel()

	f, rules := newTestFilter(t, &fakeStore{})

	// Accepted by the frontend's WHATWG-based URL validation, rejected by
	// net/url for the invalid percent-escape.
	outcome := f.AddURL(context.Background(), "https://example.com/100%discount.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.Failed || outcome.Err == nil {
		t.Errorf("expected Failed with an error, got %+v", outcome)
	}
	if outcome.Truncated {
		t.Error("unexpected Truncated")
	}
	assertRules(t, rules.got())
}

func TestUnresolvableIncludeMarksFailed(t *testing.T) {
	t.Parallel()

	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/root.txt": {content: "||root.example.com^\n!#include https://evil.example.org/x.txt\n"},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/root.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.Failed || outcome.Err == nil {
		t.Errorf("expected Failed from the cross-origin include, got %+v", outcome)
	}
	assertRules(t, rules.got(), "||root.example.com^")
}

func TestIncludeDepthOverflowMarksFailed(t *testing.T) {
	t.Parallel()

	// A chain of includeMaxDepth+2 lists, each including the next: the last
	// one sits one level past the cap and must be dropped as Failed.
	entries := make(map[string]listEntry)
	for i := range includeMaxDepth + 2 {
		content := fmt.Sprintf("||l%d.example.com^\n", i)
		if i <= includeMaxDepth {
			content += fmt.Sprintf("!#include l%d.txt\n", i+1)
		}
		entries[fmt.Sprintf("https://example.com/l%d.txt", i)] = listEntry{content: content}
	}
	store := &fakeStore{entries: entries}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/l0.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.Failed || outcome.Err == nil {
		t.Errorf("expected Failed from the depth overflow, got %+v", outcome)
	}
	got := rules.got()
	if !slices.Contains(got, fmt.Sprintf("||l%d.example.com^", includeMaxDepth)) {
		t.Errorf("expected the list at the depth cap to be applied, got %q", got)
	}
	if slices.Contains(got, fmt.Sprintf("||l%d.example.com^", includeMaxDepth+1)) {
		t.Errorf("expected the list past the depth cap to be dropped, got %q", got)
	}
}

func TestIncludeFailureLeavesSiblingsUnaffected(t *testing.T) {
	t.Parallel()

	getErr := errors.New("include unreachable")
	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/root.txt":   {content: "||root.example.com^\n!#include broken.txt\n!#include good.txt\n"},
		"https://example.com/broken.txt": {getErr: getErr},
		"https://example.com/good.txt":   {content: "||good.example.com^\n"},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/root.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.Failed {
		t.Error("expected Failed from the broken include")
	}
	if outcome.Truncated {
		t.Error("unexpected Truncated")
	}
	if !errors.Is(outcome.Err, getErr) {
		t.Errorf("expected Err to wrap the include's fetch error, got %v", outcome.Err)
	}
	assertRules(t, rules.got(), "||root.example.com^", "||good.example.com^")
}

func TestIncludeTruncationTaintsOutcome(t *testing.T) {
	t.Parallel()

	readErr := errors.New("connection reset")
	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/root.txt":  {content: "||root.example.com^\n!#include child.txt\n"},
		"https://example.com/child.txt": {content: "||child.example.com^\n", readErr: readErr},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/root.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.Truncated {
		t.Error("expected Truncated from the broken include stream")
	}
	if outcome.Failed {
		t.Error("unexpected Failed")
	}
	if !errors.Is(outcome.Err, readErr) {
		t.Errorf("expected Err to wrap the read error, got %v", outcome.Err)
	}
	assertRules(t, rules.got(), "||root.example.com^", "||child.example.com^")
}

func TestStaleServeReported(t *testing.T) {
	t.Parallel()

	store := &fakeStore{entries: map[string]listEntry{
		"https://example.com/root.txt":  {content: "||root.example.com^\n!#include child.txt\n"},
		"https://example.com/child.txt": {content: "||child.example.com^\n", src: filterliststore.SourceStaleCache},
	}}
	f, rules := newTestFilter(t, store)

	outcome := f.AddURL(context.Background(), "https://example.com/root.txt", "test", true, filterliststore.ModeDefault)

	if !outcome.ServedStale {
		t.Error("expected ServedStale from the stale include")
	}
	if outcome.Truncated || outcome.Failed || outcome.Err != nil {
		t.Errorf("expected an otherwise clean outcome, got %+v", outcome)
	}
	assertRules(t, rules.got(), "||root.example.com^", "||child.example.com^")
}

func TestOutcomeMerge(t *testing.T) {
	t.Parallel()

	errA, errB := errors.New("a"), errors.New("b")
	merged := Outcome{Truncated: true, Err: errA}.Merge(Outcome{ServedStale: true, Err: errB})

	if !merged.Truncated || !merged.ServedStale || merged.Failed {
		t.Errorf("expected flags ORed, got %+v", merged)
	}
	if !errors.Is(merged.Err, errA) || !errors.Is(merged.Err, errB) {
		t.Errorf("expected both errors retained, got %v", merged.Err)
	}
}

func newTestFilter(t *testing.T, store *fakeStore) (*Filter, *fakeNetworkRules) {
	t.Helper()
	rules := &fakeNetworkRules{}
	f, err := NewFilter(rules, fakeInjector{}, store, fakeObserver{}, fakeWhitelist{})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	return f, rules
}

// assertRules compares as sets: includes are parsed on their own goroutines,
// so arrival order is not deterministic.
func assertRules(t *testing.T, got []string, want ...string) {
	t.Helper()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("rules mismatch:\ngot  %q\nwant %q", got, want)
	}
}

type listEntry struct {
	content string
	getErr  error                  // returned from Get instead of a reader
	readErr error                  // returned by the reader after content, in place of EOF
	src     filterliststore.Source // defaults to SourceNetwork
}

// ctxKey marks the caller's context so fetches can prove they received it.
type ctxKey struct{}

// fetchRecord captures what one Get call observed.
type fetchRecord struct {
	mode   filterliststore.FetchMode
	ctxVal any // the value under ctxKey in the received ctx
}

// fakeStore's entries are fixed at construction and only read afterwards;
// mu guards just the per-URL fetch and EOF records.
type fakeStore struct {
	entries map[string]listEntry

	mu      sync.Mutex
	fetches map[string]fetchRecord
	eofs    map[string]bool
}

func (s *fakeStore) fetched(url string) fetchRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetches[url]
}

// reachedEOF reports whether the reader served for url was consumed to EOF -
// the fake's stand-in for the store's "verified download, cached" condition.
func (s *fakeStore) reachedEOF(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eofs[url]
}

func (s *fakeStore) Get(ctx context.Context, url string, mode filterliststore.FetchMode) (io.ReadCloser, filterliststore.Source, error) {
	s.mu.Lock()
	if s.fetches == nil {
		s.fetches = make(map[string]fetchRecord)
	}
	s.fetches[url] = fetchRecord{mode: mode, ctxVal: ctx.Value(ctxKey{})}
	s.mu.Unlock()

	e, ok := s.entries[url]
	if !ok {
		return nil, filterliststore.SourceUnknown, fmt.Errorf("no fake entry for %q", url)
	}
	if e.getErr != nil {
		return nil, filterliststore.SourceUnknown, e.getErr
	}
	src := e.src
	if src == filterliststore.SourceUnknown {
		src = filterliststore.SourceNetwork
	}
	r := io.Reader(strings.NewReader(e.content))
	if e.readErr != nil {
		r = io.MultiReader(r, iotest.ErrReader(e.readErr))
	}
	r = &eofTracker{Reader: r, onEOF: func() {
		s.mu.Lock()
		if s.eofs == nil {
			s.eofs = make(map[string]bool)
		}
		s.eofs[url] = true
		s.mu.Unlock()
	}}
	return io.NopCloser(r), src, nil
}

// eofTracker invokes onEOF when the wrapped reader reports io.EOF.
type eofTracker struct {
	io.Reader
	onEOF func()
}

func (r *eofTracker) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if errors.Is(err, io.EOF) {
		r.onEOF()
	}
	return n, err
}

type fakeNetworkRules struct {
	mu    sync.Mutex
	rules []string
}

func (n *fakeNetworkRules) ParseRule(rule string, _ *string) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rules = append(n.rules, rule)
	return false, nil
}

func (n *fakeNetworkRules) got() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.rules)
}

func (n *fakeNetworkRules) ModifyReq(*http.Request) ([]rule.Rule, bool, string) {
	return nil, false, ""
}
func (n *fakeNetworkRules) ModifyRes(*http.Request, *http.Response) ([]rule.Rule, error) {
	return nil, nil
}
func (n *fakeNetworkRules) CreateBlockResponse(*http.Request) *http.Response { return nil }
func (n *fakeNetworkRules) CreateRedirectResponse(*http.Request, string) *http.Response {
	return nil
}
func (n *fakeNetworkRules) CreateBlockPageResponse(*http.Request, []rule.Rule, int) (*http.Response, error) {
	return nil, nil
}
func (n *fakeNetworkRules) Compact() {}

type fakeInjector struct{}

func (fakeInjector) AddRule(string, bool) (bool, error)         { return false, nil }
func (fakeInjector) Inject(*http.Request, *http.Response) error { return nil }

type fakeObserver struct{}

func (fakeObserver) OnFilterBlock(string, string, string, []rule.Rule, process.Info)            {}
func (fakeObserver) OnFilterRedirect(string, string, string, string, []rule.Rule, process.Info) {}
func (fakeObserver) OnFilterModify(string, string, string, []rule.Rule, process.Info)           {}

type fakeWhitelist struct{}

func (fakeWhitelist) GetPort() int { return 0 }
