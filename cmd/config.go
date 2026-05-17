package cmd

import (
	"fmt"
	"os"
	"sort"

	"qssh/internal"
	"qssh/internal/i18n"
)

// Config handles --config get/set operations.
func Config(args []string) {
	if len(args) == 0 {
		listConfig()
		return
	}

	switch args[0] {
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, i18n.T("config.usage.get"))
			os.Exit(1)
		}
		getConfig(args[1])
	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, i18n.T("config.usage.set"))
			os.Exit(1)
		}
		setConfig(args[1], args[2])
	case "unset":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, i18n.T("config.usage.unset"))
			os.Exit(1)
		}
		unsetConfig(args[1])
	default:
		fmt.Fprintf(os.Stderr, i18n.T("config.unknown_action")+"\n", args[0])
		os.Exit(1)
	}
}

func listConfig() {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	all := c.All()
	if len(all) == 0 {
		fmt.Println(i18n.T("config.empty"))
		return
	}
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s = %s\n", k, all[k])
	}
}

func getConfig(key string) {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	val := c.Get(key)
	if val == "" {
		fmt.Println(i18n.T("config.not_set"))
	} else {
		fmt.Println(val)
	}
}

func setConfig(key, value string) {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	if err := c.Set(key, value); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("config.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s = %s\n", key, value)
}

func unsetConfig(key string) {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	if err := c.Set(key, ""); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("config.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf(i18n.T("config.unset")+"\n", key)
}