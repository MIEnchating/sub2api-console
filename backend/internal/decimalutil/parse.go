package decimalutil

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE]([+-]?[0-9]+))?$`)

// Parse bounds external decimal input before math/big can allocate powers of
// ten. Rat's fraction, hexadecimal and underscore syntaxes are not decimals.
func Parse(raw string) (*big.Rat, bool) {
	value := strings.TrimSpace(raw)
	if len(value) == 0 || len(value) > 128 {
		return nil, false
	}
	match := decimalPattern.FindStringSubmatch(value)
	if match == nil {
		return nil, false
	}
	if match[1] != "" {
		exponent, err := strconv.Atoi(match[1])
		if err != nil || exponent < -1000 || exponent > 1000 {
			return nil, false
		}
	}
	return new(big.Rat).SetString(value)
}
