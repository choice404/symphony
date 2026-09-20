package nvim

import "fmt"

/**
 * asInt
 * Reads any integer or float the msgpack decoder produces as an int
 * @param v {interface{}} - the decoded value
 * @return int, bool
 **/
func asInt(v interface{}) (int, bool) {
	// Switch over every numeric type the decoder can hand back
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	}
	// Anything else is not a number
	return 0, false
}

/**
 * asString
 * Reads a decoded value as a string
 * @param v {interface{}} - the decoded value
 * @return string, bool
 **/
func asString(v interface{}) (string, bool) {
	// Switch over the two shapes a string arrives as
	switch s := v.(type) {
	case string:
		return s, true
	case []byte:
		return string(s), true
	}
	// Anything else is not a string
	return "", false
}

/**
 * asSlice
 * Reads a decoded value as a slice
 * @param v {interface{}} - the decoded value
 * @return []interface{}, bool
 **/
func asSlice(v interface{}) ([]interface{}, bool) {
	// Cast to the slice shape the decoder uses
	s, ok := v.([]interface{})
	// Return the slice and whether the cast held
	return s, ok
}

/**
 * asMap
 * Reads a decoded value as a string keyed map, converting an interface keyed map when needed
 * @param v {interface{}} - the decoded value
 * @return map[string]interface{}, bool
 **/
func asMap(v interface{}) (map[string]interface{}, bool) {
	// Switch over the two map shapes the decoder can produce
	switch m := v.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		// The converted map
		out := make(map[string]interface{}, len(m))
		// Loop over every entry
		for k, val := range m {
			// Keep the entry when the key is a string
			if ks, ok := asString(k); ok {
				out[ks] = val
			}
		}
		// Return the converted map
		return out, true
	}
	// Anything else is not a map
	return nil, false
}

/**
 * intAt
 * Reads the int at an index of an args slice with a named error on failure
 * @param args {[]interface{}} - the event args
 * @param i {int} - the index
 * @param event {string} - the event name for the error
 * @return int, error
 **/
func intAt(args []interface{}, i int, event string) (int, error) {
	// Fail when the index is past the end
	if i >= len(args) {
		return 0, fmt.Errorf("%s: missing arg %d", event, i)
	}
	// Read the int
	n, ok := asInt(args[i])
	// Fail when it is not a number
	if !ok {
		return 0, fmt.Errorf("%s: arg %d is %T not int", event, i, args[i])
	}
	// Return the int
	return n, nil
}

/**
 * stringAt
 * Reads the string at an index of an args slice with a named error on failure
 * @param args {[]interface{}} - the event args
 * @param i {int} - the index
 * @param event {string} - the event name for the error
 * @return string, error
 **/
func stringAt(args []interface{}, i int, event string) (string, error) {
	// Fail when the index is past the end
	if i >= len(args) {
		return "", fmt.Errorf("%s: missing arg %d", event, i)
	}
	// Read the string
	s, ok := asString(args[i])
	// Fail when it is not a string
	if !ok {
		return "", fmt.Errorf("%s: arg %d is %T not string", event, i, args[i])
	}
	// Return the string
	return s, nil
}
