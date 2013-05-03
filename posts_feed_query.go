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

func (q *PostsFeedQuery) SincePost(entity, id string) *PostsFeedQuery {
	q.Set("since_post", entity+" "+id)
	return q
}

func (q *PostsFeedQuery) BeforePost(entity, id string) *PostsFeedQuery {
	q.Set("before_post", entity+" "+id)
	return q
}

func (q *PostsFeedQuery) UntilPost(entity, id string) *PostsFeedQuery {
	q.Set("until_post", entity+" "+id)
	return q
}

func (q *PostsFeedQuery) SinceTime(t time.Time) *PostsFeedQuery {
	q.Set("since_time", timeMillis(t))
	return q
}

func (q *PostsFeedQuery) BeforeTime(t time.Time) *PostsFeedQuery {
	q.Set("before_time", timeMillis(t))
	return q
}

func (q *PostsFeedQuery) UntilTime(t time.Time) *PostsFeedQuery {
	q.Set("until_time", timeMillis(t))
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

func timeMillis(t time.Time) string {
	return strconv.FormatInt(t.UnixNano()/int64(time.Millisecond), 10)
}
