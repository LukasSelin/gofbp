package cffdrstest

import (
	"encoding/json"
	"math"
)

// GridCase is one cell of the fixture's fbp_grid_cases: the grid it belongs
// to, its place in terra's cell order, and every other column by name. A null
// is NA — the grid block emits every column on every row, so null never means
// "not emitted" — and reads back as NaN.
type GridCase struct {
	Grid string
	Cell int
	v    map[string]*float64
}

func (c *GridCase) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	c.v = map[string]*float64{}
	for k, x := range m {
		switch k {
		case "grid":
			c.Grid, _ = x.(string)
		case "cell":
			f, _ := x.(float64)
			c.Cell = int(f)
		default:
			if f, ok := x.(float64); ok {
				c.v[k] = &f
			} else {
				c.v[k] = nil
			}
		}
	}
	return nil
}

// Col is column k, NaN for NA. A column the row does not carry is NaN too, and
// TestCFFDRSPredictGrid checks every column it reads is present first.
func (c GridCase) Col(k string) float64 {
	if p := c.v[k]; p != nil {
		return *p
	}
	return math.NaN()
}

// Has reports whether the row carries column k at all, NA or not.
func (c GridCase) Has(k string) bool {
	_, ok := c.v[k]
	return ok
}
