package tent

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PostsFeedQuery struct{ url.Values }

func NewPostsFeedQuery() *PostsFeedQuery { return &PostsFeedQuery{make(url.Values)} }

func (q *PostsFeedQuery) Limit(n int) *PostsFeedQuery {
	q.Set("limit", strconv.Itoa(n))
	return q
}

func (q *PostsFeedQuery) Since(t time.Time, version string) *PostsFeedQuery {
	q.Set("since", paginationRef(t, version))
	return q
}

func (q *PostsFeedQuery) Before(t time.Time, version string) *PostsFeedQuery {
	q.Set("before", paginationRef(t, version))
	return q
}

func (q *PostsFeedQuery) Until(t time.Time, version string) *PostsFeedQuery {
	q.Set("until", paginationRef(t, version))
	return q
}

func (q *PostsFeedQuery) Entities(entities ...string) *PostsFeedQuery {
	q.Set("entities", strings.Join(entities, ","))
	return q
}

func (q *PostsFeedQuery) Types(types ...string) *PostsFeedQuery {
	q.Set("types", strings.Join(types, ","))
	return q
}

func (q *PostsFeedQuery) MaxRefs(n int) *PostsFeedQuery {
	q.Set("max_refs", strconv.Itoa(n))
	return q
}

func (q *PostsFeedQuery) Mentions(mentions ...[]string) *PostsFeedQuery {
	buf := &bytes.Buffer{}
	for i, m := range mentions {
		buf.WriteString(m[0])
		if len(m) > 1 {
			buf.WriteByte(' ')
			buf.WriteString(m[1])
		}
		if i < len(mentions)-1 {
			buf.WriteByte(',')
		}
	}
	q.Add("mentions", buf.String())
	return q
}

func paginationRef(t time.Time, version string) string {
	ref := strconv.FormatInt(t.UnixNano()/int64(time.Millisecond), 10)
	if version != "" {
		ref += " " + version
	}
	return ref
}
