package main

var ok bool

func helperRecover() {
	_ = recover()
}

func indirectRecover() {
	helperRecover()
}

func finalRecover() {
	if recover() != nil {
		ok = true
	}
	if ok {
		println("PASS")
	} else {
		println("FAIL")
	}
}

func main() {
	defer finalRecover()
	defer indirectRecover()
	panic("boom")
}
