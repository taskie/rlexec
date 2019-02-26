# rlexec

## Usage

### Read lines and write them to the specified file

```sh
rlexec out.txt
```

FIFO special file can be the output file.

```sh
mkfifo fifo
uniq fifo
# and `rlexec fifo` in another terminal
```

### Use a history file

```sh
rlexec -H history.txt out.txt
```

## License

Apache License 2.0