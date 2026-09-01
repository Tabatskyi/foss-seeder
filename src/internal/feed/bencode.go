package feed

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
)

var (
	ErrInvalidBencode = errors.New("invalid bencode data")
	ErrNoInfoDict     = errors.New("no info dictionary found in torrent")
)

type bencodeParser struct {
	r *bufio.Reader
}

func parseBencode(r io.Reader) (any, error) {
	p := &bencodeParser{r: bufio.NewReader(r)}
	return p.parseValue()
}

func (p *bencodeParser) parseValue() (any, error) {
	b, err := p.r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 'i':
		return p.parseInt()
	case 'l':
		return p.parseList()
	case 'd':
		return p.parseDict()
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		_ = p.r.UnreadByte()
		return p.parseString()
	default:
		return nil, fmt.Errorf("%w: unexpected byte '%c'", ErrInvalidBencode, b)
	}
}

func (p *bencodeParser) parseInt() (int64, error) {
	var buf bytes.Buffer
	for {
		b, err := p.r.ReadByte()
		if err != nil {
			return 0, err
		}
		if b == 'e' {
			break
		}
		buf.WriteByte(b)
	}
	return strconv.ParseInt(buf.String(), 10, 64)
}

func (p *bencodeParser) parseString() (string, error) {
	var lenBuf bytes.Buffer
	for {
		b, err := p.r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == ':' {
			break
		}
		lenBuf.WriteByte(b)
	}
	length, err := strconv.Atoi(lenBuf.String())
	if err != nil || length < 0 {
		return "", fmt.Errorf("%w: invalid string length", ErrInvalidBencode)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func (p *bencodeParser) parseList() ([]any, error) {
	var list []any
	for {
		b, err := p.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 'e' {
			break
		}
		_ = p.r.UnreadByte()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		list = append(list, val)
	}
	return list, nil
}

func (p *bencodeParser) parseDict() (map[string]any, error) {
	dict := make(map[string]any)
	for {
		b, err := p.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 'e' {
			break
		}
		_ = p.r.UnreadByte()
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		dict[key] = val
	}
	return dict, nil
}

// ParseTorrentSize extracts the total payload size in bytes from .torrent metainfo.
func ParseTorrentSize(r io.Reader) (int64, error) {
	val, err := parseBencode(r)
	if err != nil {
		return 0, err
	}
	dict, ok := val.(map[string]any)
	if !ok {
		return 0, ErrInvalidBencode
	}

	infoRaw, ok := dict["info"]
	if !ok {
		return 0, ErrNoInfoDict
	}
	info, ok := infoRaw.(map[string]any)
	if !ok {
		return 0, ErrNoInfoDict
	}

	// 1. Single file: info["length"]
	if lengthRaw, ok := info["length"]; ok {
		if length, ok := lengthRaw.(int64); ok && length >= 0 {
			return length, nil
		}
	}

	// 2. Multi-file: info["files"] -> [ {"length": 123}, ... ]
	if filesRaw, ok := info["files"]; ok {
		if filesList, ok := filesRaw.([]any); ok {
			var total int64
			for _, f := range filesList {
				if fMap, ok := f.(map[string]any); ok {
					if lRaw, ok := fMap["length"]; ok {
						if l, ok := lRaw.(int64); ok && l > 0 {
							total += l
						}
					}
				}
			}
			return total, nil
		}
	}

	return 0, nil
}

// ParseTorrentSizeFromBytes parses the total size from a byte slice.
func ParseTorrentSizeFromBytes(data []byte) (int64, error) {
	return ParseTorrentSize(bytes.NewReader(data))
}
