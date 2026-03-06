package aarch64

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"j5.nz/rtg/std/compiler/binary"
	"j5.nz/rtg/std/compiler/ir"
)

type debugFuncSpan struct {
	name  string
	start int
	end   int
}

type debugBufferedWriter struct {
	f   *os.File
	buf []byte
	err error
}

func newDebugBufferedWriter(f *os.File) *debugBufferedWriter {
	return &debugBufferedWriter{
		f:   f,
		buf: make([]byte, 0, 4096),
	}
}

func (w *debugBufferedWriter) WriteByte(ch byte) {
	if w.err != nil {
		return
	}
	if len(w.buf) == cap(w.buf) {
		w.flush()
		if w.err != nil {
			return
		}
	}
	w.buf = append(w.buf, ch)
}

func (w *debugBufferedWriter) WriteString(s string) {
	if w.err != nil {
		return
	}
	for len(s) > 0 {
		if len(w.buf) == cap(w.buf) {
			w.flush()
			if w.err != nil {
				return
			}
		}
		space := cap(w.buf) - len(w.buf)
		if space > len(s) {
			space = len(s)
		}
		w.buf = append(w.buf, s[:space]...)
		s = s[space:]
	}
}

func (w *debugBufferedWriter) WriteInt(v int) {
	w.WriteString(strconv.Itoa(v))
}

func (w *debugBufferedWriter) flush() {
	if w.err != nil || len(w.buf) == 0 {
		return
	}
	start := 0
	for start < len(w.buf) {
		n, err := w.f.Write(w.buf[start:])
		if n > 0 {
			start = start + n
		}
		if err != nil {
			if start >= len(w.buf) {
				break
			}
			w.err = err
			return
		}
		if n == 0 {
			w.err = fmt.Errorf("write made no progress")
			return
		}
	}
	w.buf = w.buf[:0]
}

func (w *debugBufferedWriter) Finish() error {
	w.flush()
	return w.err
}

func debugHexNibble(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + (v - 10)
}

func debugHexEncodeSpan(code []byte, start int, end int) string {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(code) {
		start = len(code)
	}
	if end > len(code) {
		end = len(code)
	}
	n := end - start
	if n <= 0 {
		return ""
	}
	out := make([]byte, n*2)
	for i := 0; i < n; i++ {
		b := code[start+i]
		out[i*2] = debugHexNibble((b >> 4) & 0x0f)
		out[i*2+1] = debugHexNibble(b & 0x0f)
	}
	return string(out)
}

func debugQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if ch < 32 {
				b.WriteString("\\x")
				b.WriteByte(debugHexNibble((ch >> 4) & 0x0f))
				b.WriteByte(debugHexNibble(ch & 0x0f))
			} else {
				b.WriteByte(ch)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func debugOpcodeName(op ir.Opcode) string {
	return binary.OpcodeName(op)
}

func normalizeSegments(raw []InstByteSegment, minStart int, maxEnd int) []InstByteSegment {
	if len(raw) == 0 {
		return nil
	}
	segments := make([]InstByteSegment, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		seg := raw[i]
		if seg.End <= seg.Start {
			continue
		}
		if seg.Start < minStart {
			seg.Start = minStart
		}
		if seg.End > maxEnd {
			seg.End = maxEnd
		}
		if seg.End <= seg.Start {
			continue
		}
		segments = append(segments, seg)
	}
	if len(segments) == 0 {
		return nil
	}
	// Insertion sort by start,end.
	for i := 1; i < len(segments); i++ {
		j := i
		for j > 0 {
			left := segments[j-1]
			right := segments[j]
			if left.Start < right.Start || (left.Start == right.Start && left.End <= right.End) {
				break
			}
			segments[j-1] = right
			segments[j] = left
			j = j - 1
		}
	}
	merged := make([]InstByteSegment, 0, len(segments))
	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		if len(merged) == 0 {
			merged = append(merged, seg)
			continue
		}
		last := len(merged) - 1
		if seg.Start <= merged[last].End {
			if seg.End > merged[last].End {
				merged[last].End = seg.End
			}
			continue
		}
		merged = append(merged, seg)
	}
	return merged
}

func segmentByteCount(segments []InstByteSegment) int {
	total := 0
	for i := 0; i < len(segments); i++ {
		if segments[i].End > segments[i].Start {
			total = total + (segments[i].End - segments[i].Start)
		}
	}
	return total
}

func callTargetForSegments(segments []InstByteSegment, callTargets map[int]string) string {
	if len(segments) == 0 || len(callTargets) == 0 {
		return ""
	}
	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		for off, target := range callTargets {
			if off >= seg.Start && off < seg.End {
				return target
			}
		}
	}
	return ""
}

func readU32LE(code []byte, off int) uint32 {
	return uint32(code[off]) | uint32(code[off+1])<<8 | uint32(code[off+2])<<16 | uint32(code[off+3])<<24
}

func callTargetFromBL(segments []InstByteSegment, code []byte, targetByOffset map[int]string) string {
	if len(segments) == 0 || len(targetByOffset) == 0 {
		return ""
	}
	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		off := seg.Start
		for off+4 <= seg.End {
			inst := readU32LE(code, off)
			if (inst & 0xFC000000) == 0x94000000 {
				imm := int32(inst & 0x03FFFFFF)
				if (imm & 0x02000000) != 0 {
					imm = imm | ^int32(0x03FFFFFF)
				}
				targetOff := off + int(imm)*4
				if name, ok := targetByOffset[targetOff]; ok && name != "" {
					return name
				}
			}
			off = off + 4
		}
	}
	return ""
}

func collectDebugFuncSpans(funcOffsets map[string]int, textSize int) []debugFuncSpan {
	if len(funcOffsets) == 0 {
		return nil
	}
	spans := make([]debugFuncSpan, 0, len(funcOffsets))
	for name, off := range funcOffsets {
		if off < 0 || off > textSize {
			continue
		}
		spans = append(spans, debugFuncSpan{name: name, start: off})
	}
	// Insertion sort by start,name.
	for i := 1; i < len(spans); i++ {
		j := i
		for j > 0 {
			left := spans[j-1]
			right := spans[j]
			if left.start < right.start || (left.start == right.start && left.name <= right.name) {
				break
			}
			spans[j-1] = right
			spans[j] = left
			j = j - 1
		}
	}
	for i := 0; i < len(spans); i++ {
		end := textSize
		start := spans[i].start
		for j := i + 1; j < len(spans); j++ {
			if spans[j].start > start {
				end = spans[j].start
				break
			}
		}
		spans[i].end = end
	}
	return spans
}

func writeInstLine(w *debugBufferedWriter, idx int, inst ir.Inst, segments []InstByteSegment, code []byte, callTargets map[int]string, targetByOffset map[int]string) {
	size := segmentByteCount(segments)
	if size <= 0 {
		return
	}
	w.WriteString("  inst idx=")
	w.WriteInt(idx)
	w.WriteString(" op=")
	w.WriteString(debugOpcodeName(inst.Op))
	if inst.Op == ir.OP_CALL {
		target := inst.Name
		if target == "" {
			target = callTargetForSegments(segments, callTargets)
		}
		if target == "" {
			target = callTargetFromBL(segments, code, targetByOffset)
		}
		if target != "" {
			w.WriteString(" target=")
			w.WriteString(debugQuote(target))
		}
	}
	if inst.Op == ir.OP_CALL_INTRINSIC && inst.Name != "" {
		w.WriteString(" intrinsic=")
		w.WriteString(debugQuote(inst.Name))
	}
	if inst.Op == ir.OP_IFACE_CALL && inst.Name != "" {
		w.WriteString(" target=")
		w.WriteString(debugQuote(inst.Name))
	}
	if len(segments) == 1 {
		seg := segments[0]
		w.WriteString(" off=")
		w.WriteInt(seg.Start)
		w.WriteString(" size=")
		w.WriteInt(seg.End - seg.Start)
		w.WriteString(" hex=")
		w.WriteString(debugHexEncodeSpan(code, seg.Start, seg.End))
		w.WriteString("\n")
		return
	}
	w.WriteString(" parts=")
	w.WriteInt(len(segments))
	w.WriteString(" size=")
	w.WriteInt(size)
	w.WriteString(" {\n")
	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		w.WriteString("    part off=")
		w.WriteInt(seg.Start)
		w.WriteString(" size=")
		w.WriteInt(seg.End - seg.Start)
		w.WriteString(" hex=")
		w.WriteString(debugHexEncodeSpan(code, seg.Start, seg.End))
		w.WriteString("\n")
	}
	w.WriteString("  }\n")
}

func writeFuncBlock(w *debugBufferedWriter, name string, start int, end int, code []byte, f *ir.IRFunc, traces []InstByteTrace, callTargets map[int]string, targetByOffset map[int]string) {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(code) {
		start = len(code)
	}
	if end > len(code) {
		end = len(code)
	}
	size := end - start
	if f == nil {
		w.WriteString("section name=")
		w.WriteString(debugQuote(name))
		w.WriteString(" off=")
		w.WriteInt(start)
		w.WriteString(" size=")
		w.WriteInt(size)
		w.WriteString("\n")
		w.WriteString("  unmapped off=")
		w.WriteInt(start)
		w.WriteString(" size=")
		w.WriteInt(size)
		w.WriteString(" hex=")
		w.WriteString(debugHexEncodeSpan(code, start, end))
		w.WriteString("\n")
		return
	}

	w.WriteString("func name=")
	w.WriteString(debugQuote(name))
	w.WriteString(" off=")
	w.WriteInt(start)
	w.WriteString(" size=")
	w.WriteInt(size)
	w.WriteString(" ir_insts=")
	w.WriteInt(len(f.Code))
	w.WriteString(" {\n")

	coverageRaw := make([]InstByteSegment, 0, len(f.Code))
	emittedInsts := 0
	zeroInsts := 0
	for i := 0; i < len(f.Code); i++ {
		var segs []InstByteSegment
		if i < len(traces) {
			segs = normalizeSegments(traces[i].Segments, start, end)
		}
		if segmentByteCount(segs) == 0 {
			zeroInsts = zeroInsts + 1
			continue
		}
		emittedInsts = emittedInsts + 1
		coverageRaw = append(coverageRaw, segs...)
		writeInstLine(w, i, f.Code[i], segs, code, callTargets, targetByOffset)
	}

	covered := normalizeSegments(coverageRaw, start, end)
	cursor := start
	for i := 0; i < len(covered); i++ {
		seg := covered[i]
		if cursor < seg.Start {
			w.WriteString("  unmapped off=")
			w.WriteInt(cursor)
			w.WriteString(" size=")
			w.WriteInt(seg.Start - cursor)
			w.WriteString(" hex=")
			w.WriteString(debugHexEncodeSpan(code, cursor, seg.Start))
			w.WriteString("\n")
		}
		if seg.End > cursor {
			cursor = seg.End
		}
	}
	if cursor < end {
		w.WriteString("  unmapped off=")
		w.WriteInt(cursor)
		w.WriteString(" size=")
		w.WriteInt(end - cursor)
		w.WriteString(" hex=")
		w.WriteString(debugHexEncodeSpan(code, cursor, end))
		w.WriteString("\n")
	}
	w.WriteString("  stats emitted_insts=")
	w.WriteInt(emittedInsts)
	w.WriteString(" zero_insts=")
	w.WriteInt(zeroInsts)
	w.WriteString("\n")
	w.WriteString("}\n")
}

func writeDebugBinarySection(out *os.File, g *CodeGen, irmod *ir.IRModule) error {
	w := newDebugBufferedWriter(out)
	w.WriteString("rtgdbg 1\n")
	w.WriteString("arch aarch64\n")
	w.WriteString("target ")
	if g.target != nil {
		w.WriteString(debugQuote(g.target.GOOS + "/" + g.target.GOARCH))
	} else {
		w.WriteString(debugQuote("unknown"))
	}
	w.WriteString("\n")
	w.WriteString("text_size ")
	w.WriteInt(len(g.code))
	w.WriteString("\n\n")

	funcByName := make(map[string]*ir.IRFunc)
	for i := 0; i < len(irmod.Funcs); i++ {
		funcByName[irmod.Funcs[i].Name] = irmod.Funcs[i]
	}
	traceByName := g.FuncInstTraces()
	callTargets := make(map[int]string)
	for i := 0; i < len(g.callFixups); i++ {
		fix := g.callFixups[i]
		if fix.Target == "" {
			continue
		}
		callTargets[fix.CodeOffset] = fix.Target
	}
	targetByOffset := make(map[int]string)
	for name, off := range g.funcOffsets {
		if cur, ok := targetByOffset[off]; !ok || name < cur {
			targetByOffset[off] = name
		}
	}
	spans := collectDebugFuncSpans(g.funcOffsets, len(g.code))
	if len(spans) == 0 {
		writeFuncBlock(w, "$text", 0, len(g.code), g.code, nil, nil, callTargets, targetByOffset)
		return w.Finish()
	}
	if spans[0].start > 0 {
		writeFuncBlock(w, "$entry", 0, spans[0].start, g.code, nil, nil, callTargets, targetByOffset)
		w.WriteString("\n")
	}
	for i := 0; i < len(spans); i++ {
		name := spans[i].name
		writeFuncBlock(w, name, spans[i].start, spans[i].end, g.code, funcByName[name], traceByName[name], callTargets, targetByOffset)
		if i+1 < len(spans) {
			w.WriteString("\n")
		}
	}
	return w.Finish()
}

func WriteIRAndBinaryDebug(path string, irmod *ir.IRModule, g *CodeGen) error {
	if irmod == nil {
		return fmt.Errorf("nil IR module")
	}
	if g == nil {
		return fmt.Errorf("nil code generator")
	}
	if path == "" {
		return fmt.Errorf("empty debug output path")
	}
	if path == "-" {
		return writeDebugBinarySection(os.Stdout, g, irmod)
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := writeDebugBinarySection(out, g, irmod); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
