package numbers

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ExtractFloat converts common scalar types into float64.
func ExtractFloat(val any) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		if v == "" {
			return 0, fmt.Errorf("empty string")
		}
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("unsupported float type %T", val)
	}
}

// ExtractInt converts common scalar types into int64.
func ExtractInt(val any) (int64, error) {
	switch v := val.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	case string:
		if v == "" {
			return 0, fmt.Errorf("empty string")
		}
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported int type %T", val)
	}
}
