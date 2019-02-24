# rltee

## Usage

### Read lines and write them to the specified file

```sh
rltee out.txt
```

FIFO special file can be the output file.

```sh
mkfifo fifo
uniq fifo
# and `rltee fifo` in another terminal
```

### Use a history file

```sh
rltee -H history.txt out.txt
```

## License

Apache License 2.0