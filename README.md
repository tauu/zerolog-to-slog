# zerolog-to-slog converter

**NOTE** The mayority of the code of this project was written by an LLM.

This tool reads golang source code and converts calls to zerlog into equivalent calls to slog. E.g.
```golang
log.Error().Err(err).Str("opeartion", operation).Msg("the operation failed")
```
Would be converted to
```golang
slog.LogAttrs(ctx, slog.LevelError, "the operation failed", slog.Any("err",err), slog.String("operation", operation))
```

## Installation
Run the following command to install the tool.
```sh
go install github.com/tauu/zerolog-to-slog
```

## Usage
Specify the directory in which the .go files which are to be converted from zerolog to slog reside and run the program. This will only print the converted code to stdout without writing any modifications.
```sh
zerolog-to-slog --dir ./program
```
To replace the code in place add the `--replace` flag.

**WARNING** This operation will not ask for confirmation to replace files. Backup your code before running this command.
```sh
zerolog-to-slog --dir ./program --replace
```