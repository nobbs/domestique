package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNonNegativeSpeedsRejectsANonFiniteSpeed(t *testing.T) {
	t.Parallel()
	for _, list := range []string{"NaN", "+Inf", "0,-Inf"} {
		_, err := parseNonNegativeSpeeds(list)
		require.Error(t, err, "parseNonNegativeSpeeds(%q)", list)
	}
}
