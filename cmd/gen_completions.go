//go:build ignore

// gen_completions generates bash/zsh/fish completion files from the flag
// registrations in main.go. Run via: go run gen_completions.go
// Outputs to ../completions/ (kept in the Makefile's completions target).
//
// The generator parses main.go's flag.StringVar/IntVar/BoolVar/Var calls to
// extract every flag name and its help string, so completions can never drift
// from the actual CLI.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type flagInfo struct {
	name string
	desc string
	arg  bool // takes a value
}

// Profile-taking commands (completions suggest profile names after these).
var profileFlags = map[string]bool{
	"add": true, "edit": true, "delete": true, "copy": true, "rename": true,
	"exec": true, "sftp-start": true, "sftp-stop": true, "daemon-start": true,
	"daemon-stop": true, "history": true, "proxy": true, "export": true,
	"webdav-start": true, "webdav-stop": true,
}

var flagRe = regexp.MustCompile(`flag\.(String|Int|Bool|Var)Var\([^,]+,\s*"([a-z0-9-]+)"\s*,\s*([^,]*),\s*"((?:[^"\\]|\\.)*)"`)

func main() {
	src, err := os.ReadFile("main.go")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read main.go:", err)
		os.Exit(1)
	}

	var flags []flagInfo
	for _, m := range flagRe.FindAllStringSubmatch(string(src), -1) {
		typ, name, val, desc := m[1], m[2], strings.TrimSpace(m[3]), m[4]
		takesArg := typ != "Bool" && !(typ == "Var" && val == "false")
		flags = append(flags, flagInfo{name: name, desc: desc, arg: takesArg})
	}
	// Deduplicate (--yes registered twice: --yes and -y) and drop internal
	// flags (help text starts with "Internal:").
	seen := map[string]bool{}
	var uniq []flagInfo
	for _, f := range flags {
		if strings.HasPrefix(f.desc, "Internal") {
			continue
		}
		if !seen[f.name] {
			seen[f.name] = true
			uniq = append(uniq, f)
		}
	}
	flags = uniq
	sort.Slice(flags, func(i, j int) bool { return flags[i].name < flags[j].name })

	os.MkdirAll("completions", 0755)
	writeBash(flags)
	writeZsh(flags)
	writeFish(flags)
	fmt.Printf("generated %d flags -> completions/_qssh, qssh.bash, qssh.fish\n", len(flags))
}

func writeBash(flags []flagInfo) {
	var all []string
	for _, f := range flags {
		if f.name == "y" {
			all = append(all, "-y")
			continue
		}
		all = append(all, "--"+f.name)
	}
	flagList := strings.Join(all, " ")

	var b strings.Builder
	b.WriteString("# bash completion for qssh\n_qssh() {\n  local cur prev words cword\n  _init_completion || return\n\n  local profiles\n")
	b.WriteString(`  profiles=$(qssh --list --json 2>/dev/null | python3 -c 'import json,sys
try:
  data=json.load(sys.stdin)
  print(" ".join(p.get("name","") for p in data))
except Exception:
  pass' 2>/dev/null)
  if [[ -z $profiles ]]; then
    profiles=$(qssh --list 2>/dev/null | tail -n +3 | awk 'NF{print $1}' | tr '\n' ' ')
  fi

  case $prev in
`)
	// Merge all profile-taking flags into one case pattern with | separators.
	names := make([]string, 0, len(profileFlags))
	for name := range profileFlags {
		names = append(names, name)
	}
	sort.Strings(names)
	patterns := make([]string, 0, len(names))
	for _, n := range names {
		patterns = append(patterns, "--"+n)
	}
	b.WriteString("    " + strings.Join(patterns, "|") + ")\n")
	b.WriteString(`      COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
      return ;;
  esac

  if [[ $cur == -* ]]; then
    COMPREPLY=($(compgen -W '`)
	b.WriteString(flagList)
	b.WriteString(`' -- "$cur"))
  else
    COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
  fi
}
complete -F _qssh qssh
`)
	os.WriteFile(filepath.Join("completions", "qssh.bash"), []byte(b.String()), 0644)
}

func writeZsh(flags []flagInfo) {
	var b strings.Builder
	b.WriteString("#compdef qssh\n_qssh() {\n  local -a profiles\n")
	b.WriteString(`  profiles=(${(f)"$(_call_program profiles qssh --list 2>/dev/null | tail -n +3 | awk "{print \$1}")"})`)
	b.WriteString("\n  _arguments \\\n")
	for _, f := range flags {
		if f.name == "y" {
			continue
		}
		spec := fmt.Sprintf("    \"--%s[%s]", f.name, f.desc)
		if profileFlags[f.name] {
			spec += ":name:(${profiles[@]})"
		} else if f.arg {
			spec += ":value:"
		}
		spec += "\" \\\n"
		b.WriteString(spec)
	}
	b.WriteString("    \"*: :->profile\"\n")
	b.WriteString(`  case $state in
    profile)
      _describe profiles profiles
      ;;
  esac
}
_qssh "$@"
`)
	os.WriteFile(filepath.Join("completions", "_qssh"), []byte(b.String()), 0644)
}

func writeFish(flags []flagInfo) {
	var b strings.Builder
	b.WriteString("# fish completion for qssh\n")
	b.WriteString("function __qssh_profiles\n")
	b.WriteString(`    if command qssh --list --json >/dev/null 2>&1
        command qssh --list --json 2>/dev/null | command python3 -c 'import json,sys
try:
  data=json.load(sys.stdin)
  for p in data: print(p.get("name",""))
except Exception:
  pass' 2>/dev/null
    else
        command qssh --list 2>/dev/null | tail -n +3 | awk 'NF{print $1}'
    end
end

complete -c qssh -f

`)
	for _, f := range flags {
		var line string
		if f.name == "y" {
			line = "complete -c qssh -s y -d \"" + f.desc + "\""
		} else {
			line = fmt.Sprintf("complete -c qssh -l %s -d %q", f.name, f.desc)
			if profileFlags[f.name] {
				line += " -r -a \"(__qssh_profiles)\""
			} else if f.arg {
				line += " -r"
			}
		}
		b.WriteString(line + "\n")
	}
	os.WriteFile(filepath.Join("completions", "qssh.fish"), []byte(b.String()), 0644)
}
