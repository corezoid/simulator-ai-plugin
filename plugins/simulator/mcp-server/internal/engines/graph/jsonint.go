package graph

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
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

// flexInt is jsonInt that also takes a numeric string, and reads null, "" or a
// non-numeric string as 0. Chart configs carry the same key as a string or a
// number depending on the writer (dynamicSource.top is "20" from createChart
// and the UI's preset buttons, a number from its int input), and one odd key
// must not make a chart unreadable.
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			f = 0
		}
		*n = flexInt(math.Round(f))
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		*n = 0
		return nil
	}
	*n = flexInt(math.Round(f))
	return nil
}
