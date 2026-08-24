package program

import "github.com/sebishogun/verifoxx2/internal/schema"

func (p Program) LookupSymbol(value string) (schema.SymbolID, bool) {
	for i := range p.SymbolStarts {
		start := int(p.SymbolStarts[i])
		length := int(p.SymbolLengths[i])
		if length != len(value) || start < 0 || start+length > len(p.SymbolBytes) {
			continue
		}
		match := true
		for j := 0; j < length; j++ {
			if p.SymbolBytes[start+j] != value[j] {
				match = false
				break
			}
		}
		if match {
			return schema.SymbolID(i + 1), true
		}
	}
	return 0, false
}

func (p Program) Symbol(id schema.SymbolID) string {
	if !id.Valid() || int(id) > len(p.SymbolStarts) || len(p.SymbolStarts) != len(p.SymbolLengths) {
		return ""
	}
	i := int(id) - 1
	start := int(p.SymbolStarts[i])
	end := start + int(p.SymbolLengths[i])
	if start < 0 || end < start || end > len(p.SymbolBytes) {
		return ""
	}
	return string(p.SymbolBytes[start:end])
}
