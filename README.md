# go-check-color
## Install

```bash
go build -o go-check-color ./cmd/go-check-color
```
## Use

```bash
./go-check-color -in input.jpg -n 8
```

Flaggs:
- `-in` (string): path (png/jpg/gif)
- `-n` (int): color count (default 8)
- `-json` (bool): Out json
- `-preview` (string): Out pallete

Examples:

```bash
./go-check-color -in input.jpg -n 10

./go-check-color -in input.jpg -n 12 -json > palette.json

./go-check-color -in input.jpgphoto.png -n 8 -preview palette.png
```
