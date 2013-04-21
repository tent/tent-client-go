package tent

import (
	"strings"
	"testing"

	. "launchpad.net/gocheck"
)

// Hook gocheck into the gotest runner.
func Test(t *testing.T) { TestingT(t) }

type DiscoverySuite struct{}

var _ = Suite(&DiscoverySuite{})

var metaHTML = strings.NewReader(`
<html>
<head>
<title>foo</title>
<link rel="icon" href="http://foo.bar">
<link href="http://foo.bar">
<link>
<link rel="https://tent.io/rels/meta-post">
<link rel="https://tent.io/rels/meta-post" href="a">
<link rel="https://tent.io/rels/meta-post" href="b">
<link rel="https://tent.io/rels/meta-post" href="c"/>
<link rel="https://tent.io/rels/meta-post" href="d"></link>
</head>
</html>
`)

func (s *DiscoverySuite) TestHTMLLinkParsing(c *C) {
	res, err := parseHTMLMetaLinks(metaHTML)
	c.Assert(err, IsNil)
	c.Assert(res, DeepEquals, []string{"a", "b", "c", "d"})
}
