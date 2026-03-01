package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
)

func fail(msg string) {
	fmt.Printf("FAIL: %s\n", msg)
	os.Exit(1)
}

func testErrors() {
	err := errors.New("err-x")
	if err == nil || err.Error() != "err-x" {
		fail("errors.New/error")
	}
	if errors.Unwrap(err) != nil {
		fail("errors.Unwrap(non-wrapper)")
	}
	if errors.Unwrap(nil) != nil {
		fail("errors.Unwrap(nil)")
	}
}

func testStrconv() {
	if strconv.FormatInt(-255, 16) != "-ff" {
		fail("strconv.FormatInt neg")
	}
	if strconv.FormatInt(5, 1) != "5" {
		fail("strconv.FormatInt invalid-base")
	}
	if strconv.Itoa(42) != "42" {
		fail("strconv.Itoa")
	}

	n, err := strconv.ParseInt("0xff", 0, 64)
	if err != nil || n != 255 {
		fail("strconv.ParseInt hex-prefix")
	}
	n, err = strconv.ParseInt("077", 0, 64)
	if err != nil || n != 63 {
		fail("strconv.ParseInt octal-prefix")
	}
	n, err = strconv.ParseInt("-10", 10, 8)
	if err != nil || n != -10 {
		fail("strconv.ParseInt signed")
	}
	if _, err = strconv.ParseInt("128", 10, 8); err == nil {
		fail("strconv.ParseInt range")
	}
	if _, err = strconv.ParseInt("", 10, 0); err == nil {
		fail("strconv.ParseInt syntax")
	}
	if _, err = strconv.Atoi("bad"); err == nil {
		fail("strconv.Atoi syntax")
	}

	numErr := &strconv.NumError{Func: "Atoi", Num: "x", Err: strconv.ErrSyntax}
	msg := numErr.Error()
	if !strings.Contains(msg, "Atoi: parsing \"x\"") {
		fail("strconv.NumError context")
	}
	if !strings.Contains(msg, "invalid syntax") {
		fail("strconv.NumError cause")
	}
}

func testBytes() {
	if bytes.Compare([]byte("ab"), []byte("ab")) != 0 {
		fail("bytes.Compare eq")
	}
	if bytes.Compare([]byte("ab"), []byte("a")) <= 0 {
		fail("bytes.Compare longer")
	}
	if bytes.Index([]byte("abc"), []byte("")) != 0 {
		fail("bytes.Index empty")
	}
	if bytes.Index([]byte("abc"), []byte("zz")) != -1 {
		fail("bytes.Index missing")
	}
	if bytes.Contains([]byte("abc"), []byte("zz")) {
		fail("bytes.Contains false")
	}
	if !bytes.HasPrefix([]byte("gopher"), []byte("go")) {
		fail("bytes.HasPrefix true")
	}
	if bytes.HasPrefix([]byte("gopher"), []byte("oh")) {
		fail("bytes.HasPrefix false")
	}
	if !bytes.HasSuffix([]byte("gopher"), []byte("her")) {
		fail("bytes.HasSuffix true")
	}
	if bytes.HasSuffix([]byte("gopher"), []byte("go")) {
		fail("bytes.HasSuffix false")
	}

	buf := bytes.NewBuffer([]byte("abc"))
	if buf.Len() != 3 {
		fail("bytes.Buffer.Len")
	}
	if buf.Cap() < 3 {
		fail("bytes.Buffer.Cap")
	}
	if string(buf.Bytes()) != "abc" {
		fail("bytes.Buffer.Bytes")
	}

	c, err := buf.ReadByte()
	if err != nil || c != 'a' {
		fail("bytes.Buffer.ReadByte")
	}
	if string(buf.Next(1)) != "b" {
		fail("bytes.Buffer.Next")
	}
	tmp := make([]byte, 2)
	n, err := buf.Read(tmp)
	if err != nil || n != 1 || tmp[0] != 'c' {
		fail("bytes.Buffer.Read tail")
	}
	n, err = buf.Read(tmp)
	if n != 0 || err == nil {
		fail("bytes.Buffer.Read eof")
	}

	_, _ = buf.WriteString("xy")
	_ = buf.WriteByte('z')
	if buf.String() != "xyz" {
		fail("bytes.Buffer.Write*")
	}
	buf.Reset()
	if buf.Len() != 0 {
		fail("bytes.Buffer.Reset")
	}
	if len(buf.Bytes()) != 0 {
		fail("bytes.Buffer.Bytes empty")
	}
}

func testBufio() {
	src1 := bytes.NewBufferString("line1\nline2")
	r1 := bufio.NewReader(src1)
	part, err := r1.ReadBytes('\n')
	if err != nil || string(part) != "line1\n" {
		fail("bufio.Reader.ReadBytes")
	}
	part2, err := r1.ReadString('\n')
	if err == nil || part2 != "line2" {
		fail("bufio.Reader.ReadString tail")
	}

	src2 := bytes.NewBufferString("0123456789abcdef")
	r2 := bufio.NewReaderSize(src2, 16)
	if r2.Buffered() != 0 {
		fail("bufio.Reader.Buffered initial")
	}
	block := make([]byte, 16)
	n, err := r2.Read(block)
	if err != nil || n != 16 || string(block) != "0123456789abcdef" {
		fail("bufio.Reader.Read direct")
	}
	if r2.Buffered() != 0 {
		fail("bufio.Reader.Buffered after-read")
	}

	src3 := bytes.NewBufferString("xy")
	r3 := bufio.NewReaderSize(src3, 4)
	ch, err := r3.ReadByte()
	if err != nil || ch != 'x' {
		fail("bufio.Reader.ReadByte")
	}

	var out1 bytes.Buffer
	w1 := bufio.NewWriterSize(&out1, 4)
	if w1.Buffered() != 0 {
		fail("bufio.Writer.Buffered initial")
	}
	_, _ = w1.WriteString("ab")
	_ = w1.WriteByte('c')
	if w1.Buffered() != 3 {
		fail("bufio.Writer.Buffered pending")
	}
	_, _ = w1.Write([]byte("defg"))
	if w1.Flush() != nil {
		fail("bufio.Writer.Flush")
	}
	if out1.String() != "abcdefg" {
		fail("bufio.Writer content")
	}

	var out2 bytes.Buffer
	w2 := bufio.NewWriterSize(&out2, 4)
	_, err = w2.Write([]byte("12345678901234567890"))
	if err != nil {
		fail("bufio.Writer direct write err")
	}
	if out2.String() != "12345678901234567890" {
		fail("bufio.Writer direct write")
	}
}

func testFlag() {
	fs := flag.NewFlagSet("demo")
	if fs.Parsed() {
		fail("flag.Parsed initial")
	}
	bp := fs.Bool("b", false, "")
	ip := fs.Int("n", 7, "")
	sp := fs.String("s", "def", "")
	if fs.Set("missing", "1") == nil {
		fail("flag.Set unknown")
	}
	if fs.Set("b", "maybe") == nil {
		fail("flag.Set bool invalid")
	}
	err := fs.Parse([]string{"-b=true", "-n", "42", "-s=ok", "--", "tail1", "tail2"})
	if err != nil {
		fail("flag.FlagSet.Parse")
	}
	_ = bp
	if *ip != 42 || *sp != "ok" {
		fail("flag.Int/String value")
	}
	if !fs.Parsed() {
		fail("flag.Parsed true")
	}
	if fs.NArg() != 2 || fs.Arg(0) != "tail1" || fs.Arg(1) != "tail2" {
		fail("flag.Args/Arg/NArg")
	}
	err = fs.Parse([]string{"-unknown"})
	if err == nil {
		fail("flag.Parse unknown")
	}
	err = fs.Parse([]string{"-n"})
	if err == nil {
		fail("flag.Parse missing arg")
	}

	oldArgs := os.Args
	flag.CommandLine = flag.NewFlagSet("cmd")
	gInt := flag.Int("count", 1, "")
	gStr := flag.String("name", "x", "")
	gBool := flag.Bool("v", false, "")
	os.Args = []string{"cmd", "-count=3", "-name", "rtg", "-v", "arg1"}
	flag.Parse()
	os.Args = oldArgs
	if *gInt != 3 || *gStr != "rtg" {
		fail("flag global values")
	}
	_ = gBool
	if !flag.Parsed() {
		fail("flag global parsed")
	}
	if flag.NArg() != 1 || flag.Arg(0) != "arg1" {
		fail("flag global args")
	}
}

func testLog() {
	var b1 bytes.Buffer
	l := log.New(&b1, "pre:", 123)
	if l.Prefix() != "pre:" || l.Flags() != 123 {
		fail("log.New/Prefix/Flags")
	}
	l.SetPrefix("p2:")
	l.SetFlags(7)
	if l.Prefix() != "p2:" || l.Flags() != 7 {
		fail("log.SetPrefix/SetFlags")
	}
	l.Print("x")
	l.Printf("n=%d", 2)
	l.Println("ab")
	if !bytes.Contains(b1.Bytes(), []byte("p2:x")) {
		fail("log.Logger.Print")
	}
	if !bytes.Contains(b1.Bytes(), []byte("p2:n=2")) {
		fail("log.Logger.Printf")
	}
	if !bytes.Contains(b1.Bytes(), []byte("p2:ab")) {
		fail("log.Logger.Println")
	}

	log.SetPrefix("glob:")
	if log.Prefix() != "glob:" {
		fail("log.SetPrefix")
	}
	log.SetFlags(9)
	if log.Flags() != 9 {
		fail("log.SetFlags")
	}
}

func testTestingPkg() {
	oldArgs := os.Args
	os.Args = []string{"prog", "-v", "-run=Add", "-bench", "Bench"}
	runPattern, benchPattern, verbose := testing.ParseTestArgs()
	os.Args = oldArgs
	if runPattern != "Add" || benchPattern != "Bench" || !verbose {
		fail("testing.ParseTestArgs")
	}
	if !testing.Match("TestAdd", "Add") {
		fail("testing.Match true")
	}
	if testing.Match("TestAdd", "Nope") {
		fail("testing.Match false")
	}
	if !testing.IsFailNow("rtg.testing.failnow") {
		fail("testing.IsFailNow")
	}
	if testing.PanicString("abc") != "abc" {
		fail("testing.PanicString")
	}

	t := testing.BeginTest("manual", false)
	if t == nil || t.Failed() {
		fail("testing.BeginTest")
	}
	t.Fail()
	if !t.Failed() {
		fail("testing.T.Fail")
	}
	testing.FinishTest(t, "manual", false)

	b := testing.BeginBenchmark("manualBench", false)
	if b == nil {
		fail("testing.BeginBenchmark")
	}
	b.ResetTimer()
	b.StartTimer()
	b.StopTimer()
	_ = b.Elapsed()
	b.SetBytes(10)
	b.Fail()
	if !b.Failed() {
		fail("testing.B.Fail")
	}
	testing.FinishBenchmark(b, "manualBench", false)
}

func main() {
	testErrors()
	testStrconv()
	testBytes()
	testBufio()
	testFlag()
	testLog()
	testTestingPkg()
	fmt.Print("PASS")
}
