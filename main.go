package main

import (
	"flag"
	"fmt"
	"os"

	"qssh/cmd"
)

const version = "0.2.1"

func main() {
	var (
		addName       string
		editName      string
		delName       string
		mountName     string
		umountName    string
		daemonName    string
		daemonPort    string
		doConfig      bool
		doList        bool
		showVer       bool
	)

	flag.StringVar(&addName, "add", "", "Create a new profile")
	flag.StringVar(&editName, "edit", "", "Edit an existing profile")
	flag.StringVar(&delName, "delete", "", "Delete a profile")
	flag.StringVar(&mountName, "mount", "", "Mount a profile via WebDAV (usage: qssh --mount <name>)")
	flag.StringVar(&umountName, "umount", "", "Unmount a profile (usage: qssh --umount <name>)")
	flag.StringVar(&daemonName, "mount-daemon", "", "Internal: mount worker (profile name)")
	flag.StringVar(&daemonPort, "port", "", "Internal: mount worker port")
	flag.BoolVar(&doConfig, "config", false, "View or modify config (usage: qssh --config [get|set <key> <value>])")
	flag.BoolVar(&doList, "list", false, "List profiles (optional: qssh --list filter)")
	flag.BoolVar(&showVer, "version", false, "Print version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `QSSH - SSH Credential Manager v%s

Usage:
  qssh <profile>                    Connect to a profile
  qssh --add <name>                 Create a new profile
  qssh --edit <name>                Edit an existing profile
  qssh --list [filter]              List profiles (optional substring filter)
  qssh --delete <name>              Delete a profile
  qssh --mount <name>               Mount profile via WebDAV (background)
  qssh --umount <name>              Unmount a profile
  qssh --config [get|set ...]       View or modify config
  qssh --version                    Print version
`, version)
	}
	flag.Parse()

	switch {
	case showVer:
		fmt.Printf("qssh v%s\n", version)
		return
	case daemonName != "":
		cmd.MountDaemon(daemonName, daemonPort)
	case doConfig:
		cmd.Config(flag.Args())
	case addName != "":
		cmd.Add(addName)
	case editName != "":
		cmd.Edit(editName)
	case delName != "":
		cmd.Delete(delName)
	case mountName != "":
		mountPoint := ""
		if flag.NArg() > 0 {
			mountPoint = flag.Arg(0)
		}
		cmd.Mount(mountName, mountPoint)
	case umountName != "":
		cmd.Unmount(umountName)
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