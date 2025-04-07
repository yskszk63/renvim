package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/mattn/go-isatty"
	"github.com/neovim/go-client/nvim"
)

type nvimClient interface {
	RegisterHandler(method string, fn any) error
	Subscribe(event string) error
	Unsubscribe(event string) error
	Command(command string) error
	CurrentBuffer() (nvim.Buffer, error)
	SetBufferLines(nvim.Buffer, int, int, bool, [][]byte) error
	ChannelID() int
	ExecLua(code string, result any, args ...any) error
}

func optonly(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			return false
		}
	}
	return true
}

func execAsNvim(args []string, fnexec func(string, []string, []string) error) error {
	bin, err := exec.LookPath("nvim")
	if err != nil {
		return fmt.Errorf("no nvim found: %w", err)
	}
	newargs := make([]string, len(args)+1)
	copy(newargs[1:], args)
	newargs[0] = bin

	env := os.Environ()

	if err := fnexec(bin, newargs, env); err != nil {
		return fmt.Errorf("failed to exec nvim: %w", err)
	}
	return nil
}

func prepareEnv(cx context.Context, wg *sync.WaitGroup, client nvimClient, bufwg *sync.WaitGroup) error {
	fn := func() {
		bufwg.Done()
	}
	if err := client.RegisterHandler("renvimExit", fn); err != nil {
		return fmt.Errorf("failed to register handler: %w", err)
	}
	if err := client.Subscribe("renvimExit"); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	wg.Add(1)
	context.AfterFunc(cx, func() {
		defer wg.Done()

		if err := client.Unsubscribe("renvimExit"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to unsubscribe: %s\n", err)
		}
	})

	return nil
}

func registerCloseNotify(cx context.Context, wg *sync.WaitGroup, client nvimClient, bufwg *sync.WaitGroup) error {
	cid := client.ChannelID()

	buf, err := client.CurrentBuffer()
	if err != nil {
		return fmt.Errorf("failed to get current buffer: %w", err)
	}

	// FIXME notify bufnr
	code := fmt.Sprintf(`return vim.api.nvim_create_autocmd('BufWinLeave', {
  buffer = %d,
  callback = function(e)
    vim.rpcnotify(%d, 'renvimExit')
  end,
})`, buf, cid)

	var id int
	var args struct{}

	if err := client.ExecLua(code, &id, &args); err != nil {
		return err
	}
	bufwg.Add(1)

	wg.Add(1)
	context.AfterFunc(cx, func() {
		defer wg.Done()

		code := fmt.Sprintf(`return vim.api.nvim_del_autocmd(%d)`, id)

		var result any
		var args struct{}

		if err := client.ExecLua(code, &result, &args); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	})

	return nil
}

func tabnew(cx context.Context, wg *sync.WaitGroup, client nvimClient, stdin *os.File, bufwg *sync.WaitGroup) error {
	tty := isatty.IsTerminal(stdin.Fd())

	if !tty {
		// may be not work on mac.
		proc, err := filepath.EvalSymlinks("/proc/self")
		if err == nil {
			fd := filepath.Join(proc, "fd", fmt.Sprint(stdin.Fd()))
			if err := tabnewWithFile(cx, wg, client, fd, bufwg); err != nil {
				return err
			}

			if err := client.Command("silent! 0file"); err != nil {
				return err
			}

			return nil
		}
	}

	if err := client.Command("tabnew"); err != nil {
		return fmt.Errorf("failed to open file: %s", err)
	}

	if err := registerCloseNotify(cx, wg, client, bufwg); err != nil {
		return err
	}

	if !tty {
		buf, err := client.CurrentBuffer()
		if err != nil {
			return fmt.Errorf("failed to get current buffer: %s", err)
		}

		sc := bufio.NewScanner(stdin)
		for sc.Scan() {
			if err := client.SetBufferLines(buf, -2, -2, false, [][]byte{sc.Bytes()}); err != nil {
				return fmt.Errorf("failed to get set buffer lines: %s", err)
			}
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("failed to get current buffer: %s", err)
		}
	}

	return nil
}

func tabnewWithFile(cx context.Context, wg *sync.WaitGroup, client nvimClient, file string, bufwg *sync.WaitGroup) error {
	p, err := filepath.Abs(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve file %s: %s\n", p, err)
		p = file
	}
	command := fmt.Sprintf("tabnew %s", p)
	if err := client.Command(command); err != nil {
		return fmt.Errorf("failed to open file %s: %s", p, err)
	}

	if err := registerCloseNotify(cx, wg, client, bufwg); err != nil {
		return err
	}

	return nil
}

func printVersionIfVersionExists(args []string) {
	for _, a := range args {
		if a == "--version" {
			fmt.Printf("renvim v0.0.0 -- Neovim wrapper.\n\n") // FIXME version
		}
	}
}

func run() error {
	val, present := os.LookupEnv("NVIM")
	args := os.Args[1:]

	if !present || val == "" || (len(args) > 0 && optonly(args)) {
		printVersionIfVersionExists(args)
		if err := execAsNvim(args, syscall.Exec); err != nil {
			return err
		}
	}

	client, err := nvim.Dial(val)
	if err != nil {
		return err
	}
	defer client.Close()

	if len(args) == 0 {
		args = []string{"-"}
	}

	// WaitGroup for context done.
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	cx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// WaitGroup for buffers done.
	bufwg := &sync.WaitGroup{}

	if err := prepareEnv(cx, wg, client, bufwg); err != nil {
		return err
	}

	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			fmt.Fprintf(os.Stderr, "Ignore option %s\n", a)
			continue
		}

		if a == "-" {
			if err := tabnew(cx, wg, client, os.Stdin, bufwg); err != nil {
				return err
			}

		} else {
			if err := tabnewWithFile(cx, wg, client, a, bufwg); err != nil {
				return err
			}
		}
	}

	go func() {
		bufwg.Wait()
		cancel()
	}()

	<-cx.Done()
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}
