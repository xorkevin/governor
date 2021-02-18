package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidEmail(t *testing.T) {
	assert := assert.New(t)

	assert.Nil(validEmail("gov@xorkevin.com"), "email should be valid")
	assert.Nil(validEmail("gov@xor-kevin.com"), "hostname may contain dashes")
	assert.Nil(validEmail("gov@tld"), "top level domain email should be valid")
	assert.Nil(validEmail("gov+te.st@tld"), "+ and . are valid characters")
	assert.Nil(validEmail("_gov+test-@tld"), "_ and - may begin and end local part")
	assert.NotNil(validEmail("+gov@tld"), "+ may not begin local part")
	assert.NotNil(validEmail("gov..test@tld"), "two . may not be adjacent in local part")
	assert.NotNil(validEmail(".gov@tld"), ". may not begin local part")
	assert.NotNil(validEmail("gov.@tld"), ". may not end local part")
	assert.NotNil(validEmail("gov@-tld"), "- may not begin hostname part")
	assert.NotNil(validEmail("gov@tld-"), "- may not end hostname part")
	assert.NotNil(validEmail("gov@xor..kevin.com"), "two . may not be adjacent in hostname part")
}
