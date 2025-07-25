package gojq

type scopeStack struct {
	data  []scopeBlock
	index int
	limit int
}

func (s *scopeStack) reset() {
	s.data = s.data[:0]
	s.index = -1
	s.limit = -1
}

type scopeBlock struct {
	value scope
	next  int
}

func newScopeStack() *scopeStack {
	return &scopeStack{index: -1, limit: -1}
}

func (s *scopeStack) push(v scope) {
	b := scopeBlock{v, s.index}
	s.index = max(s.index, s.limit) + 1
	if s.index < len(s.data) {
		s.data[s.index] = b
	} else {
		s.data = append(s.data, b)
	}
}

func (s *scopeStack) pop() scope {
	b := s.data[s.index]
	s.index = b.next
	return b.value
}

func (s *scopeStack) empty() bool {
	return s.index < 0
}

func (s *scopeStack) save() (index, limit int) {
	index, limit = s.index, s.limit
	if s.index > s.limit {
		s.limit = s.index
	}
	return
}

func (s *scopeStack) restore(index, limit int) {
	s.index, s.limit = index, limit
}
