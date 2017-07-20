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

func log(x int32) int64 {
	return int64(math.Pow(math.Log1p(math.Abs(float64(x))), 3))
}

const (
	timeSlope int64 = 64
)

// Log is the logarithmic sort
func Log(ups, downs int32, creationTime int64) int64 {
	x := ups - downs
	y := creationTime - time.Now().Unix()
	return sign(x)*log(x)*timeSlope + y
}
