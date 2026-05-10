package rulemodifiers

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"golang.org/x/net/html"
)

func TestScrambleJSModifier(t *testing.T) {
	t.Parallel()

	t.Run("replaces keys in inline scripts in HTML response", func(t *testing.T) {
		t.Parallel()

		m := &ScrambleJSModifier{}
		m.Parse("scramblejs=key1|key2")

		original := []byte(`<html>
<head>
<script>
	var key1 = "str";
	var v = "key2";
</script>
<body>
	key1, key2 here should be unmodified
	<script>
		console.log(key1, key2);
	</script>
</body>
</html>`)
		res := newHTTPResponse("text/html", original)

		m.ModifyRes(res)
		n, err := html.Parse(res.Body)
		if err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		var buf bytes.Buffer
		if err := html.Render(&buf, n); err != nil {
			t.Fatalf("failed to render HTML: %v", err)
		}

		if !bytes.Contains(buf.Bytes(), []byte("key1, key2 here should be unmodified")) {
			t.Error("text outside scripts was incorrectly modified")
		}

		var scriptContainsOriginalKeys bool
		var checkScriptContent func(*html.Node)
		checkScriptContent = func(node *html.Node) {
			if node.Type == html.ElementNode && node.Data == "script" {
				for c := node.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						if bytes.Contains([]byte(c.Data), []byte("key1")) ||
							bytes.Contains([]byte(c.Data), []byte("key2")) {
							scriptContainsOriginalKeys = true
							return
						}
					}
				}
			}

			for c := node.FirstChild; c != nil; c = c.NextSibling {
				checkScriptContent(c)
			}
		}

		checkScriptContent(n)
		if scriptContainsOriginalKeys {
			t.Error("script tags contain keys that should've been scrambled")
		}
	})

	t.Run("replaces keys in JS response", func(t *testing.T) {
		t.Parallel()

		m := &ScrambleJSModifier{}
		m.Parse("scramblejs=key1|key2")

		original := []byte(`var key1 = "str";
var v = "key2"`)
		res := newHTTPResponse("text/javascript", original)

		m.ModifyRes(res)
		modified, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("failed to read modified body: %v", err)
		}

		if bytes.Contains(modified, []byte("key1")) || bytes.Contains(modified, []byte("key2")) {
			t.Error("resulting body contains keys that should've been scrambled")
		}
	})
}

func newHTTPResponse(contentType string, body []byte) *http.Response {
	return &http.Response{
		Body:       io.NopCloser(bytes.NewReader(body)),
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{contentType},
		},
	}
}
