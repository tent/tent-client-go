package tent

import (
	"errors"
	"strconv"
	"time"
)

var UnixTimeUnmarshalError = errors.New("tent: invalid unix timestamp")

type UnixTime struct{ time.Time }

func (t UnixTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("0"), nil
	}
	return strconv.AppendInt(nil, t.UnixNano()/int64(time.Millisecond), 10), nil
}

func (t *UnixTime) UnmarshalJSON(data []byte) error {
	i, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return UnixTimeUnmarshalError
	}
	t.Time = time.Unix(0, i*int64(time.Millisecond))
	return nil
}
