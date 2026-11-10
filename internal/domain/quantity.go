package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Quantity is an exact, nonnegative count in thousandths of a unit.
type Quantity int64

func ParseQuantity(input string) (Quantity, error) {
	whole, fraction, decimal := strings.Cut(input, ".")
	if whole == "" || len(fraction) > 3 || (decimal && fraction == "") {
		return 0, Invalid("quantity must have up to three decimal places")
	}
	for _, segment := range []string{whole, fraction} {
		for _, c := range segment {
			if c < '0' || c > '9' {
				return 0, Invalid("quantity must be a nonnegative decimal")
			}
		}
	}
	thousandths, err := strconv.ParseInt(whole+fraction+strings.Repeat("0", 3-len(fraction)), 10, 64)
	if err != nil {
		return 0, Invalid("quantity is too large")
	}
	return Quantity(thousandths), nil
}

func (q Quantity) String() string {
	return fmt.Sprintf("%d.%03d", q/1000, q%1000)
}

func (q Quantity) MarshalJSON() ([]byte, error) { return json.Marshal(q.String()) }

func (q *Quantity) UnmarshalJSON(data []byte) error {
	var input string
	if err := json.Unmarshal(data, &input); err != nil || input == "" {
		return Invalid("quantity must be a decimal string")
	}
	parsed, err := ParseQuantity(input)
	if err != nil {
		return err
	}
	*q = parsed
	return nil
}
