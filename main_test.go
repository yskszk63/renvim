package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/creack/pty"
	"github.com/justincormack/go-memfd"
	"github.com/neovim/go-client/nvim"
)

func p(s string) *string {
	return &s
}

func TestOptonly(t *testing.T) {
	tests := []struct {
		input []string
		want  bool
	}{
		{[]string{}, true},
		{[]string{"--version"}, true},
		{[]string{"file.txt"}, false},
		{[]string{"file.txt", "--version"}, false},
		{[]string{"--version", "--version"}, true},
	}

	for _, test := range tests {
		got := optonly(test.input)
		if got != test.want {
			t.Fatalf("want %v for %v, but %v", test.want, test.input, got)
		}
	}
}

func TestExecAsNvim(t *testing.T) {
	pass := func(string, []string, []string) error { return nil }
	fail := func(string, []string, []string) error { return fmt.Errorf("err") }

	tests := []struct {
		path string
		args []string
		exec func(string, []string, []string) error
		err  *string
	}{
		{"testdata/nvim_exists", []string{}, pass, nil},
		{"testdata/no_nvim_exists", []string{}, pass, p(`no nvim found: exec: "nvim": executable file not found in $PATH`)},
		{"testdata/nvim_exists", []string{}, fail, p("failed to exec nvim: err")},
		{"testdata/nvim_exists", []string{"foo", "bar"}, func(bin string, args, env []string) error {
			bin, err := filepath.Abs("testdata/nvim_exists/nvim")
			if err != nil || bin != bin {
				t.Fail()
			}

			if !reflect.DeepEqual(args, []string{bin, "foo", "bar"}) {
				t.Fail()
			}

			return nil
		}, nil},
	}

	for _, test := range tests {
		oldPath := os.Getenv("PATH")
		defer func() {
			os.Setenv("PATH", oldPath)
		}()

		path, err := filepath.Abs(test.path)
		if err != nil {
			t.Fatal(err)
		}
		os.Setenv("PATH", path)

		err = execAsNvim(test.args, test.exec)
		if test.err != nil && *test.err != strings.TrimRight(err.Error(), "\n") {
			t.Fatalf("expect `%s` but `%s`", *test.err, err)
		}
		if test.err == nil && err != nil {
			t.Fatal(err)
		}
	}
}

type testClient struct {
	fnRegisterHandler func(string, any) error
	fnSubscribe       func(string) error
	fnExec            func(string, bool) (string, error)
	fnCommand         func(string) error
	fnCurrentBuffer   func() (nvim.Buffer, error)
	fnSetBufferLines  func(nvim.Buffer, int, int, bool, [][]byte) error
}

func (t *testClient) RegisterHandler(method string, fn any) error {
	return t.fnRegisterHandler(method, fn)
}
func (t *testClient) Subscribe(event string) error {
	return t.fnSubscribe(event)
}
func (t *testClient) Exec(src string, output bool) (string, error) {
	return t.fnExec(src, output)
}
func (t *testClient) Command(command string) error {
	return t.fnCommand(command)
}
func (t *testClient) CurrentBuffer() (nvim.Buffer, error) {
	return t.fnCurrentBuffer()
}
func (t *testClient) SetBufferLines(buf nvim.Buffer, start, end int, strict bool, lines [][]byte) error {
	return t.SetBufferLines(buf, start, end, strict, lines)
}
func (t *testClient) ChannelID() int {
	return 3
}
func (t *testClient) ExecLua(code string, result any, args ...any) error {
	return nil
}
func (t *testClient) Unsubscribe(event string) error {
	return nil
}

func TestPrepareEnv(t *testing.T) {
	tests := []struct {
		registerHandlerResult error
		subscribeResult       error
		execResult            error
		wants                 *string
	}{
		{nil, nil, nil, nil},
		{fmt.Errorf("err"), nil, nil, p("failed to register handler: err")},
		{nil, fmt.Errorf("err"), nil, p("failed to subscribe: err")},
	}

	for i, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			wg := &sync.WaitGroup{}
			bufwg := &sync.WaitGroup{}

			client := &testClient{
				fnRegisterHandler: func(method string, fn any) error {
					return test.registerHandlerResult
				},
				fnSubscribe: func(event string) error {
					return test.subscribeResult
				},
			}
			err := prepareEnv(t.Context(), wg, client, bufwg)
			if test.wants != nil && *test.wants != strings.TrimRight(err.Error(), "\n") {
				t.Fatalf("expect `%s` but `%s`", *test.wants, err)
			}
			if test.wants == nil && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTabnewWithPty(t *testing.T) {
	pty, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer pty.Close()
	defer tty.Close()

	client := &testClient{
		fnCommand: func(command string) error {
			if command != "tabnew" {
				t.Fatalf("unexpected command %s", command)
			}
			return nil
		},
		fnCurrentBuffer: func() (nvim.Buffer, error) {
			return nvim.Buffer(128), nil
		},
		fnSetBufferLines: func(buf nvim.Buffer, start int, end int, strict bool, lines [][]byte) error {
			t.Fatal("must not be called")
			return nil
		},
	}

	wg := &sync.WaitGroup{}
	bufwg := &sync.WaitGroup{}

	if err := tabnew(t.Context(), wg, client, pty, bufwg); err != nil {
		t.Fatal(err)
	}
}

func TestTabnewWithoutPty(t *testing.T) {
	stdin, err := memfd.Create()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()

	client := &testClient{
		fnCommand: func(command string) error {
			return nil
		},
		fnCurrentBuffer: func() (nvim.Buffer, error) {
			return nvim.Buffer(128), nil
		},
		fnSetBufferLines: func(buf nvim.Buffer, start int, end int, strict bool, lines [][]byte) error {
			return nil
		},
	}

	wg := &sync.WaitGroup{}
	bufwg := &sync.WaitGroup{}

	if err := tabnew(t.Context(), wg, client, stdin.File, bufwg); err != nil {
		t.Fatal(err)
	}
}
