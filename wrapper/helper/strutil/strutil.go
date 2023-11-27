package strutil

import (
	"sort"
	"strings"
)

// ParseStringSlice parses a `sep`-separated list of strings into a
// []string with surrounding whitespace removed.
//
// The output will always be a valid slice but may be of length zero.
func ParseStringSlice(input string, sep string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return []string{}
	}

	splitStr := strings.Split(input, sep)
	ret := make([]string, len(splitStr))
	for i, val := range splitStr {
		ret[i] = strings.TrimSpace(val)
	}

	return ret
}

// TrimStrings takes a slice of strings and returns a slice of strings
// with trimmed spaces
func TrimStrings(items []string) []string {
	ret := make([]string, len(items))
	for i, item := range items {
		ret[i] = strings.TrimSpace(item)
	}
	return ret
}

// StrListContains looks for a string in a list of strings.
func StrListContains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}

// EqualStringMaps tests whether two map[string]string objects are equal.
// Equal means both maps have the same sets of keys and values. This function
// is 6-10x faster than a call to reflect.DeepEqual().
func EqualStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}

	for k := range a {
		v, ok := b[k]
		if !ok || a[k] != v {
			return false
		}
	}

	return true
}

// RemoveDuplicates removes duplicate and empty elements from a slice of
// strings. This also may convert the items in the slice to lower case and
// returns a sorted slice.
func RemoveDuplicates(items []string, lowercase bool) []string {
	itemsMap := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if lowercase {
			item = strings.ToLower(item)
		}
		if item == "" {
			continue
		}
		itemsMap[item] = true
	}
	items = make([]string, 0, len(itemsMap))
	for item := range itemsMap {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}
