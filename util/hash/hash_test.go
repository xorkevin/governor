package hash

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_KDF(t *testing.T) {
	assert := assert.New(t)
	key := "password"

	// success case
	h, err := KDF(key)
	assert.Nil(err, "hash should be successful")
	assert.True(VerifyKDF(key, h), "key should be correct")

	// invalid key
	assert.False(VerifyKDF("notpass", h), "incorrect key should fail")

	// invalid hash
	assert.False(VerifyKDF(key, ""), "blank hash should fail")
	assert.False(VerifyKDF(key, "$s0"), "invalid number of hash components should fail")

	// invalid parameters
	assert.False(VerifyKDF(key, "$s0$$$"), "invalid number of parameters should fail")
	assert.False(VerifyKDF(key, "$s0$,,$$"), "invalid parameters should fail")
	assert.False(VerifyKDF(key, "$s0$0,,$$"), "invalid parameters should fail")
	assert.False(VerifyKDF(key, "$s0$0,0,$$"), "invalid parameters should fail")
	assert.False(VerifyKDF(key, "$s0$0,0,0$$"), "invalid parameters should fail")

	// invalid salt
	assert.False(VerifyKDF(key, "$s0$0,0,0$bogus+salt+value$"), "invalid salt should fail")

	// invalid hash
	assert.False(VerifyKDF(key, "$s0$0,0,0$bogussaltvalue$bogus+hash+value"), "invalid hash should fail")

	// invalid param values
	assert.False(VerifyKDF(key, "$s0$0,0,0$bogussaltvalue$bogushashvalue"), "invalid parameter values should fail")
}

func Benchmark_VerfyKDF(b *testing.B) {
	key := "password"
	h, _ := KDF(key)
	for n := 0; n < b.N; n++ {
		VerifyKDF(key, h)
	}
}
