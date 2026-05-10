package ruletree

import (
	"bufio"
	"os"
	"testing"
)

func TestInsert(t *testing.T) {
	t.Parallel()

	t.Run("non-initial domain boundary is rejected", func(t *testing.T) {
		t.Parallel()

		tr := New[string]()
		tr.Insert("a||b", "a||b")

		if len(tr.root.edges) != 0 {
			t.Fatal("expected tr.root.edges to be empty")
		}
	})

	t.Run("duplicate values", func(t *testing.T) {
		t.Parallel()

		tr := New[string]()
		tr.Insert("||example.com/ads/*", "R1a")

		tr.Insert("||example.com/ads/*", "R1b")

		got := tr.Get("http://example.com/ads/x")
		set := asSet(got)
		if _, ok := set["R1a"]; !ok {
			t.Fatalf("missing R1a in %v", got)
		}
		if _, ok := set["R1b"]; !ok {
			t.Fatalf("missing R1b in %v", got)
		}
	})
}

func TestPatternMatching(t *testing.T) {
	t.Parallel()

	t.Run("wildcard matching", func(t *testing.T) {
		t.Parallel()

		t.Run("zero chars", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("ab*cd", "ab*cd")

			got := tr.Get("abcd")
			want := []string{"ab*cd"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("multiple chars", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("a*c", "a*c")

			got := tr.Get("abbbbbc")
			want := []string{"a*c"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("version wildcard", func(t *testing.T) {
			t.Parallel()

			t.Run("v1", func(t *testing.T) {
				t.Parallel()

				tr := New[string]()
				tr.Insert("example.com/api/v*", "example.com/api/v*")

				got := tr.Get("https://example.com/api/v1")
				want := []string{"example.com/api/v*"}
				if !equalSets(got, want) {
					t.Fatalf("got=%v, want=%v", got, want)
				}
			})

			t.Run("multiple rules", func(t *testing.T) {
				t.Parallel()

				tr := New[string]()
				tr.Insert("example.com/api/v*", "example.com/api/v*")

				tr.Insert("example.com/api/v*/endpoint", "example.com/api/v*/endpoint")

				got := tr.Get("https://example.com/api/v2/endpoint")
				want := []string{"example.com/api/v*", "example.com/api/v*/endpoint"}
				if !equalSets(got, want) {
					t.Fatalf("got=%v, want=%v", got, want)
				}
			})

			t.Run("incomplete match", func(t *testing.T) {
				t.Parallel()

				tr := New[string]()
				tr.Insert("example.com/api/v*/endpoint", "example.com/api/v*/endpoint")

				got := tr.Get("https://example.com/api/v2/test")
				want := []string{}
				if !equalSets(got, want) {
					t.Fatalf("got=%v, want=%v", got, want)
				}
			})
		})

		t.Run("branching", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("e*", "e*")
			tr.Insert("e*|", "e*|")
			tr.Insert("e*^a", "e*^a")

			got := tr.Get("https://example.com")
			want := []string{"e*", "e*|"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})
	})

	t.Run("separator matching", func(t *testing.T) {
		t.Parallel()

		t.Run("query parameter", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("ads^", "ads^")

			got := tr.Get("http://example.com/ads?x=1")
			want := []string{"ads^"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("multiple subsequent separators", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("ads^", "ads^")

			got := tr.Get("http://example.com/ads/?x=1")
			want := []string{"ads^"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("end of address", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("ads^", "ads^")

			got := tr.Get("http://example.com/ads")
			want := []string{"ads^"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("letters", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("ads^", "ads^")

			got := tr.Get("http://example.com/adsx")
			want := []string{}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})
	})

	t.Run("anchor matching", func(t *testing.T) {
		t.Parallel()

		t.Run("beginning of address", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("|http://example.org", "|http://example.org")

			got := tr.Get("http://example.org/page")
			want := []string{"|http://example.org"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("middle of address", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("|http://example.org", "|http://example.org")

			got := tr.Get("http://domain.com/?url=http://example.org")
			want := []string{}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("exact suffix", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert(".com/b.js|", ".com/b.js|")

			got := tr.Get("http://example.com/b.js")
			want := []string{".com/b.js|"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("trailing chars", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("/ads/targeted|", "/ads/targeted|")

			got := tr.Get("http://example.com/ads/targeted/extra")
			want := []string{}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})
	})

	t.Run("domain boundary matching", func(t *testing.T) {
		t.Parallel()

		t.Run("main domain", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com/ads", "||example.com/ads")

			got := tr.Get("http://example.com/ads")
			want := []string{"||example.com/ads"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("lookalike domain", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com/ads", "||example.com/ads")

			got := tr.Get("http://notexample.com/ads")
			want := []string{}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("subdomain", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com/ads", "||example.com/ads")

			got := tr.Get("https://sub.example.com/ads")
			want := []string{"||example.com/ads"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("lookalike subdomain", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com/ads", "||example.com/ads")

			got := tr.Get("https://sub.bexample.com/ads")
			want := []string{}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("wss protocol", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com/ads", "||example.com/ads")

			got := tr.Get("wss://example.com/ads")
			want := []string{"||example.com/ads"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})
	})

	t.Run("domain boundary with separator", func(t *testing.T) {
		t.Parallel()

		t.Run("plain host", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com^", "||example.com^")

			got := tr.Get("https://sub.example.com")
			want := []string{"||example.com^"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("host with path", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com^", "||example.com^")

			got := tr.Get("https://sub.example.com/path")
			want := []string{"||example.com^"}

			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("different domain", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("||example.com^", "||example.com^")

			got := tr.Get("https://badexample.com/")
			want := []string{}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})
	})

	t.Run("generic matching", func(t *testing.T) {
		t.Parallel()

		t.Run("singular", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("", "")

			got := tr.Get("https://example.com")
			want := []string{""}

			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("multiple", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			tr.Insert("", "")
			tr.Insert("|https://example.com", "|https://example.com")
			tr.Insert("||example.com", "||example.com")

			got := tr.Get("https://example.com")
			want := []string{"", "|https://example.com", "||example.com"}
			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})
	})

	t.Run("complex rule intersections", func(t *testing.T) {
		t.Parallel()

		t.Run("set 1", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			rules := []string{
				"||example.com/*",
				"||example.com/ads/*",
				"|http://example.com/ads/top",
				"|https://example.com/ads/bottom",
				"/ads/*",
				"*/top*",
				"ads^",
				"swf|",
				"|https://sub.example.com/strict|",
			}

			for _, rule := range rules {
				tr.Insert(rule, rule)
			}

			got := tr.Get("http://sub.example.com/ads/top?x=1")
			want := []string{
				"||example.com/*",
				"||example.com/ads/*",
				"*/top*",
				"/ads/*",
				"ads^",
			}

			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})

		t.Run("set 2", func(t *testing.T) {
			t.Parallel()

			tr := New[string]()
			rules := []string{
				"||example.com^",
				"||example.com/ads/*",
				"||example.net^",
				"|https://example.com/login",
				"|https://sub.example.com/strict|",
				"str",
				".com*ct",
				".com*co",
				".com*tt",
			}

			for _, rule := range rules {
				tr.Insert(rule, rule)
			}

			got := tr.Get("https://sub.example.com/strict")
			want := []string{
				"||example.com^",
				"|https://sub.example.com/strict|",
				"str",
				".com*ct",
			}

			if !equalSets(got, want) {
				t.Fatalf("got=%v, want=%v", got, want)
			}
		})
	})

	t.Run("testdata", func(t *testing.T) {
		t.Parallel()

		t.Run("adsdelivery", func(t *testing.T) {
			t.Parallel()

			tr := buildTestTree(t)

			got := tr.Get("https://example.com/adsdelivery/test")
			want := []string{"/adsdelivery/*"} // easylist.txt#L218
			if !equalSets(got, want) {
				t.Errorf("got=%v, want=%v", got, want)
			}
		})

		t.Run("geodator", func(t *testing.T) {
			t.Parallel()

			tr := buildTestTree(t)

			got := tr.Get("https://geodator.com/geo.php?ip=localhost")
			want := []string{
				"/geo.php?",       // easyprivacy.txt#L1044
				"||geodator.com^", // easylist.txt#L14474
			}
			if !equalSets(got, want) {
				t.Errorf("got=%v, want=%v", got, want)
			}
		})

		t.Run("doubleclick", func(t *testing.T) {
			t.Parallel()

			tr := buildTestTree(t)

			got := tr.Get("https://g.doubleclick.com/statscounter/script.js?id=GTM-123&pagegroup=test&url=zenprivacy.net")
			want := []string{
				"||doubleclick.com^", // easylist.txt#L39723
				"/statscounter/*",    // easyprivacy.txt#L2235
				"&pagegroup=*&url=",  // easyprivacy.txt#L2937
				".js?id=GTM-",        // easyprivacy.txt#L2950
			}
			if !equalSets(got, want) {
				t.Errorf("got=%v, want=%v", got, want)
			}
		})

		t.Run("gtm", func(t *testing.T) {
			t.Parallel()

			tr := buildTestTree(t)

			got := tr.Get("http://gtm.example.net/t/id.js?st=321")
			want := []string{"://gtm.*.js?st="} // easyprivacy.txt#L2957
			if !equalSets(got, want) {
				t.Errorf("got=%v, want=%v", got, want)
			}
		})
	})
}

func buildTestTree(t *testing.T) *Tree[string] {
	t.Helper()

	filterLists := []string{"testdata/easylist.txt", "testdata/easyprivacy.txt"}

	tr := New[string]()

	for _, list := range filterLists {
		f, err := os.Open(list)
		if err != nil {
			t.Fatalf("open %q: %v", list, err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			tr.Insert(line, line)
		}

		if err := scanner.Err(); err != nil {
			t.Fatalf("scan: %v", err)
		}
	}

	return tr
}

func asSet(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

func equalSets(a, b []string) bool {
	am := asSet(a)
	bm := asSet(b)
	if len(am) != len(bm) {
		return false
	}
	for k := range am {
		if _, ok := bm[k]; !ok {
			return false
		}
	}
	return true
}
