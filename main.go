package main

import (
    "fmt"
    "net"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "syscall"

    "github.com/vmihailenco/msgpack/v5"
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

    conn, err := net.Dial("unix", val)
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to connect %s: %s\n", val, err)
        os.Exit(-1)
    }
    defer conn.Close()

    enc := msgpack.NewEncoder(conn)
    dec := msgpack.NewDecoder(conn)

    tabnew := func(file *string) error {
        // msgpack-rpc request
        // [type, msgid, method, params]

        if err := enc.EncodeArrayLen(4); err != nil {
            return err
        }
        if err := enc.EncodeUint8(0); err != nil {
            return err
        }
        if err := enc.EncodeUint32(0); err != nil {
            return err
        }
        if err := enc.EncodeString("nvim_command"); err != nil {
            return err
        }

        var command string
        if file != nil {
            command = fmt.Sprintf("tabnew %s", *file)
        } else {
            command = "tabnew"
        }

        if err := enc.EncodeArrayLen(1); err != nil {
            return err
        }
        if err := enc.EncodeString(command); err != nil {
            return err
        }

        return nil
    }

    recv := func() error {
        // msgpack-rpc response
        // [type, msgid, error, result]

        alen, err := dec.DecodeArrayLen()
        if err != nil {
            return err
        }
        if alen != 4 {
            return fmt.Errorf("unexpected array len %d", alen)
        }

        _, err = dec.DecodeInt()
        if err != nil {
            return err
        }

        _, err = dec.DecodeInt()
        if err != nil {
            return nil
        }

        arglen, err := dec.DecodeArrayLen()
        if err != nil {
            return err
        }
        if arglen != -1 {
            return fmt.Errorf("unexpected array len %d", arglen)
        }

        return nil
    }

    if len(args) == 0 {
        if err := tabnew(nil); err != nil {
            fmt.Fprintf(os.Stderr, "failed to open file %s: %s\n", val, err)
        }
        if err := recv(); err != nil {
            fmt.Fprintf(os.Stderr, "failed to open file %s: %s\n", val, err)
        }

    } else {
        for _, a := range args {
            if strings.HasPrefix(a, "--") {
                fmt.Fprintf(os.Stderr, "Ignore option %s\n", a)
            }

            p, err := filepath.Abs(a)
            if err != nil {
                fmt.Fprintf(os.Stderr, "failed to resolve file %s: %s\n", val, err)
                p = a
            }
            if err := tabnew(&p); err != nil {
                fmt.Fprintf(os.Stderr, "failed to open file %s: %s\n", val, err)
            }
            if err := recv(); err != nil {
                fmt.Fprintf(os.Stderr, "failed to open file %s: %s\n", val, err)
            }
        }
    }
}
