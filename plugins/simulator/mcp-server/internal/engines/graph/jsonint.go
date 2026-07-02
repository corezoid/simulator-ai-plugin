package graph

import (
	"encoding/json"
	"math"
)

// jsonInt unmarshals a JSON number that may be an integer OR fractional into an
// int, rounding to nearest. Some backend layer positions are stored as floats
// (e.g. -1543.58); a plain `int` struct field makes json.Unmarshal fail with
// "cannot unmarshal number ... into int", which previously broke pullGraphFile /
// pushGraphFile / getAllLayerPlacements / pruneLongEdges whenever a layer held a
// single fractional coordinate. Rounding is safe here — these are pixel
// positions, and the write side already uses integer coordinates.
type jsonInt int

func (n *jsonInt) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*n = jsonInt(math.Round(f))
	return nil
}
