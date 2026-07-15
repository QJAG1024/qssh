package cmd

import (
	"fmt"
	"os"
	"strings"

	"qssh/internal/i18n"
	"qssh/internal/privacy"
)

// Privacy handles --privacy [on|off|clear|status].
// With no args, prints status. on/off writes sticky runtime state (until reboot).
func Privacy(args []string) {
	if len(args) == 0 || strings.EqualFold(args[0], "status") {
		printPrivacyStatus()
		return
	}
	mode := strings.ToLower(strings.TrimSpace(args[0]))
	switch mode {
	case "on", "off":
		if err := privacy.SetSticky(mode); err != nil {
			fmt.Fprintf(os.Stderr, "privacy: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(i18n.T("privacy.set", mode))
	case "clear", "default", "reset":
		if err := privacy.SetSticky("clear"); err != nil {
			fmt.Fprintf(os.Stderr, "privacy: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(i18n.T("privacy.cleared"))
	default:
		fmt.Fprintln(os.Stderr, i18n.T("privacy.usage"))
		os.Exit(1)
	}
	printPrivacyStatus()
}

func printPrivacyStatus() {
	en, src := privacy.Status()
	state := "off"
	if en {
		state = "on"
	}
	fmt.Println(i18n.T("privacy.status", state, src))
}
