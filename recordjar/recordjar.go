// Copyright 2016 Andrew 'Diddymus' Rolfe. All rights reserved.
//
// Use of this source code is governed by the license in the LICENSE file
// included with the source code.

package recordjar

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"code.wolfmud.org/WolfMUD.git/text"
)

// Jar represents the collection of Records in a recordjar.
type Jar []Record

// Record represents the separate records in a recordjar.
type Record map[string][]byte

// splitLine is a regex to split fields and data in a recordjar .wrj file. The
// result of a FindSubmatch should always be a [][]byte of length 3 consisting
// of: the string matched, the field name, the data.
var splitLine = regexp.MustCompile(text.Uncomment(`
	^            # match start of string
	(?:          # non-capture group for 'field:'
	  \s*        # don't capture white space before 'field'
	  ([^\s:]+)  # capture 'field' - non-white-space/non-colon
	  :          # non-capture match of colon as field:value separator
	)?           # match non-captured 'field:' zero or once, prefer once
	\s*          # consume any whitepace - leading or after 'field:' if matched
	(.*?)        # capture everything left umatched, not greedy
	$            # match at end of string
`))

const (
	maxLineWidth = 78          // Maximum length of a line in a .wrj file
	FTSection    = "FREE TEXT" // Internal (due to space) freetext field for reading
)

var (
	comment       = []byte("//") // Comment marker
	rSeparator    = []byte("%%") // Record separator marker
	fSeparator    = []byte(": ") // Field/data separator as []byte
	fSeparatorLen = len(fSeparator)
	CR            = []byte("\r")
	LF            = []byte("\n")
	Empty         = []byte{}
	Space         = []byte(" ")
)

// Read takes as input an io.Reader - assuming the data to be in the WolfMUD
// recordjar format - and the field name to use for the free text section. The
// input is parsed into a jar which is then returned.
//
// For details of the recordjar format see the separate package documentation.
//
// BUG(diddymus): There is no provision for preserving comments.
func Read(in io.Reader, freetext string) (j Jar) {

	var (
		b   *bufio.Reader
		ok  bool
		err error

		// Variables for processing current line
		line    []byte   // current line from Reader
		startWS bool     // current line starts with whitespace before trimming?
		tokens  [][]byte // temp vars for name:data pair parsed from line
		name    string   // current name from line
		data    []byte   // current data from line
		field   string   // current field being processed (may differ from name)

		// Some flags to improve code readability
		noName = false // true if line has no name
		noData = false // true if line has no data
		noLine = false // true if line has no name and no data
	)

	// If not using a buffered Reader, make it buffered
	if b, ok = in.(*bufio.Reader); !ok {
		b = bufio.NewReader(in)
	}

	// Make sure the field name to use for free text section is uppercased
	freetext = strings.ToUpper(freetext)

	// Setup an initially empty record for the Jar
	r := Record{}

	// mergeFreeText is a helper for merging an actual, named freetext field
	// with an unnamed freetext section.
	mergeFreeText := func() {
		if _, ok = r[FTSection]; ok {
			if _, ok = r[freetext]; ok {
				r[freetext] = append(r[freetext], '\n')
				r[freetext] = append(r[freetext], r[FTSection]...)
			} else {
				r[freetext] = r[FTSection]
			}
			delete(r, FTSection)
		}
	}

	for err == nil {
		line, err = b.ReadBytes('\n')

		// If we read no data and find EOF continue and let loop exit
		if len(line) == 0 && err == io.EOF {
			continue
		}

		// Read and parse current line
		line = bytes.TrimRightFunc(line, unicode.IsSpace)
		startWS = bytes.IndexFunc(line, unicode.IsSpace) == 0
		tokens = splitLine.FindSubmatch(line)
		name, data = string(bytes.ToUpper(tokens[1])), tokens[2]

		noName = len(name) == 0
		noData = len(data) == 0
		noLine = noName && noData

		// Ignore comments found outside of free text section
		if noName && field != FTSection && bytes.HasPrefix(data, comment) {
			continue
		}

		// Handle record separator by recording current Record in Jar and setting
		// up a new next record, reset current field being processed. If a record
		// separator appears after a free text section there must be no leading
		// white-space before it otherwise it will be taken for free text.
		if noName && bytes.Equal(data, rSeparator) {
			if field != FTSection || (field == FTSection && !startWS) {
				if len(r) > 0 {
					mergeFreeText()
					j = append(j, r)
					r = Record{}
				}
				field = ""
				continue
			}
		}

		// If we get a new name and not inside a free text section then store new
		// name as the current field being processed
		if !noName && field != FTSection {
			field = name
		}

		// Switch to free text field if an empty line and we are not already
		// processing the free text section. If there was no current field being
		// processed we need to record the blank line so that it is included in the
		// free text section. This lets us have a record that has only a free text
		// section and can start with a blank line, which is not counted as a
		// separator line.
		if noLine && field != FTSection {
			if field == "" {
				r[FTSection] = []byte{}
			}
			field = FTSection
			continue
		}

		// Handle data as free text if already processing the free text section, or
		// we have no field - in which case assume we are starting a free text
		// section
		if field == FTSection || field == "" {
			if _, ok := r[FTSection]; ok {
				r[FTSection] = append(r[FTSection], '\n')
			}
			r[FTSection] = append(r[FTSection], line...)
			field = FTSection
			continue
		}

		// Handle field. Append a space before appending text if continuation
		if _, ok = r[field]; ok {
			r[field] = append(r[field], ' ')
		}
		r[field] = append(r[field], data...)
	}

	// Append last record to the Jar if we have one
	if len(r) > 0 {
		mergeFreeText()
		j = append(j, r)
		r = Record{}
	}

	return
}

// Write writes out a Record Jar to the specified io.Writer. The freetext
// string is used to specify which field name in a record should be used for
// the free text section. For example, if the freetext string is 'Description'
// then any fields named description in a record will be written out in the
// free text section.
//
// For details of the recordjar format see the separate package documentation.
//
// TODO(diddymus): Uppercase character after a hyphen in field names so that
// we can have 'On-Action', 'On-Reset', 'On-Cleanup' automatically.
//
// BUG(diddymus): There is no provision for writing out comments.
// BUG(diddymus): The empty field "" is invalid, currently dropped silently.
// BUG(diddymus): Unicode used in field names not normalised so 'Nаme' with a
// Cyrillic 'а' (U+0430) and 'Name' with a latin 'a' (U+0061) would be
// different fields.
// BUG: If a continuation line starts with ": " and we outdent it we don't
// refold lines even though we have two extra character positions available.
func (j Jar) Write(out io.Writer, freetext string) {

	var buf bytes.Buffer // Temporary buffer for current record

	// A slice of spaces we can re-slice to get variable lengths of padding
	padding := bytes.Repeat(Space, maxLineWidth-fSeparatorLen)

	// Normalise passed in field name for free text section
	freetext = text.TitleFirst(strings.ToLower(freetext))

	for _, rec := range j {

		norm := make(map[string][]byte, len(rec)) // Copy of rec, normalised keys
		keys := make([]string, 0, len(rec))       // List of sortable norm keys
		maxFieldLen := 0                          // Longest normalised field name

		// Copy fields from rec to norm but with normalised keys. As we go through
		// the field names note the length of the longest normalised field name.
		for field, data := range rec {

			if field == "" { // Ignore invalid empty field name
				continue
			}

			field = text.TitleFirst(strings.ToLower(field))
			norm[field], keys = data, append(keys, field)

			// Ignore field name for free text section as field name never written out
			if field == freetext {
				continue
			}

			if l := len(field); l > maxFieldLen {
				maxFieldLen = l
			}
		}

		// Write out fields for current record in the order given by the sorted keys
		sort.Strings(keys)
		for _, field := range keys {

			// Ignore the free text section field as it has to be written last
			if field == freetext {
				continue
			}

			// Fold the field data, which will now have network '\r\n' line endings.
			// Strip the '\r' to get Unix line endings. Finally split the data into
			// separate lines using `\n` as the delimiter.
			data := text.Fold(norm[field], maxLineWidth-maxFieldLen-fSeparatorLen)
			data = bytes.Replace(data, CR, Empty, -1)
			lines := bytes.Split(data, LF)

			// Write field name, separator, and first data line
			buf.Write(padding[0 : maxFieldLen-len(field)])
			buf.WriteString(field)
			buf.WriteByte(':')
			if len(lines[0]) != 0 {
				buf.Write(Space)
				buf.Write(lines[0])
			}
			buf.Write(LF)

			// Write continuation data lines. If a continuation line starts with ": "
			// then outdent it so that the colon lines up with the field name/data
			// separator.
			for _, l := range lines[1:] {
				if len(l) >= fSeparatorLen && bytes.Equal(l[0:2], fSeparator) {
					buf.Write(padding[0:maxFieldLen])
				} else {
					buf.Write(padding[0 : maxFieldLen+fSeparatorLen])
				}
				buf.Write(l)
				buf.Write(LF)
			}
		}

		// Write out the free text section, if we have one.
		if data, ok := norm[freetext]; ok {

			// Write separator line if record has a fields section
			if len(norm) > 1 {
				buf.Write(LF)
			}

			data = text.Fold(data, maxLineWidth)
			data = bytes.Replace(data, CR, Empty, -1)
			buf.Write(data)
			buf.Write(LF)
		}

		// If we have written any fields for the record, write a record separator.
		if len(norm) > 0 {
			buf.Write(rSeparator)
			buf.Write(LF)
		}
		buf.WriteTo(out)
	}
}
