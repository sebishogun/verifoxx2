package compile

import (
	"strings"

	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

type interner struct {
	ids    map[string]schema.SymbolID
	values []string
}

func newInterner() interner {
	return interner{ids: make(map[string]schema.SymbolID)}
}

func (in *interner) intern(value string) schema.SymbolID {
	if value == "" {
		return 0
	}
	if id, ok := in.ids[value]; ok {
		return id
	}
	id := schema.SymbolID(len(in.values) + 1)
	in.ids[value] = id
	in.values = append(in.values, value)
	return id
}

func (in *interner) freeze(dst *program.Program) {
	total := 0
	for _, value := range in.values {
		total += len(value)
	}
	var text strings.Builder
	text.Grow(total)
	dst.SymbolStarts = make([]uint32, len(in.values))
	dst.SymbolLengths = make([]uint32, len(in.values))
	for i, value := range in.values {
		dst.SymbolStarts[i] = uint32(text.Len())
		dst.SymbolLengths[i] = uint32(len(value))
		text.WriteString(value)
	}
	dst.SymbolText = text.String()
}
