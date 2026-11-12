package points

import (
	"encoding/json"
	"strconv"
)

// Integer is a dimensionless integer such as a bet multiplier.
type Integer int64

func (v Integer) MarshalJSON() ([]byte, error) { return json.Marshal(strconv.FormatInt(int64(v), 10)) }
func (v *Integer) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return ErrInvalid
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || strconv.FormatInt(n, 10) != text {
		return ErrInvalid
	}
	*v = Integer(n)
	return nil
}
