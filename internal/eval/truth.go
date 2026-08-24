package eval

import "github.com/sebishogun/verifoxx2/internal/schema"

func setRow(plane []uint64, row uint32) {
	plane[row>>6] |= uint64(1) << (row & 63)
}

func hasRow(plane []uint64, row uint32) bool {
	return plane[row>>6]&(uint64(1)<<(row&63)) != 0
}

func setReason(context *Context, reason schema.ReasonID, instruction schema.InstructionID, row uint32) {
	setRow(context.ReasonPlane(reason, instruction), row)
}

func tailMask(rows uint32) uint64 {
	if tail := rows & 63; tail != 0 {
		return (uint64(1) << tail) - 1
	}
	return ^uint64(0)
}
