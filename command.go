package ishell

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"regexp"
	"text/tabwriter"
)

type ArgType int

const (
	IntType ArgType = 0
	StringType  ArgType = 1
	BoolType    ArgType = 2
)

type CmdArg struct {
	// short flag, such as 'p'
	flag string
	// long flag, i.e. accessed by 'process'
	// acts as the key in the args map in a cmd
	longFlag string
	// the type of the argument
	typ ArgType
	// whether there can be multiple of these arguments
	canHaveMultiple bool
	// whether this is required
	required bool
}

type ParsedArg struct {
	Key   string
	Typ   ArgType
	Value string
}

// Cmd is a shell command handler.
type Cmd struct {
	// Command name.
	Name string
	// Command name aliases.
	Aliases []string
	// Function to execute for the command.
	Func func(c *Context)
	// One liner help message for the command.
	Help string
	// More descriptive help message for the command.
	LongHelp string

	// Completer is custom autocomplete for command.
	// It takes in command arguments and returns
	// autocomplete options.
	// By default all commands get autocomplete of
	// subcommands.
	// A non-nil Completer overrides the default behaviour.
	Completer func(args []string) []string

	// CompleterWithPrefix is custom autocomplete like
	// for Completer, but also provides the prefix
	// already so far to the completion function
	// If both Completer and CompleterWithPrefix are given,
	// CompleterWithPrefix takes precedence
	CompleterWithPrefix func(prefix string, args []string) []string

	// subcommands.
	children map[string]*Cmd

	// args
	args map[string]*CmdArg
}

func NewCmdArg(flag string, longFlag string, typ ArgType, 
	canHaveMultiple bool, required bool) (*CmdArg, error) {
	var ret *CmdArg
	if !(len(flag) == 1 && regexp.MustCompile(`^[a-zA-Z0-9]$`).MatchString(flag)) {
		return ret, fmt.Errorf("Flag '%s' is not a valid parameter", flag)
	}

	if !(len(longFlag) > 1 && regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]+$`).MatchString(longFlag)) {
		return ret, fmt.Errorf("LongFlag '%s' is not a valid parameter", longFlag)
	}

	// not a valid ArgType
	if typ < 0 || typ > 2 {
		return ret, fmt.Errorf("Typ '%d' is not a valid parameter. Please use values IntType, StringType, or BoolType", typ)
	}

	ret = &CmdArg{
		flag: flag,
		longFlag: longFlag,
		typ: typ,
		canHaveMultiple: canHaveMultiple,
		required: required,
	}

	return ret, nil
}

// AddCmd adds cmd as a subcommand.
func (c *Cmd) AddCmd(cmd *Cmd) {
	if c.children == nil {
		c.children = make(map[string]*Cmd)
	}
	c.children[cmd.Name] = cmd
}

// DeleteCmd deletes cmd from subcommands.
func (c *Cmd) DeleteCmd(name string) {
	delete(c.children, name)
}

func (c *Cmd) AddCmdArg(arg *CmdArg) {
	if c.args == nil {
		c.args = make(map[string]*CmdArg)
	}
	c.args[arg.longFlag] = arg
}

// Children returns the subcommands of c.
func (c *Cmd) Children() []*Cmd {
	var cmds []*Cmd
	for _, cmd := range c.children {
		cmds = append(cmds, cmd)
	}
	sort.Sort(cmdSorter(cmds))
	return cmds
}

func (c *Cmd) hasSubcommand() bool {
	if len(c.children) > 1 {
		return true
	}
	if _, ok := c.children["help"]; !ok {
		return len(c.children) > 0
	}
	return false
}

// HelpText returns the computed help of the command and its subcommands.
func (c Cmd) HelpText() string {
	var b bytes.Buffer
	p := func(s ...interface{}) {
		fmt.Fprintln(&b)
		if len(s) > 0 {
			fmt.Fprintln(&b, s...)
		}
	}
	if c.LongHelp != "" {
		p(c.LongHelp)
	} else if c.Help != "" {
		p(c.Help)
	} else if c.Name != "" {
		p(c.Name, "has no help")
	}
	if c.hasSubcommand() {
		p("Commands:")
		w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
		for _, child := range c.Children() {
			fmt.Fprintf(w, "\t%s\t\t\t%s\n", child.Name, child.Help)
		}
		w.Flush()
		p()
	}
	return b.String()
}

// findChildCmd returns the subcommand with matching name or alias.
func (c *Cmd) findChildCmd(name string) *Cmd {
	// find perfect matches first
	if cmd, ok := c.children[name]; ok {
		return cmd
	}

	// find alias matching the name
	for _, cmd := range c.children {
		for _, alias := range cmd.Aliases {
			if alias == name {
				return cmd
			}
		}
	}

	return nil
}

// FindCmd finds the matching Cmd for args.
// It returns the Cmd and the remaining args.
func (c Cmd) FindCmd(args []string) (*Cmd, []string) {
	var cmd *Cmd
	for i, arg := range args {
		if cmd1 := c.findChildCmd(arg); cmd1 != nil {
			cmd = cmd1
			c = *cmd
			continue
		}
		return cmd, args[i:]
	}
	return cmd, nil
}

// Check to see if the string is a long argument param
func is_long_arg(str string) bool {
	return len(str) > 2 && str[:2] == "--"
}

// Check to see if the string is a short argument param
// didn't check to see if the second char is "-"
func is_short_arg(str string) bool {
	return len(str) > 1 && str[:1] == "-"
}

/*
Returns the key if arg matches either a Flag or LongFlag in the command's 'args'
parameter.
*/
func (c Cmd) find_arg(arg string) string {
	key := ""
	is_long := is_long_arg(arg)
	if !(is_long != is_short_arg(arg)) {
		return key
	}

	for i, argument := range c.args {
		cleaned := strings.Replace(arg, "-", "", -1)
		if is_long && argument.longFlag == cleaned {
			return i
		} else if argument.flag == cleaned {
			return i
		}
	}

	return key
}

func (c Cmd) find_positional(arg string) string {
	key := ""
	for i, argument := range c.args {
		// is positional
		if argument.flag == "" && argument.longFlag == "" {
			return i
		}
	}
	return key
}

// Do an initial pass to split up arguments that can be put together
func (c Cmd) initial_pass(args []string) []string {
	ret := make([]string, 0)

	for _, arg := range args {
		if is_short_arg(arg) && !is_long_arg(arg) && len(arg) > 2 {
			without_dash := arg[1:]
			for _, char := range without_dash {
				ret = append(ret, "-"+string(char))
			}
		} else {
			ret = append(ret, arg)
		}
	}
	return ret
}

// checks to see if an integer argument is a valid integer
func validate_int(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

// validates the arguments to make sure there are no repeats that aren't allowed, or if every
// required argument exists
func (c Cmd) validate_args(parsed []ParsedArg) error {
	count := make(map[string]int)

	// make a count dict, but also check to make sure
	// every arg that needs a value has one
	for _, arg := range parsed {
		if (arg.Typ == IntType || arg.Typ == StringType) &&
			arg.Value == "" {
				fmt.Println(arg)
			return fmt.Errorf("%s is missing an argument", arg.Key)
		}
		val, exists := count[arg.Key]
		if !exists {
			count[arg.Key] = 1
		} else {
			count[arg.Key] = val + 1
		}
	}

	// iterate through every argument given with the command and check the count
	// that each arg has in the counter. validate that the required commands exist,
	// and that there aren't any arguments that shouldnt have multiples.
	for key, arg := range c.args {
		val, exists := count[key]
		if arg.required && !exists {
			return fmt.Errorf("%s is a required argument", key)
		}
		if !arg.canHaveMultiple && val > 1 {
			return fmt.Errorf("There cannot be multiple instances of %s", key)
		}
	}
	return nil
}

// Parses args, returns keys to the values
func (c Cmd) ParseArgs(args []string) ([]ParsedArg, error) {
	ret := make([]ParsedArg, 0)
	further_split := c.initial_pass(args)

	var temp_arg ParsedArg
	// once an arg is found, set awaiting_value to true
	awaiting_value := false
	for _, arg := range further_split {
		key := c.find_arg(arg)

		// found a matching arg!
		if key != "" {
			temp_arg = ParsedArg{
				Key: key,
				Typ: c.args[key].typ,
			}
			if c.args[key].typ != BoolType {
				awaiting_value = true
			} else {
				ret = append(ret, temp_arg)
			}
			continue
		}

		// didn't find the arg, if awaiting_value is true then this value is parsed_arg.
		if key == "" && awaiting_value {
			if temp_arg.Typ == IntType && !validate_int(arg) {
				return ret, fmt.Errorf("String %s is not a valid integer for argument '%s'", arg, temp_arg.Key)
			}
			temp_arg.Value = arg
			ret = append(ret, temp_arg)
			awaiting_value = false
			continue
		}

		// awaiting_value == false, so look for positional argument
		key = c.find_positional(arg)

		// there's a positional argument that can fit this value!
		if key != "" {
			temp_arg = ParsedArg{
				Key:   key,
				Typ:   c.args[key].typ,
				Value: arg,
			}
			if temp_arg.Typ == IntType && !validate_int(arg) {
				return ret, fmt.Errorf("String %s is not a valid integer for argument '%s'", arg, temp_arg.Key)
			}
			ret = append(ret, temp_arg)
		} else {
			return ret, fmt.Errorf("Invalid argument %s", arg)
		}
	}

	if awaiting_value {
		return ret, fmt.Errorf("There is a parameter missing a value")
	}

	err := c.validate_args(ret)

	return ret, err
}

type cmdSorter []*Cmd

func (c cmdSorter) Len() int           { return len(c) }
func (c cmdSorter) Less(i, j int) bool { return c[i].Name < c[j].Name }
func (c cmdSorter) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
