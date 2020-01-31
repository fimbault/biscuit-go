package datalog

import (
	"fmt"
	"regexp"
	"strings"
)

type Matcher interface {
	Match(ID) bool
}

type IDType byte

const (
	IDTypeSymbol IDType = iota
	IDTypeVariable
	IDTypeInteger
	IDTypeString
	IDTypeDate
)

type ID interface {
	Type() IDType
}

type Symbol uint64

func (Symbol) Type() IDType { return IDTypeSymbol }

type Variable uint32

func (Variable) Type() IDType { return IDTypeVariable }

type Integer int64

func (Integer) Type() IDType { return IDTypeInteger }

type String string

func (String) Type() IDType { return IDTypeString }

type Date uint64

func (Date) Type() IDType { return IDTypeDate }

type IntegerComparison byte

const (
	IntegerComparisonEqual IntegerComparison = iota
	IntegerComparisonLT
	IntegerComparisonGT
	IntegerComparisonLTE
	IntegerComparisonGTE
)

type IntegerComparisonMatcher struct {
	Comparison IntegerComparison
	Integer    Integer
}

func (m *IntegerComparisonMatcher) Match(id ID) bool {
	if id.Type() != IDTypeInteger {
		return false
	}
	v := id.(Integer)
	switch m.Comparison {
	case IntegerComparisonEqual:
		return v == m.Integer
	case IntegerComparisonLT:
		return v < m.Integer
	case IntegerComparisonGT:
		return v > m.Integer
	case IntegerComparisonLTE:
		return v <= m.Integer
	case IntegerComparisonGTE:
		return v >= m.Integer
	default:
		return false
	}
}

type IntegerInMatcher struct {
	Set map[Integer]struct{}
	Not bool
}

func (m *IntegerInMatcher) Match(id ID) bool {
	i, ok := id.(Integer)
	if !ok {
		return false
	}
	_, match := m.Set[i]
	return match == !m.Not
}

type StringComparison byte

const (
	StringComparisonEqual StringComparison = iota
	StringComparisonPrefix
	StringComparisonSuffix
)

type StringComparisonMatcher struct {
	Comparison StringComparison
	String     String
}

func (m *StringComparisonMatcher) Match(id ID) bool {
	v, ok := id.(String)
	if !ok {
		return false
	}
	switch m.Comparison {
	case StringComparisonEqual:
		return m.String == v
	case StringComparisonPrefix:
		return strings.HasPrefix(string(v), string(m.String))
	case StringComparisonSuffix:
		return strings.HasSuffix(string(v), string(m.String))
	default:
		return false
	}
}

type StringInMatcher struct {
	Set map[String]struct{}
	Not bool
}

func (m *StringInMatcher) Match(id ID) bool {
	s, ok := id.(String)
	if !ok {
		return false
	}
	_, match := m.Set[s]
	return match == !m.Not
}

type StringRegexpMatcher regexp.Regexp

func (m *StringRegexpMatcher) Match(id ID) bool {
	s, ok := id.(String)
	if !ok {
		return false
	}
	return (*regexp.Regexp)(m).MatchString(string(s))
}

type DateComparison byte

const (
	DateComparisonBefore DateComparison = iota
	DateComparisonAfter
)

type DateComparisonMatcher struct {
	Comparison DateComparison
	Date       Date
}

func (m *DateComparisonMatcher) Match(id ID) bool {
	v, ok := id.(Date)
	if !ok {
		return false
	}
	switch m.Comparison {
	case DateComparisonBefore:
		return v <= m.Date
	case DateComparisonAfter:
		return v >= m.Date
	default:
		return false
	}
}

type SymbolInMatcher struct {
	Set map[Symbol]struct{}
	Not bool
}

func (m *SymbolInMatcher) Match(id ID) bool {
	sym, ok := id.(Symbol)
	if !ok {
		return false
	}
	_, match := m.Set[sym]
	return match == !m.Not
}

type InvalidMatcher struct{}

func (InvalidMatcher) Match(ID) bool { return false }

type Predicate struct {
	Name Symbol
	IDs  []ID
}

func (p Predicate) Equal(p2 Predicate) bool {
	if p.Name != p2.Name || len(p.IDs) != len(p2.IDs) {
		return false
	}
	for i, id := range p.IDs {
		if id != p2.IDs[i] {
			return false
		}
	}

	return true
}

func (p Predicate) Match(p2 Predicate) bool {
	if p.Name != p2.Name || len(p.IDs) != len(p2.IDs) {
		return false
	}
	for i, id := range p.IDs {
		_, v1 := id.(Variable)
		_, v2 := p2.IDs[i].(Variable)
		if v1 || v2 {
			continue
		}
		if id != p2.IDs[i] {
			return false
		}
	}
	return true
}

func (p Predicate) Clone() Predicate {
	res := Predicate{Name: p.Name, IDs: make([]ID, len(p.IDs))}
	copy(res.IDs, p.IDs)
	return res
}

type Fact struct {
	Predicate
}

type Constraint struct {
	Name Variable
	Matcher
}

func (c Constraint) Check(name Variable, id ID) bool {
	if c.Name != name {
		return true
	}
	if _, ok := id.(Variable); ok {
		panic("should not check constraint on a variable")
	}
	return c.Match(id)
}

type Rule struct {
	Head        Predicate
	Body        []Predicate
	Constraints []Constraint
}

type InvalidRuleError struct {
	Rule            Rule
	MissingVariable Variable
}

func (e InvalidRuleError) Error() string {
	return fmt.Sprintf("datalog: variable %d in head is missing from body and/or constraints")
}

func (r Rule) Apply(facts *FactSet, newFacts *FactSet) error {
	variables := make(MatchedVariables)
	for _, p := range r.Body {
		for _, id := range p.IDs {
			v, ok := id.(Variable)
			if !ok {
				continue
			}
			variables[v] = nil
		}
	}

	for _, h := range NewCombinator(variables, r.Body, r.Constraints, facts).Combine() {
		p := r.Head.Clone()
		for i, id := range p.IDs {
			k, ok := id.(Variable)
			if !ok {
				continue
			}
			v, ok := h[k]
			if !ok {
				return InvalidRuleError{r, k}
			}
			p.IDs[i] = *v
		}
		newFacts.Insert(Fact{p})
	}

	return nil
}

type Caveat struct {
	Queries []Rule
}

type FactSet []Fact

func (s *FactSet) Insert(f Fact) bool {
	for _, v := range *s {
		if v.Equal(f.Predicate) {
			return false
		}
	}
	*s = append(*s, f)
	return true
}

func (s *FactSet) InsertAll(facts []Fact) {
	for _, f := range facts {
		s.Insert(f)
	}
}

type World struct {
	facts *FactSet
	rules []Rule
}

func (w *World) AddFact(f Fact) {
	w.facts.Insert(f)
}

func (w *World) AddRule(r Rule) {
	w.rules = append(w.rules, r)
}

func (w *World) Run() error {
	for i := 0; i < 100; i++ {
		var newFacts FactSet
		for _, r := range w.rules {
			if err := r.Apply(r.facts, &newFacts); err != nil {
				return err
			}
		}
		l := len(newFacts)
		w.facts.InsertAll([]Fact(newFacts))
		if len(w.facts) == l {
			return nil
		}
	}
	return fmt.Errorf("datalog: world ran more than 100 iterations")
}

func (w *World) Query(pred Predicate) *FactSet {
	res := &FactSet{}
	for _, f := range *w.facts {
		if f.Predicate.Name != pred.Name {
			continue
		}
		minLen := len(f.Predicate.IDs)
		if l := len(pred.IDs); l < minLen {
			minLen = l
		}
		for i := 0; i < minLen; i++ {
			fID := f.Predicate.IDs[i]
			pID := pred.IDs[i]
			if fID.Type() != IDTypeVariable && fid.Type() == pid.Type() {
				if fID != pID {
					continue
				}
			} else if fID.Type() != IDTypeSymbol && pID.Type() != IDTypeVariable {
				continue
			}
			*res = append(*res, f)
		}
	}
	return res
}

func (w *World) QueryRule(rule Rule) *FactSet {
	newFacts := &FactSet{}
	rule.Apply(w.facts, newFacts)
	return newFacts
}

type MatchedVariables map[Variable]*ID

func NewMatchedVariables(vs map[Variable]struct{}) MatchedVariables {
	res := make(MatchedVariables, len(vs))
	for k := range vs {
		res[k] = nil
	}
	return res
}

func (m MatchedVariables) Insert(k Variable, v ID) bool {
	existing := m[k]
	if existing == nil {
		m[k] = &v
		return true
	}
	return *existing == v
}

func (m MatchedVariables) Complete() map[Variable]*ID {
	for _, v := range m {
		if v == nil {
			return nil
		}
	}
	return (map[Variable]*ID)(m)
}

func (m MatchedVariables) Clone() MatchedVariables {
	res := make(MatchedVariables, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}

type Combinator struct {
	variables    MatchedVariables
	predicates   []Predicate
	constraints  []Constraint
	allFacts     *FactSet
	currentFacts *FactSet
}

func NewCombinator(variables MatchedVariables, predicates []Predicate, constraints []Constraint, allFacts *FactSet) *Combinator {
	c := &Combinator{
		variables:   variables,
		predicates:  predicates,
		constraints: constraints,
		allFacts:    allFacts,
	}
	currentFacts := make(FactSet, 0, len(*allFacts))
	for _, f := range *allFacts {
		if f.Match(predicates[0]) {
			currentFacts = append(currentFacts, f)
		}
	}
	c.currentFacts = &currentFacts
	return c
}

func (c *Combinator) Combine() []map[Variable]*ID {
	var variables []map[Variable]*ID
	if len(c.predicates) == 0 {
		if vars := c.variables.Complete(); vars != nil {
			variables = append(variables, vars)
		}
		return variables
	}
	if len(*c.currentFacts) == 0 {
		return variables
	}

	for i, pred := range c.predicates {
		for _, currentFact := range *c.currentFacts {
			vars := c.variables.Clone()
			matchIDs := true
			minLen := len(pred.IDs)
			if l := len(currentFact.Predicate.IDs); l < minLen {
				minLen = l
			}

			for j := 0; j < minLen; j++ {
				id := pred.IDs[j]
				k, ok := id.(Variable)
				if !ok {
					continue
				}
				v := currentFact.Predicate.IDs[j]
				for _, con := range c.constraints {
					if !con.Check(k, v) {
						matchIDs = false
						break
					}
				}
				if !vars.Insert(k, v) {
					matchIDs = false
				}
				if !matchIDs {
					break
				}
			}

			if !matchIDs {
				continue
			}

			if len(c.predicates) > i+1 {
				variables = append(variables, NewCombinator(vars, c.predicates[i+1:], c.constraints, c.allFacts).Combine()...)
			} else {
				if v := vars.Complete(); v != nil {
					variables = append(variables, v)
				}
			}
		}
	}
	return variables
}