package nest2go

import (
	"regexp"
	"unicode/utf8"
)

var invalidFileNameCharReg = regexp.MustCompile("[^A-Za-z0-9_.]")
var invalidEndDotReg = regexp.MustCompile("[.]+$")

// preserve some space for timestamp and file extention
const maxFileLength = 230

func safeFileName(name string) string {
	// remove end dots
	name = invalidEndDotReg.ReplaceAllString(name, "")
	// truncate
	if len(name) > maxFileLength {
		for i, _ := range name {
			if i >= maxFileLength {
				name = name[:i]
				_, size := utf8.DecodeLastRuneInString(name)
				if size > 1 {
					name = name[:i-size]
				}
				break
			}
		}
	}
	name = invalidEndDotReg.ReplaceAllString(name, "-")
	return name
}
