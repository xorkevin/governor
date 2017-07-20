package score

import (
	"math"
	"time"
)

func sign(x int32) int64 {
	if x < 0 {
		return -1
	}
	return 1
}

func log(x int32) float64 {
	return math.Pow(math.Log1p(math.Abs(float64(x))), 3)
}

const (
	timeSlope float64 = 64
)

// Log is the logarithmic sort
func Log(ups, downs int32, creationTime int64) int64 {
	x := ups - downs
	y := creationTime - time.Now().Unix()
	return sign(x)*int64(log(x)*timeSlope) + y
}
