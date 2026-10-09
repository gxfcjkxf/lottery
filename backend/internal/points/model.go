// Package points defines the exact integer model for the sixteen point buckets.
package points

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

var (
	ErrInvalid      = errors.New("invalid points value")
	ErrInsufficient = errors.New("insufficient points")
	ErrOverflow     = errors.New("points integer overflow")
)

// Amount is an exact signed integer point amount. JSON represents it as a
// decimal string so JavaScript clients never round large values.
type Amount int64

func ParseAmount(value string) (Amount, error) {
	if value == "" || value == "-0" || strings.HasPrefix(value, "+") {
		return 0, ErrInvalid
	}
	start := 0
	if value[0] == '-' {
		start = 1
		if len(value) == 1 {
			return 0, ErrInvalid
		}
	}
	if value[start] == '0' && len(value)-start != 1 {
		return 0, ErrInvalid
	}
	for i := start; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, ErrOverflow
		}
		return 0, ErrInvalid
	}
	return Amount(n), nil
}

func (a Amount) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(a), 10))
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	if a == nil {
		return ErrInvalid
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
		return ErrInvalid
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return fmt.Errorf("%w: amount must be a decimal string", ErrInvalid)
	}
	parsed, err := ParseAmount(value)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

var sourceNames = [...]string{"recharge", "winning", "gift", "commission"}
var debitSourceNames = [...]string{"recharge", "winning", "commission", "gift"}
var stateNames = [...]string{"available", "manual_frozen", "system_frozen", "withdrawal"}

// Balance stores [source][state], with the fixed source and state orders
// declared by sourceNames and stateNames.
type Balance [4][4]Amount

type Allocation struct {
	Source string `json:"source"`
	State  string `json:"state"`
	Points Amount `json:"points"`
}

func SourceIndex(source string) (int, error) {
	for i, name := range sourceNames {
		if source == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: unknown source", ErrInvalid)
}

func StateIndex(state string) (int, error) {
	for i, name := range stateNames {
		if state == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: unknown state", ErrInvalid)
}

func (b Balance) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for source, sourceName := range sourceNames {
		if source > 0 {
			out.WriteByte(',')
		}
		name, _ := json.Marshal(sourceName)
		out.Write(name)
		out.WriteString(":{")
		for state, stateName := range stateNames {
			if state > 0 {
				out.WriteByte(',')
			}
			stateJSON, _ := json.Marshal(stateName)
			out.Write(stateJSON)
			out.WriteByte(':')
			amountJSON, _ := b[source][state].MarshalJSON()
			out.Write(amountJSON)
		}
		out.WriteByte('}')
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func (b *Balance) UnmarshalJSON(data []byte) error {
	if b == nil {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("%w: balance must be an object", ErrInvalid)
	}
	var parsed Balance
	var sourceSeen [4]bool
	sourceCount := 0
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: malformed source", ErrInvalid)
		}
		sourceName, ok := token.(string)
		if !ok {
			return fmt.Errorf("%w: malformed source name", ErrInvalid)
		}
		source, err := SourceIndex(sourceName)
		if err != nil {
			return err
		}
		if sourceSeen[source] {
			return fmt.Errorf("%w: duplicate source", ErrInvalid)
		}
		sourceSeen[source] = true
		sourceCount++
		var bucket json.RawMessage
		if err = decoder.Decode(&bucket); err != nil {
			return fmt.Errorf("%w: malformed bucket", ErrInvalid)
		}
		if err = decodeBucket(bucket, &parsed[source]); err != nil {
			return err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return fmt.Errorf("%w: malformed balance object", ErrInvalid)
	}
	if sourceCount != 4 {
		return fmt.Errorf("%w: incomplete balance sources", ErrInvalid)
	}
	for source := range sourceSeen {
		if !sourceSeen[source] {
			return fmt.Errorf("%w: missing source", ErrInvalid)
		}
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON value", ErrInvalid)
	}
	*b = parsed
	return nil
}

func decodeBucket(data []byte, bucket *[4]Amount) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("%w: bucket must be an object", ErrInvalid)
	}
	var seen [4]bool
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: malformed state", ErrInvalid)
		}
		stateName, ok := token.(string)
		if !ok {
			return fmt.Errorf("%w: malformed state name", ErrInvalid)
		}
		state, err := StateIndex(stateName)
		if err != nil {
			return err
		}
		if seen[state] {
			return fmt.Errorf("%w: duplicate state", ErrInvalid)
		}
		seen[state] = true
		var amount Amount
		if err = decoder.Decode(&amount); err != nil {
			return err
		}
		bucket[state] = amount
	}
	if _, err = decoder.Token(); err != nil {
		return fmt.Errorf("%w: malformed bucket object", ErrInvalid)
	}
	for _, present := range seen {
		if !present {
			return fmt.Errorf("%w: missing state", ErrInvalid)
		}
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing bucket JSON", ErrInvalid)
	}
	return nil
}

func checkedAdd(a, b Amount) (Amount, error) {
	if b > 0 && int64(a) > math.MaxInt64-int64(b) {
		return 0, ErrOverflow
	}
	if b < 0 && int64(a) < math.MinInt64-int64(b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func (b Balance) Validate() error {
	var total Amount
	for _, source := range b {
		for _, amount := range source {
			if amount < 0 {
				return fmt.Errorf("%w: negative balance bucket", ErrInvalid)
			}
			var err error
			total, err = checkedAdd(total, amount)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (b Balance) Apply(delta Balance) (Balance, error) {
	var after Balance
	for source := range b {
		for state := range b[source] {
			value, err := checkedAdd(b[source][state], delta[source][state])
			if err != nil {
				return Balance{}, err
			}
			if value < 0 {
				return Balance{}, fmt.Errorf("%w: balance bucket underflow", ErrInsufficient)
			}
			after[source][state] = value
		}
	}
	if err := after.Validate(); err != nil {
		return Balance{}, err
	}
	return after, nil
}

func (b Balance) StateTotal(state int) (Amount, error) {
	if state < 0 || state >= len(stateNames) {
		return 0, ErrInvalid
	}
	var total Amount
	for source := range b {
		var err error
		total, err = checkedAdd(total, b[source][state])
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (b Balance) SourceTotal(source int) (Amount, error) {
	if source < 0 || source >= len(sourceNames) {
		return 0, ErrInvalid
	}
	var total Amount
	for state := range b[source] {
		var err error
		total, err = checkedAdd(total, b[source][state])
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (b Balance) Allocate(points Amount, stateName string) ([]Allocation, error) {
	if points <= 0 {
		return nil, ErrInvalid
	}
	state, err := StateIndex(stateName)
	if err != nil {
		return nil, err
	}
	if err = b.Validate(); err != nil {
		return nil, err
	}
	remaining := points
	allocations := make([]Allocation, 0, len(sourceNames))
	for _, sourceName := range debitSourceNames {
		source, err := SourceIndex(sourceName)
		if err != nil {
			return nil, err
		}
		if remaining == 0 {
			break
		}
		available := b[source][state]
		if available == 0 {
			continue
		}
		used := available
		if used > remaining {
			used = remaining
		}
		allocations = append(allocations, Allocation{Source: sourceName, State: stateName, Points: used})
		remaining -= used
	}
	if remaining != 0 {
		return nil, ErrInsufficient
	}
	return allocations, nil
}

func AllocationDelta(allocations []Allocation, fromState, toState string) (Balance, error) {
	var delta Balance
	if fromState == "" && toState == "" || len(allocations) == 0 {
		return delta, ErrInvalid
	}
	from, to := -1, -1
	var err error
	if fromState != "" {
		from, err = StateIndex(fromState)
		if err != nil {
			return delta, err
		}
	}
	if toState != "" {
		to, err = StateIndex(toState)
		if err != nil {
			return delta, err
		}
	}
	var seen [4][4]bool
	var total Amount
	for _, allocation := range allocations {
		source, err := SourceIndex(allocation.Source)
		if err != nil {
			return Balance{}, err
		}
		state, err := StateIndex(allocation.State)
		if err != nil {
			return Balance{}, err
		}
		if from >= 0 {
			if state != from {
				return Balance{}, ErrInvalid
			}
		} else if state != to {
			return Balance{}, ErrInvalid
		}
		if allocation.Points <= 0 {
			return Balance{}, ErrInvalid
		}
		if seen[source][state] {
			return Balance{}, ErrInvalid
		}
		seen[source][state] = true
		total, err = checkedAdd(total, allocation.Points)
		if err != nil {
			return Balance{}, err
		}
		if from >= 0 {
			delta[source][from], err = checkedAdd(delta[source][from], -allocation.Points)
			if err != nil {
				return Balance{}, err
			}
		}
		if to >= 0 {
			delta[source][to], err = checkedAdd(delta[source][to], allocation.Points)
			if err != nil {
				return Balance{}, err
			}
		}
	}
	return delta, nil
}

func Negate(balance Balance) (Balance, error) {
	var negated Balance
	for source := range balance {
		for state, amount := range balance[source] {
			if amount == Amount(math.MinInt64) {
				return Balance{}, ErrOverflow
			}
			negated[source][state] = -amount
		}
	}
	return negated, nil
}
