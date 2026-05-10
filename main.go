package main

import (
	"flag"
	"fmt"
	"os"

	"qssh/cmd"
)

const version = "0.1.0"

func main() {
	var (
		addName  string
		editName string
		delName  string
		doList   bool
		showVer  bool
	)

	flag.StringVar(&addName, "add", "", "Create a new profile")
	flag.StringVar(&editName, "edit", "", "Edit an existing profile")
	flag.StringVar(&delName, "delete", "", "Delete a profile")
	flag.BoolVar(&doList, "list", false, "List profiles (optional: qssh --list filter)")
	flag.BoolVar(&showVer, "version", false, "Print version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `QSSH - SSH Credential Manager v%s

Usage:
  qssh <profile>              Connect to a profile
  qssh --add <name>           Create a new profile
  qssh --edit <name>          Edit an existing profile
  qssh --list [filter]        List profiles (optional substring filter)
  qssh --delete <name>        Delete a profile
  qssh --version              Print version
`, version)
	}
	flag.Parse()

	switch {
	case showVer:
		fmt.Printf("qssh v%s\n", version)
		return
	case addName != "":
		cmd.Add(addName)
	case editName != "":
		cmd.Edit(editName)
	case delName != "":
		cmd.Delete(delName)
	case doList:
		filter := ""
		if flag.NArg() > 0 {
			filter = flag.Arg(0)
		}
		cmd.List(filter)
	case flag.NArg() == 1:
		connectCmd(flag.Arg(0))
	default:
		flag.Usage()
		os.Exit(1)
	}
}

func connectCmd(name string) { cmd.Connect(name) }