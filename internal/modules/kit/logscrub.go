package kit

import (
	"bufio"
	"io"
	"regexp"
)

// Log scrubbing (ADR-003): upstream logs phone numbers and names at info level
// ("Received message from=…", "profile_name=…", "phone=…"). Weline can't
// change upstream's logger, so it masks those values in the process's output
// before the hosting provider stores them.

var (
	// key=value pairs whose value identifies a person. The key may be wrapped
	// in colour codes (development output).
	scrubKeyRe = regexp.MustCompile(`((?:^|[\s"])(?:\x1b\[[0-9;]*m)?(?:phone|from|to|wa_id|waid|profile_name|contact_name|contact_phone|recipient|recipient_name|recipient_phone|mobile|user_phone|customer_phone)(?:\x1b\[[0-9;]*m)?=)("(?:[^"\\]|\\.)*"|\S+)`)
	// Anything else that looks like a phone number: 9 to 15 digits, optionally
	// after a +, not part of a longer word, id or timestamp.
	scrubDigitsRe = regexp.MustCompile(`(^|[^0-9A-Za-z\-_:./])(\+?\d{9,15})($|[^0-9A-Za-z\-_:])`)
)

// ScrubLine masks personal identifiers in one log line.
func ScrubLine(line []byte) []byte {
	out := scrubKeyRe.ReplaceAll(line, []byte(`${1}[redacted]`))
	// Two passes so adjacent numbers separated by one character are both caught.
	for i := 0; i < 2; i++ {
		out = scrubDigitsRe.ReplaceAllFunc(out, func(m []byte) []byte {
			sub := scrubDigitsRe.FindSubmatch(m)
			d := sub[2]
			masked := make([]byte, 0, len(d))
			for j, c := range d {
				if c >= '0' && c <= '9' && j < len(d)-3 {
					c = '*'
				}
				masked = append(masked, c)
			}
			return append(append(append([]byte{}, sub[1]...), masked...), sub[3]...)
		})
	}
	return out
}

// ScrubCopy copies r to w line by line, scrubbing each line. Lines longer
// than 1 MB are passed through in pieces, each scrubbed.
func ScrubCopy(w io.Writer, r io.Reader) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadSlice('\n')
		if len(line) > 0 {
			if _, werr := w.Write(ScrubLine(line)); werr != nil {
				return werr
			}
		}
		switch err {
		case nil, bufio.ErrBufferFull:
			continue
		case io.EOF:
			return nil
		default:
			return err
		}
	}
}
