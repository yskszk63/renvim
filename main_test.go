package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
			if err != nil || (err == nil && bin != bin) {
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

func TestBuffers(t *testing.T) {
	tests := []struct {
		add             []int
		done            []int
		wantsAddCalled  int
		wantsDoneCalled int
	}{
		{[]int{0}, []int{0}, 1, 1},
		{[]int{0}, []int{1}, 1, 0},
		{[]int{0, 0}, []int{0}, 1, 1},
		{[]int{0, 0}, []int{0, 0}, 1, 1},
	}

	for _, test := range tests {
		addCalled := 0
		doneCalled := 0
		b := buffers{
			buffers: make(map[int]bool),
			fnadd: func(n int) {
				if n != 1 {
					t.Fail()
				}
				addCalled += 1
			},
			fndone: func() {
				doneCalled += 1
			},
		}

		for _, a := range test.add {
			b.add(a)
		}
		for _, d := range test.done {
			b.done(d)
		}

		if !reflect.DeepEqual(test.wantsAddCalled, addCalled) {
			t.Fatalf("wants %v, but %v", test.wantsAddCalled, addCalled)
		}
		if test.wantsDoneCalled != doneCalled {
			t.Fatalf("wants %v, but %v", test.wantsDoneCalled, doneCalled)
		}
	}
}

type testClient struct {
	fnRegisterHandler func(string, interface{}) error
	fnSubscribe       func(string) error
	fnExec            func(string, bool) (string, error)
	fnCommand         func(string) error
	fnCurrentBuffer   func() (nvim.Buffer, error)
	fnSetBufferLines  func(nvim.Buffer, int, int, bool, [][]byte) error
}

func (t *testClient) RegisterHandler(method string, fn interface{}) error {
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
		{nil, nil, fmt.Errorf("err"), p("failed to register autocmd: err")},
	}

	for _, test := range tests {
		client := &testClient{
			fnRegisterHandler: func(method string, fn interface{}) error {
				return test.registerHandlerResult
			},
			fnSubscribe: func(event string) error {
				return test.subscribeResult
			},
			fnExec: func(src string, output bool) (string, error) {
				return "", test.execResult
			},
		}
		b := &buffers{}
		err := prepareEnv(client, b)
		if test.wants != nil && *test.wants != strings.TrimRight(err.Error(), "\n") {
			t.Fatalf("expect `%s` but `%s`", *test.wants, err)
		}
		if test.wants == nil && err != nil {
			t.Fatal(err)
		}
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

	buf, err := tabnew(client, pty)
	if err != nil {
		t.Fatal(err)
	}
	if *buf != nvim.Buffer(128) {
		t.Fatalf("not excepted: %v", buf)
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

	buf, err := tabnew(client, stdin.File)
	if err != nil {
		t.Fatal(err)
	}
	if *buf != nvim.Buffer(128) {
		t.Fatalf("not excepted: %v", buf)
	}
}
