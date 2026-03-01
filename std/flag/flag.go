package flag

import (
	"fmt"
	"os"
	"strconv"
)

type flagKind int

const (
	kindBool flagKind = iota
	kindInt
	kindString
)

type Flag struct {
	Name     string
	Usage    string
	DefValue string

	kind flagKind
	bptr *bool
	iptr *int
	sptr *string
}

type FlagSet struct {
	name   string
	flags  []*Flag
	args   []string
	parsed bool
}

func NewFlagSet(name string) *FlagSet {
	return &FlagSet{name: name}
}

func (f *FlagSet) lookup(name string) *Flag {
	i := 0
	for i < len(f.flags) {
		if f.flags[i].Name == name {
			return f.flags[i]
		}
		i++
	}
	return nil
}

func (f *FlagSet) addFlag(fl *Flag) {
	if f.lookup(fl.Name) != nil {
		panic("flag redefined: " + fl.Name)
	}
	f.flags = append(f.flags, fl)
}

func (f *FlagSet) Bool(name string, value bool, usage string) *bool {
	p := new(bool)
	*p = value
	defValue := "false"
	if value {
		defValue = "true"
	}
	f.addFlag(&Flag{
		Name:     name,
		Usage:    usage,
		DefValue: defValue,
		kind:     kindBool,
		bptr:     p,
	})
	return p
}

func (f *FlagSet) Int(name string, value int, usage string) *int {
	p := new(int)
	*p = value
	f.addFlag(&Flag{
		Name:     name,
		Usage:    usage,
		DefValue: strconv.Itoa(value),
		kind:     kindInt,
		iptr:     p,
	})
	return p
}

func (f *FlagSet) String(name string, value string, usage string) *string {
	p := new(string)
	*p = value
	f.addFlag(&Flag{
		Name:     name,
		Usage:    usage,
		DefValue: value,
		kind:     kindString,
		sptr:     p,
	})
	return p
}

func parseBool(s string) (bool, bool) {
	if s == "1" || s == "t" || s == "T" || s == "true" || s == "TRUE" || s == "True" {
		return true, true
	}
	if s == "0" || s == "f" || s == "F" || s == "false" || s == "FALSE" || s == "False" {
		return false, true
	}
	return false, false
}

func splitArg(raw string) (string, string, bool) {
	i := 0
	for i < len(raw) {
		if raw[i] == '=' {
			return raw[0:i], raw[i+1:], true
		}
		i++
	}
	return raw, "", false
}

func (f *FlagSet) Set(name string, value string) error {
	fl := f.lookup(name)
	if fl == nil {
		return fmt.Errorf("flag provided but not defined: -%s", name)
	}
	switch fl.kind {
	case kindBool:
		v, ok := parseBool(value)
		if !ok {
			return fmt.Errorf("invalid boolean value %q for -%s", value, name)
		}
		*fl.bptr = v
	case kindInt:
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q for -%s", value, name)
		}
		*fl.iptr = v
	case kindString:
		*fl.sptr = value
	default:
		return fmt.Errorf("unsupported flag type for -%s", name)
	}
	return nil
}

func (f *FlagSet) Parse(args []string) error {
	f.args = nil
	i := 0
	for i < len(args) {
		arg := args[i]
		if len(arg) == 0 || arg == "-" || arg[0] != '-' {
			f.args = append(f.args, args[i:]...)
			f.parsed = true
			return nil
		}
		if arg == "--" {
			if i+1 < len(args) {
				f.args = append(f.args, args[i+1:]...)
			}
			f.parsed = true
			return nil
		}
		nameValue := arg[1:]
		if len(nameValue) > 0 && nameValue[0] == '-' {
			nameValue = nameValue[1:]
		}
		name, inlineValue, hasInline := splitArg(nameValue)
		fl := f.lookup(name)
		if fl == nil {
			return fmt.Errorf("flag provided but not defined: -%s", name)
		}
		switch fl.kind {
		case kindBool:
			if hasInline {
				if err := f.Set(name, inlineValue); err != nil {
					return err
				}
			} else {
				*fl.bptr = true
			}
		case kindInt, kindString:
			value := inlineValue
			if !hasInline {
				i++
				if i >= len(args) {
					return fmt.Errorf("flag needs an argument: -%s", name)
				}
				value = args[i]
			}
			if err := f.Set(name, value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported flag type for -%s", name)
		}
		i++
	}
	f.parsed = true
	return nil
}

func (f *FlagSet) Parsed() bool {
	return f.parsed
}

func (f *FlagSet) Args() []string {
	return f.args
}

func (f *FlagSet) NArg() int {
	return len(f.args)
}

func (f *FlagSet) Arg(i int) string {
	if i < 0 || i >= len(f.args) {
		return ""
	}
	return f.args[i]
}

var CommandLine *FlagSet = &FlagSet{name: ""}

func init() {
	if len(os.Args) > 0 {
		CommandLine.name = os.Args[0]
	}
}

func Bool(name string, value bool, usage string) *bool {
	return CommandLine.Bool(name, value, usage)
}

func Int(name string, value int, usage string) *int {
	return CommandLine.Int(name, value, usage)
}

func String(name string, value string, usage string) *string {
	return CommandLine.String(name, value, usage)
}

func Set(name string, value string) error {
	return CommandLine.Set(name, value)
}

func Parse() {
	err := CommandLine.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
}

func Parsed() bool {
	return CommandLine.Parsed()
}

func Args() []string {
	return CommandLine.Args()
}

func NArg() int {
	return CommandLine.NArg()
}

func Arg(i int) string {
	return CommandLine.Arg(i)
}
