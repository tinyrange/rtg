package ir

// NativeFuncSpan describes the emitted text span for one compiled function.
type NativeFuncSpan struct {
	Start int
	End   int
}

// ComputeNativeFuncSpans computes native code spans from emitted function offsets.
// Functions that share the same emitted offset receive the same span.
func ComputeNativeFuncSpans(irmod *IRModule, funcOffsets map[string]int, codeLen int) map[string]NativeFuncSpan {
	uniqueStarts := make([]int, 0, len(irmod.Funcs))
	for _, f := range irmod.Funcs {
		off, ok := funcOffsets[f.Name]
		if !ok || off < 0 || off > codeLen {
			continue
		}
		found := false
		for i := 0; i < len(uniqueStarts); i++ {
			if uniqueStarts[i] == off {
				found = true
				break
			}
		}
		if found {
			continue
		}
		uniqueStarts = append(uniqueStarts, off)
	}
	for i := 1; i < len(uniqueStarts); i++ {
		j := i
		for j > 0 && uniqueStarts[j-1] > uniqueStarts[j] {
			uniqueStarts[j-1], uniqueStarts[j] = uniqueStarts[j], uniqueStarts[j-1]
			j--
		}
	}
	if len(uniqueStarts) == 0 {
		return nil
	}
	endByStart := make(map[int]int, len(uniqueStarts))
	for i := 0; i < len(uniqueStarts); i++ {
		end := codeLen
		if i+1 < len(uniqueStarts) {
			end = uniqueStarts[i+1]
		}
		if end < uniqueStarts[i] {
			end = uniqueStarts[i]
		}
		endByStart[uniqueStarts[i]] = end
	}
	spans := make(map[string]NativeFuncSpan, len(irmod.Funcs))
	for _, f := range irmod.Funcs {
		start, ok := funcOffsets[f.Name]
		if !ok {
			continue
		}
		end, ok := endByStart[start]
		if !ok {
			continue
		}
		spans[f.Name] = NativeFuncSpan{Start: start, End: end}
	}
	return spans
}

// FirstNativeFuncOffset returns the lowest emitted function offset in the module.
func FirstNativeFuncOffset(irmod *IRModule, funcOffsets map[string]int, codeLen int) (int, bool) {
	spans := ComputeNativeFuncSpans(irmod, funcOffsets, codeLen)
	first := codeLen
	found := false
	for _, f := range irmod.Funcs {
		span, ok := spans[f.Name]
		if !ok {
			continue
		}
		if !found || span.Start < first {
			first = span.Start
			found = true
		}
	}
	return first, found
}
