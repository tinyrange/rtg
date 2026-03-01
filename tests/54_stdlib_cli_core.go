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
)

func fail(msg string) {
	fmt.Printf("FAIL: %s\n", msg)
	os.Exit(1)
}

func main() {
	// errors
	base := errors.New("boom")
	if base == nil || base.Error() != "boom" {
		fail("errors.New")
	}
	if errors.Unwrap(base) != nil {
		fail("errors.Unwrap(non-wrapper)")
	}

	// strconv
	if strconv.Itoa(-42) != "-42" {
		fail("strconv.Itoa")
	}
	n, err := strconv.Atoi("12345")
	if err != nil || n != 12345 {
		fail("strconv.Atoi")
	}
	if _, err := strconv.Atoi("12x"); err == nil {
		fail("strconv.Atoi invalid")
	}
	n64, err := strconv.ParseInt("ff", 16, 64)
	if err != nil || n64 != 255 {
		fail("strconv.ParseInt")
	}
	if strconv.FormatInt(n64, 16) != "ff" {
		fail("strconv.FormatInt")
	}

	// bytes
	if !bytes.Equal([]byte("go"), []byte("go")) {
		fail("bytes.Equal")
	}
	if bytes.Compare([]byte("ab"), []byte("ac")) >= 0 {
		fail("bytes.Compare")
	}
	if bytes.Index([]byte("gopher"), []byte("ph")) != 2 {
		fail("bytes.Index")
	}
	if !bytes.Contains([]byte("gopher"), []byte("her")) {
		fail("bytes.Contains")
	}
	var b bytes.Buffer
	_, _ = b.WriteString("hel")
	_ = b.WriteByte('l')
	_, _ = b.Write([]byte("o"))
	tmp := make([]byte, 2)
	readN, readErr := b.Read(tmp)
	if readErr != nil || readN != 2 || string(tmp) != "he" {
		fail("bytes.Buffer.Read")
	}
	if string(b.Next(2)) != "ll" {
		fail("bytes.Buffer.Next")
	}
	if b.String() != "o" {
		fail("bytes.Buffer.String")
	}
	b.Reset()
	if b.Len() != 0 {
		fail("bytes.Buffer.Reset")
	}
	nb := bytes.NewBufferString("abc")
	if nb.String() != "abc" {
		fail("bytes.NewBufferString")
	}

	// bufio
	src := bytes.NewBufferString("line1\nline2")
	reader := bufio.NewReader(src)
	line1, err := reader.ReadString('\n')
	if err != nil || line1 != "line1\n" {
		fail("bufio.Reader.ReadString first")
	}
	line2, err := reader.ReadString('\n')
	if line2 != "line2" || err == nil {
		fail("bufio.Reader.ReadString second")
	}
	var out bytes.Buffer
	writer := bufio.NewWriter(&out)
	_, _ = writer.WriteString("ok")
	_ = writer.WriteByte('!')
	if writer.Flush() != nil || out.String() != "ok!" {
		fail("bufio.Writer")
	}

	// flag
	fs := flag.NewFlagSet("demo")
	name := fs.String("name", "default", "")
	count := fs.Int("count", 1, "")
	verbose := fs.Bool("v", false, "")
	_ = verbose
	err = fs.Parse([]string{"-name=rtg", "-count", "7", "-v", "tail"})
	if err != nil {
		fail("flag.Parse")
	}
	if *name != "rtg" || *count != 7 {
		fail("flag values")
	}
	if !fs.Parsed() || fs.NArg() != 1 || fs.Arg(0) != "tail" {
		fail("flag args")
	}
	setErr := fs.Set("count", "9")
	if setErr != nil {
		fail("flag.Set")
	}
	if *count != 9 {
		fail("flag.Set")
	}

	// log
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "pref:", 0)
	logger.Print("x")
	logger.Printf("n=%d", 3)
	logger.Println("done")
	if !bytes.Contains(logBuf.Bytes(), []byte("pref:x")) {
		fail("log.Print")
	}
	if !bytes.Contains(logBuf.Bytes(), []byte("pref:n=3")) {
		fail("log.Printf")
	}
	if !bytes.Contains(logBuf.Bytes(), []byte("pref:done")) {
		fail("log.Println")
	}
	fmt.Print("PASS")
}
