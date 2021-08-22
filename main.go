package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/mattn/go-isatty"
	"github.com/neovim/go-client/nvim"
)

func optonly(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			return false
		}
	}
	return true
}

func exec_as_nvim(args []string) error {
	bin, err := exec.LookPath("nvim")
	if err != nil {
		return fmt.Errorf("no nvim found: %s\n", err)
	}
	newargs := make([]string, len(args)+1)
	copy(newargs[1:], args)
	newargs[0] = bin

	env := os.Environ()

	for _, a := range args {
		if a == "--version" {
			fmt.Printf("renvim v0.0.0 -- Neovim wrapper.\n\n") // FIXME version
		}
	}

	if err := syscall.Exec(bin, newargs, env); err != nil {
		return fmt.Errorf("failed to exec nvim: %s\n", err)
	}
	return nil
}

func prepare_env(client *nvim.Nvim, opened map[int]bool, wg *sync.WaitGroup) error {
	if err := client.RegisterHandler("renvimExit", func(buf int) {
		//fmt.Printf("OK %d %v\n", buf, opened)

		closed, found := opened[buf]
		if !closed && found {
			opened[buf] = true
			wg.Done()
		}
	}); err != nil {
		return fmt.Errorf("failed to register handler: %s", err)
	}
	if err := client.Subscribe("renvimExit"); err != nil {
		return fmt.Errorf("failed to register handler: %s", err)
	}

	c := fmt.Sprintf(`augroup renvim
autocmd! BufWinLeave * silent! call rpcnotify(0, "renvimExit", str2nr(expand("<abuf>")))
augroup END`)
	if _, err := client.Exec(c, false); err != nil {
		return fmt.Errorf("failed to regeister autocmd: %s", err)
	}

	return nil
}

func tabnew(client *nvim.Nvim, stdin *os.File) (*nvim.Buffer, error) {
	tty := isatty.IsTerminal(stdin.Fd())

	if !tty {
		// may be not work on mac.
		proc, err := filepath.EvalSymlinks("/proc/self")
		if err == nil {
			fd := filepath.Join(proc, "fd", "0")
			buf, err := tabnew_with_file(client, fd)
			if err != nil {
				return nil, err
			}

			if err := client.Command("silent! 0file"); err != nil {
				return nil, err
			}

			return buf, nil
		}
	}

	if err := client.Command("tabnew"); err != nil {
		return nil, fmt.Errorf("failed to open file: %s", err)
	}

	buf, err := client.CurrentBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to get current buffer: %s", err)
	}

	if !tty {
		sc := bufio.NewScanner(stdin)
		for sc.Scan() {
			if err := client.SetBufferLines(buf, -2, -2, false, [][]byte{[]byte(sc.Text())}); err != nil {
				return nil, fmt.Errorf("failed to get set buffer lines: %s", err)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("failed to get current buffer: %s", err)
		}
	}

	return &buf, nil
}

func tabnew_with_file(client *nvim.Nvim, file string) (*nvim.Buffer, error) {
	p, err := filepath.Abs(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve file %s: %s\n", p, err)
		p = file
	}
	command := fmt.Sprintf("tabnew %s", p)
	if err := client.Command(command); err != nil {
		return nil, fmt.Errorf("failed to open file %s: %s", p, err)
	}

	buf, err := client.CurrentBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to get current buffer: %s", err)
	}

	return &buf, nil
}

func main() {
	val, present := os.LookupEnv("NVIM_LISTEN_ADDRESS")
	args := os.Args[1:]

	if !present || val == "" || (len(args) > 0 && optonly(args)) {
		if err := exec_as_nvim(args); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(-1)
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

	var wg sync.WaitGroup
	opened := make(map[int]bool)
	if err := prepare_env(client, opened, &wg); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(-1)
	}

	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			fmt.Fprintf(os.Stderr, "Ignore option %s\n", a)
			continue
		}

		if a == "-" {
			buf, err := tabnew(client, os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				os.Exit(-1)
			}
			opened[int(*buf)] = false
			wg.Add(1)

		} else {
			buf, err := tabnew_with_file(client, a)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				os.Exit(-1)
			}
			opened[int(*buf)] = false
			wg.Add(1)

		}
	}

	wg.Wait()
}
