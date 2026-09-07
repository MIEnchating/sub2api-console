package routing

import (
	"math"
	"math/big"
)

func relativePriceScore(rate, best *big.Rat, exponent float64) float64 {
	if rate.Sign() == 0 {
		return 1
	}
	if best == nil || best.Sign() == 0 {
		return 0
	}
	// Normalize exact decimal costs before exponentiation. Keeping the ratio's
	// binary exponent separate avoids both overflow and premature underflow,
	// including very small costs paired with a weak price exponent.
	ratio := new(big.Rat).Quo(best, rate)
	value := new(big.Float).SetPrec(64).SetRat(ratio)
	mantissa := new(big.Float)
	scale := value.MantExp(mantissa)
	fraction, _ := mantissa.Float64()
	logRatio := math.Log(fraction) + float64(scale)*math.Ln2
	return math.Exp(min(0, logRatio) * exponent)
}
