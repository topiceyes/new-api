package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testIndexTemplate = `<!doctype html>
<html lang="en">
  <head>
    <link rel="icon" type="image/png" href="/logo.png" />
    <title>New API</title>
    <meta name="title" content="New API" />
    <meta name="description" content="Unified AI API gateway and admin dashboard." />
  </head>
  <body>New API should stay untouched here</body>
</html>
`

func withSystemName(t *testing.T, name string) {
	t.Helper()
	previous := SystemName
	SystemName = name
	t.Cleanup(func() { SystemName = previous })
}

func TestRenderIndexPageReplacesTitleAndMeta(t *testing.T) {
	withSystemName(t, "Acme Gateway")

	rendered := string(RenderIndexPage([]byte(testIndexTemplate)))
	assert.Contains(t, rendered, "<title>Acme Gateway</title>")
	assert.Contains(t, rendered, `<meta name="title" content="Acme Gateway" />`)
	assert.Contains(t, rendered, `<meta name="description" content="Acme Gateway" />`)
	// body 文本不属于模板标识位,不应被替换
	assert.Contains(t, rendered, "New API should stay untouched here")
}

func TestRenderIndexPageStripsInjectedAnalyticsComments(t *testing.T) {
	withSystemName(t, "Acme Gateway")

	page := testIndexTemplate + "<!--Umami QuantumNous-->\n<!--Google Analytics QuantumNous-->\n"
	rendered := string(RenderIndexPage([]byte(page)))
	assert.NotContains(t, rendered, "QuantumNous")
	assert.NotContains(t, rendered, "<!--Umami")
	assert.NotContains(t, rendered, "<!--Google Analytics")
}

func TestRenderIndexPageEscapesSystemName(t *testing.T) {
	withSystemName(t, `Acme <script>alert(1)</script> " &`)

	rendered := string(RenderIndexPage([]byte(testIndexTemplate)))
	assert.NotContains(t, rendered, "<script>alert(1)</script>")
	assert.Contains(t, rendered, "<title>Acme &lt;script&gt;")
}

func TestRenderIndexPageReflectsRuntimeRename(t *testing.T) {
	withSystemName(t, "First Name")
	first := string(RenderIndexPage([]byte(testIndexTemplate)))
	require.Contains(t, first, "<title>First Name</title>")

	SystemName = "Second Name"
	second := string(RenderIndexPage([]byte(testIndexTemplate)))
	assert.Contains(t, second, "<title>Second Name</title>")
}

func TestRenderIndexPageNoOpWithoutPattern(t *testing.T) {
	withSystemName(t, "Acme Gateway")

	page := []byte("<html><body>no title here</body></html>")
	assert.Equal(t, page, RenderIndexPage(page))
}
