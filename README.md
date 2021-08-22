# renvim

[![Go Report Card](https://goreportcard.com/badge/github.com/yskszk63/renvim)](https://goreportcard.com/report/github.com/yskszk63/renvim)

Outside Neovim aware Neovim launcher.
Open the file in the outer Neovim instance.

## Demo

![demo](assets/demo.png)

## Why

I made it because I don't like the nested Neovim to start in Neovim's Terminal.

![nested](assets/nested.png)

## Example

Launch nvim and pass args If outside Neovim Terminal.

```bash
$ renvim file.txt
```

as Git commit message editor.

```bash
$ EDITOR=renvim git commit
```

.bashrc or .zsh alias

```bash
export EDITOR=renvim
alias nvim=$EDITOR
# or alias vi=$EDITOR
```

## License

Licensed under either of

 * Apache License, Version 2.0
   ([LICENSE-APACHE](LICENSE-APACHE) or http://www.apache.org/licenses/LICENSE-2.0)
 * MIT license
   ([LICENSE-MIT](LICENSE-MIT) or http://opensource.org/licenses/MIT)

at your option.

## Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted
for inclusion in the work by you, as defined in the Apache-2.0 license, shall be
dual licensed as above, without any additional terms or conditions.
