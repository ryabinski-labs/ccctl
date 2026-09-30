package session

import "regexp"

// terminalReply matches sequences the browser terminal sends on its own, not
// because the owner typed: focus in/out, cursor position and device attribute
// reports, status reports, mode reports, OSC color replies, and mouse reports.
var terminalReply = regexp.MustCompile(`^(?:` +
	`\x1b\[[IO]` + // focus in/out
	`|\x1b\[\??\d+(?:;\d+)*R` + // cursor position report
	`|\x1b\[[?>=]?[\d;]*c` + // device attributes
	`|\x1b\[\d*n` + // device status
	`|\x1b\[\??[\d;]*\$y` + // DECRQM reply
	`|\x1bP[^\x1b]*\x1b\\` + // DCS reply
	`|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC reply
	`|\x1b\[<\d+;\d+;\d+[Mm]` + // SGR mouse report
	`|\x1b\[M[\s\S]{3}` + // X10 mouse report
	`)+$`)

// IsTerminalReply reports whether data consists only of automatic terminal replies.
func IsTerminalReply(data []byte) bool { return len(data) > 0 && terminalReply.Match(data) }
