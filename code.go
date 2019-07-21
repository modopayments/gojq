package gojq

type code struct {
	op opcode
	v  interface{}
}

type opcode int

const (
	opnop opcode = iota
	oppush
	oppop
	opdup
	opswap
	opconst
	opload
	opstore
	opobject
	opappend
	opfork
	opforkopt
	opforklabel
	opbacktrack
	opjump
	opjumpifnot
	opcall
	oppushpc
	opcallpc
	opscope
	opret
	opeach
	oppathbegin
	oppathend
)

func (op opcode) String() string {
	switch op {
	case opnop:
		return "nop"
	case oppush:
		return "push"
	case oppop:
		return "pop"
	case opdup:
		return "dup"
	case opswap:
		return "swap"
	case opconst:
		return "const"
	case opload:
		return "load"
	case opstore:
		return "store"
	case opobject:
		return "object"
	case opappend:
		return "append"
	case opfork:
		return "fork"
	case opforkopt:
		return "forkopt"
	case opforklabel:
		return "forklabel"
	case opbacktrack:
		return "backtrack"
	case opjump:
		return "jump"
	case opjumpifnot:
		return "jumpifnot"
	case opcall:
		return "call"
	case oppushpc:
		return "pushpc"
	case opcallpc:
		return "callpc"
	case opscope:
		return "scope"
	case opret:
		return "ret"
	case opeach:
		return "each"
	case oppathbegin:
		return "pathbegin"
	case oppathend:
		return "pathend"
	default:
		panic(op)
	}
}
