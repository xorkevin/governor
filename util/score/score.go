package score

import (
	"math"
)

func sign(x int32) int64 {
	if x < 0 {
		return -1
	}
	return 1
}

func log(x int32) int64 {
	return int64(math.Pow(math.Log1p(math.Abs(float64(x))), 3))
}

const (
	timeSlope int64 = 64
)

// Log is the logarithmic sort
func Log(ups, downs int32, creationTime, epoch int64) int64 {
	x := ups - downs
	y := (creationTime - epoch) / timeSlope
	return sign(x)*log(x) + y
}

const (
	zscore       float64 = 1.64 // 90% confidence
	zSqr         float64 = zscore * zscore
	zSqrd2       float64 = zSqr / 2
	zSqrd4       float64 = zSqr / 4
	maxConfScore float64 = 8070450532247928832
)

// Confidence is the confidence sort
func Confidence(ups, downs int32) int64 {
	if ups == 0 {
		return -int64(downs)
	}

	u := float64(ups)
	d := float64(downs)
	n := u + d

	above := u + zSqrd2 - zscore*math.Sqrt(u*d/n+zSqrd4)
	under := n + zSqr
	return int64((above / under) * maxConfScore)
}
