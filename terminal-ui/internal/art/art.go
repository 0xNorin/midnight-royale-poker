// Package art reads inert SGR-only artwork. It never emits asset cursor commands.
package art

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Art struct {
	Width int
	Rows  []string
}

// Parse makes every row independently renderable, including inherited styles.
func Parse(raw string) (*Art, error) {
	if !utf8.ValidString(raw) {
		return nil, fmt.Errorf("art is not UTF-8")
	}
	a := &Art{}
	fg, bg := "", ""
	attrs := map[int]bool{}
	style := func() string {
		s := "\x1b[0m"
		for _, n := range []int{1, 2, 3, 4, 7, 9} {
			if attrs[n] {
				s += fmt.Sprintf("\x1b[%dm", n)
			}
		}
		return s + fg + bg
	}
	var row strings.Builder
	row.WriteString(style())
	flush := func() error {
		s := row.String()
		w := ansi.StringWidth(s)
		if len(a.Rows) == 0 {
			a.Width = w
		}
		if w != a.Width {
			return fmt.Errorf("ragged art row %d: %d != %d", len(a.Rows)+1, w, a.Width)
		}
		a.Rows = append(a.Rows, s+"\x1b[0m")
		row.Reset()
		row.WriteString(style())
		return nil
	}
	for i := 0; i < len(raw); {
		if raw[i] == '\n' {
			if e := flush(); e != nil {
				return nil, e
			}
			i++
			continue
		}
		if raw[i] == 27 {
			if i+1 >= len(raw) || raw[i+1] != '[' {
				return nil, fmt.Errorf("unsupported escape at byte %d", i)
			}
			j := i + 2
			for j < len(raw) && (raw[j] == ';' || raw[j] >= '0' && raw[j] <= '9') {
				j++
			}
			if j >= len(raw) || raw[j] != 'm' {
				return nil, fmt.Errorf("only SGR styling is allowed at byte %d", i)
			}
			p := raw[i+2 : j]
			if p == "" {
				p = "0"
			}
			parts := strings.Split(p, ";")
			nums := make([]int, len(parts))
			for k, s := range parts {
				n, e := strconv.Atoi(s)
				if e != nil {
					return nil, fmt.Errorf("invalid SGR")
				}
				nums[k] = n
			}
			for k := 0; k < len(nums); k++ {
				n := nums[k]
				switch {
				case n == 0:
					fg = ""
					bg = ""
					attrs = map[int]bool{}
				case n == 1 || n == 2 || n == 3 || n == 4 || n == 7 || n == 9:
					attrs[n] = true
				case n == 22:
					delete(attrs, 1)
					delete(attrs, 2)
				case n == 23:
					delete(attrs, 3)
				case n == 24:
					delete(attrs, 4)
				case n == 27:
					delete(attrs, 7)
				case n == 29:
					delete(attrs, 9)
				case n == 39:
					fg = ""
				case n == 49:
					bg = ""
				case n == 38 || n == 48:
					end := k + 1
					if end >= len(nums) {
						return nil, fmt.Errorf("incomplete RGB")
					}
					if nums[end] == 2 {
						end = k + 4
					} else if nums[end] == 5 {
						end = k + 2
					} else {
						return nil, fmt.Errorf("unsupported color mode")
					}
					if end >= len(nums) {
						return nil, fmt.Errorf("incomplete color")
					}
					for _, v := range nums[k+2 : end+1] {
						if v < 0 || v > 255 {
							return nil, fmt.Errorf("color out of range")
						}
					}
					s := "\x1b[" + strings.Join(parts[k:end+1], ";") + "m"
					if n == 38 {
						fg = s
					} else {
						bg = s
					}
					k = end
				case n >= 30 && n <= 37 || n >= 90 && n <= 97:
					fg = fmt.Sprintf("\x1b[%dm", n)
				case n >= 40 && n <= 47 || n >= 100 && n <= 107:
					bg = fmt.Sprintf("\x1b[%dm", n)
				default:
					return nil, fmt.Errorf("unsupported SGR %d", n)
				}
			}
			row.WriteString(raw[i : j+1])
			i = j + 1
			continue
		}
		r, n := utf8.DecodeRuneInString(raw[i:])
		if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 {
			return nil, fmt.Errorf("control U+%04X not allowed", r)
		}
		row.WriteRune(r)
		i += n
	}
	if !strings.HasSuffix(raw, "\n") && raw != "" {
		if e := flush(); e != nil {
			return nil, e
		}
	}
	if a.Width == 0 || len(a.Rows) == 0 {
		return nil, fmt.Errorf("empty artwork")
	}
	return a, nil
}
