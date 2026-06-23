package rt

import "unicode/utf8"

type keyScanResult int

const (
	keyReady keyScanResult = iota
	keyNeedMore
)

// scanKey classifies exactly one key token off the front of b and returns the
// token status plus the number of bytes to consume when the token is emitted.
//
// A single raw terminal read can carry several keys (held-key auto-repeat or
// queued input) or one multi-byte escape sequence (arrows, SGR mouse). The
// pre-tokenizer read-key returned the whole read as one string, so a burst
// like "llll" arrived as one unrecognized blob. Scanning lets read-key hand
// out one event per call: "llll" -> four "l", while "\x1b[C" stays one key.
//
// Recognized fronts:
//
//	ESC '[' … final(0x40-0x7E)   one CSI sequence (arrows, modified keys, SGR mouse)
//	ESC 'O' x                    one SS3 sequence (application-cursor keys, F1-F4)
//	ESC (alone / other)          the bare Escape key
//	UTF-8 lead byte              the whole rune (never split)
//	any other byte               that single byte (ASCII / control)
//
// A token split across two reads returns keyNeedMore with the bytes currently
// held for that front token. ReadKey refills first when more bytes are pending;
// otherwise it emits those bytes best-effort rather than stalling forever on a
// genuinely truncated sequence.
func scanKey(b []byte) (keyScanResult, int) {
	if len(b) == 0 {
		return keyReady, 0
	}
	if b[0] == 0x1b { // ESC
		if len(b) == 1 {
			return keyNeedMore, 1
		}
		switch b[1] {
		case '[': // CSI: ESC [ params/intermediates final
			i := 2
			for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
				i++
			}
			if i < len(b) {
				return keyReady, i + 1 // include the final byte
			}
			return keyNeedMore, len(b)
		case 'O': // SS3: ESC O x
			if len(b) < 3 {
				return keyNeedMore, len(b)
			}
			return keyReady, 3
		}
		return keyReady, 1 // bare ESC
	}
	if b[0] < utf8.RuneSelf { // single-byte ASCII / control
		return keyReady, 1
	}
	if !utf8.FullRune(b) {
		return keyNeedMore, len(b)
	}
	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError && size == 1 {
		return keyReady, 1 // invalid byte — emit as-is, never stall
	}
	return keyReady, size
}

// nextKey splits exactly one key token off the front of b and returns the token
// plus the number of bytes consumed. If b contains a partial front token, it
// emits the held bytes best-effort; ReadKey normally avoids that by refilling
// when scanKey reports keyNeedMore and stdin has more bytes pending.
func nextKey(b []byte) (string, int) {
	_, n := scanKey(b)
	if n == 0 {
		return "", 0
	}
	return string(b[:n]), n
}
