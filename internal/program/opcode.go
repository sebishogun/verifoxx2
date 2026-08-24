package program

type Opcode uint8

const (
	OpInvalid Opcode = iota
	OpEqual
	OpIn
	OpEvidenceMatches
	OpAll
)

func (op Opcode) Valid() bool {
	return op >= OpEqual && op <= OpAll
}
