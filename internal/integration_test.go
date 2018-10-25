package internal

import (
	"crypto/rand"
	"fmt"
	"github.com/spacemeshos/poet-ref/shared"
	"github.com/stretchr/testify/assert"
	"testing"
)

/*
	Latest results:
	n= 25 total time = 2356.15s (29 minutes)


*/
func TestBigNip(t *testing.T) {

	x := make([]byte, 32)
	_, err := rand.Read(x)
	assert.NoError(t, err)

	// with n=25 and 16GB ram:
	// Map size:  67108863 entries ~20GB in ram w map - runtime: 1034.77s
	const n = 16

	p, err := NewProver(x, n)
	assert.NoError(t, err)

	p.ComputeDag(func(phi shared.Label, err error) {
		fmt.Printf("Dag root label: %s\n", GetDisplayValue(phi))
		assert.NoError(t, err)

		proof, err := p.GetNonInteractiveProof()
		assert.NoError(t, err)

		v, err := NewVerifier(x, n)
		assert.NoError(t, err)

		c, err := v.CreteNipChallenge(proof.Phi)
		assert.NoError(t, err)

		res := v.Verify(c, proof)
		assert.True(t, res, "failed to verify proof")

		// p.DeleteStore()
	})
}

func TestNip(t *testing.T) {

	var x = []byte("Spacemesh launched its mainent")
	const n = 11 // 33.6mb storage

	p, err := NewProver(x, n)
	assert.NoError(t, err)

	p.ComputeDag(func(phi shared.Label, err error) {
		fmt.Printf("Dag root label: %s\n", GetDisplayValue(phi))
		assert.NoError(t, err)

		proof, err := p.GetNonInteractiveProof()
		assert.NoError(t, err)

		v, err := NewVerifier(x, n)
		assert.NoError(t, err)

		c, err := v.CreteNipChallenge(proof.Phi)
		assert.NoError(t, err)

		res := v.Verify(c, proof)
		assert.True(t, res, "failed to verify proof")

		p.DeleteStore()
	})
}

func TestRndChallengeProof(t *testing.T) {

	x := make([]byte, 32)
	_, err := rand.Read(x)
	assert.NoError(t, err)

	const n = 9

	p, err := NewProver(x, n)
	assert.NoError(t, err)

	p.ComputeDag(func(phi shared.Label, err error) {

		//fmt.Printf("Dag root label: %s\n", GetDisplayValue(phi))
		assert.NoError(t, err)

		v, err := NewVerifier(x, n)
		assert.NoError(t, err)

		c, err := v.CreteRndChallenge()
		assert.NoError(t, err)

		// println("Challenge data:")
		// c.Print()

		proof, err := p.GetProof(c)
		assert.NoError(t, err)
		// PrintProof(proof)

		res := v.Verify(c, proof)
		assert.True(t, res, "failed to verify proof")

		p.DeleteStore()
	})
}

func TestRndChallengeProofEx(t *testing.T) {

	// generate random commitment
	x := make([]byte, 32)
	_, err := rand.Read(x)
	assert.NoError(t, err)

	const n = 9

	p, err := NewProver(x, n)

	defer p.DeleteStore()

	assert.NoError(t, err)

	p.ComputeDag(func(phi shared.Label, err error) {

		//fmt.Printf("Dag root label: %s\n", GetDisplayValue(phi))
		assert.NoError(t, err)

		v, err := NewVerifier(x, n)
		assert.NoError(t, err)

		for i := 0; i < 1000; i++ {

			c, err := v.CreteRndChallenge()
			assert.NoError(t, err)

			proof, err := p.GetProof(c)
			assert.NoError(t, err)

			res := v.Verify(c, proof)

			if !res {
				println("Failed to verify proof. Challenge data:")
				c.Print()
				println("Proof:")
				PrintProof(proof)
			}

			assert.True(t, res, "failed to verify proof")
		}

	})
}
