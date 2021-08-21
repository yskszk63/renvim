package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"
	"github.com/neovim/go-client/nvim"
)

func main() {
	val, present := os.LookupEnv("NVIM_LISTEN_ADDRESS")

	args := os.Args[1:]
	optonly := func() bool {
		for _, a := range args {
			if !strings.HasPrefix(a, "--") {
				return false
			}
		}
		return true
	}()

	if !present || val == "" || (len(args) > 0 && optonly) {
		bin, err := exec.LookPath("nvim")
		if err != nil {
			fmt.Fprintf(os.Stderr, "no nvim found: %s\n", err)
			os.Exit(-1)
		}
		args := make([]string, len(os.Args))
		copy(args, os.Args)
		args[0] = bin

		env := os.Environ()

		for _, a := range args[1:] {
			if a == "--version" {
				fmt.Printf("renvim v0.0.0 -- Neovim wrapper.\n\n") // FIXME version
			}
		}

		if err := syscall.Exec(bin, args, env); err != nil {
			fmt.Fprintf(os.Stderr, "failed to exec nvim: %s\n", err)
		}
	}

	client, err := nvim.Dial(val)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(-1)
	}
	defer client.Close()

	if len(args) == 0 {
		args = []string{"-"}
	}

	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			fmt.Fprintf(os.Stderr, "Ignore option %s\n", a)
		}

		if a == "-" {
			if err := client.Command("tabnew"); err != nil {
				fmt.Fprintf(os.Stderr, "failed to open file: %s\n", err)
				os.Exit(-1)
			}

			if !isatty.IsTerminal(os.Stdin.Fd()) {
				buf, err := client.CurrentBuffer()
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to get current buffer: %s\n", err)
					os.Exit(-1)
				}

				sc := bufio.NewScanner(os.Stdin)
				for sc.Scan() {
					if err := client.SetBufferLines(buf, -2, -2, false, [][]byte{[]byte(sc.Text())}); err != nil {
						fmt.Fprintf(os.Stderr, "failed to get set buffer lines: %s\n", err)
						os.Exit(-1)
					}
				}
				if err := sc.Err(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to get current buffer: %s\n", err)
					os.Exit(-1)
				}
			}

			if err := client.Command("silent 1delete _"); err != nil {
				fmt.Fprintf(os.Stderr, "failed to open file: %s\n", err)
				os.Exit(-1)
			}

		} else {
			p, err := filepath.Abs(a)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to resolve file %s: %s\n", p, err)
				p = a
			}
			command := fmt.Sprintf("tabnew %s", p)
			if err := client.Command(command); err != nil {
				fmt.Fprintf(os.Stderr, "failed to open file %s: %s\n", p, err)
			}
		}
	}
}
