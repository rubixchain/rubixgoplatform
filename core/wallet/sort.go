package wallet

import "sort"

type By func(p1, p2 *Token) bool

func (by By) Sort(tokens []Token) {
	ps := &tokenSorter{
		tokens: tokens,
		by:     by, // The Sort method's receiver is the function (closure) that defines the sort order.
	}
	sort.Sort(ps)
}

// planetSorter joins a By function and a slice of Planets to be sorted.
type tokenSorter struct {
	tokens []Token
	by     func(t1, t2 *Token) bool // Closure used in the Less method.
}

// Len is part of sort.Interface.
func (s *tokenSorter) Len() int {
	return len(s.tokens)
}

// Swap is part of sort.Interface.
func (s *tokenSorter) Swap(i, j int) {
	s.tokens[i], s.tokens[j] = s.tokens[j], s.tokens[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *tokenSorter) Less(i, j int) bool {
	return s.by(&s.tokens[i], &s.tokens[j])
}

func TokenSort(t []Token, dsc bool) {
	ascFunc := func(t1, t2 *Token) bool {
		return t1.TokenValue < t2.TokenValue
	}
	dscFunc := func(t1, t2 *Token) bool {
		return ascFunc(t2, t1)
	}
	if dsc {
		By(dscFunc).Sort(t)
	} else {
		By(ascFunc).Sort(t)
	}
}
