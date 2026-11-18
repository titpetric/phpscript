package files

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

// registerCSV installs fputcsv and fgetcsv over encoding/csv, ignoring PHP's $escape.
func registerCSV(rt *runner.Runtime) {
	// fputcsv writes $fields to $stream as one RFC 4180 record ending in \n
	// and returns the number of bytes written, or false on failure; it ignores
	// $escape and refuses an $enclosure other than '"'.
	rt.RegisterFunc("fputcsv", func(stream io.Writer, fields any, opts ...any) (any, error) {
		sep, err := csvControls("fputcsv", opts)
		if err != nil {
			return false, err
		}
		record := make([]string, 0, 8)
		modelutil.RangeValues(fields, func(_, val any) bool {
			record = append(record, phpval.String(val))
			return true
		})

		counter := &countingWriter{w: stream}
		w := csv.NewWriter(counter)
		w.Comma = sep
		if err := w.Write(record); err != nil {
			return false, nil
		}
		w.Flush()
		if w.Error() != nil {
			return false, nil
		}
		return counter.n, nil
	})

	// fgetcsv reads one CSV record from $stream and returns its fields as
	// strings, or false at end of file; it ignores $length and $escape, and
	// refuses an $enclosure other than '"'.
	rt.RegisterFunc("fgetcsv", func(stream io.Reader, opts ...any) (any, error) {
		// The first optional argument is $length, a line-length hint PHP
		// itself stopped needing; the separator and its companions follow.
		controls := opts
		if len(controls) > 0 {
			controls = controls[1:]
		}
		sep, err := csvControls("fgetcsv", controls)
		if err != nil {
			return false, err
		}

		r := csv.NewReader(byteReader{r: stream})
		r.Comma = sep
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		record, err := r.Read()
		if err != nil {
			// End of file, or a record too malformed to parse: both are
			// false and PHP's read loop terminates.
			return false, nil
		}

		fields := model.NewArray()
		for _, field := range record {
			fields.Append(field)
		}
		return fields, nil
	})
}

// csvControls reads the separator tail, refusing an enclosure other than '"'.
func csvControls(fn string, opts []any) (rune, error) {
	sep := ','
	if len(opts) > 0 && opts[0] != nil {
		s, ok := opts[0].(string)
		runes := []rune(s)
		if !ok || len(runes) != 1 {
			return 0, fmt.Errorf("%s(): $separator must be a single character", fn)
		}
		sep = runes[0]
	}
	if len(opts) > 1 && opts[1] != nil {
		if enc, ok := opts[1].(string); !ok || enc != `"` {
			return 0, fmt.Errorf(`%s(): only the default enclosure '"' is implemented`, fn)
		}
	}
	return sep, nil
}

// countingWriter counts the bytes written to w.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// byteReader passes encoding/csv one byte per Read; a record cannot overrun.
type byteReader struct {
	r io.Reader
}

func (b byteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return b.r.Read(p)
}
