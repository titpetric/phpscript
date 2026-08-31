package files

import (
	"io"
	"os"

	"github.com/titpetric/phpscript/runner"
)

// registerStreamReads installs the read half of the handle API. fopen() already
// returns the handle these take, and fwrite() and stream_get_contents() already
// take one, so what was missing was every way of reading a handle short of all
// of it at once.
//
// None of them buffers. A buffered reader would read further than the script
// asked for, which fseek() and ftell() would then disagree with, so fgets()
// walks the handle a byte at a time instead. That is one syscall per byte on a
// host file: reading a whole file and splitting it is cheaper here, and a
// script that can should.
func registerStreamReads(rt *runner.Runtime) {
	// The $whence values fseek takes. PHP numbers them 0, 1 and 2, and so does
	// Go's io package, which is why the argument is passed through as it
	// arrives rather than translated.
	rt.SetConst("SEEK_SET", int64(io.SeekStart))
	rt.SetConst("SEEK_CUR", int64(io.SeekCurrent))
	rt.SetConst("SEEK_END", int64(io.SeekEnd))

	// fread reads at most $length bytes from $stream and returns them, or false when the handle cannot be read; a read at the end of the handle returns the empty string, which is how a loop knows to stop.
	rt.RegisterFunc("fread", func(stream io.Reader, length int64) any {
		if stream == nil || length <= 0 {
			return false
		}
		buf := make([]byte, length)
		n, err := stream.Read(buf)
		if n == 0 && err != nil {
			if err == io.EOF {
				return ""
			}
			return false
		}
		return string(buf[:n])
	})

	// fgets reads one line from $stream, keeping the newline that ends it, and returns false at the end of the handle; $length bounds the line, so a line longer than it comes back in pieces.
	rt.RegisterFunc("fgets", func(stream io.Reader, length ...int64) any {
		if stream == nil {
			return false
		}
		limit := int64(-1)
		if len(length) > 0 && length[0] > 0 {
			// PHP's $length counts the terminating NUL it does not give a
			// script, so the line it reads is one shorter.
			limit = length[0] - 1
		}

		var line []byte
		one := make([]byte, 1)
		for limit < 0 || int64(len(line)) < limit {
			n, err := stream.Read(one)
			if n == 0 {
				if len(line) == 0 {
					return false
				}
				break
			}
			line = append(line, one[0])
			if one[0] == '\n' {
				break
			}
			if err != nil {
				break
			}
		}
		if len(line) == 0 {
			return false
		}
		return string(line)
	})

	// feof reports whether $stream is at its end. It answers for a handle fopen() gave out, by comparing where the handle sits against how long the file is; php://input and php://output are not seekable and answer false, so a script draining the request body should read it with stream_get_contents.
	rt.RegisterFunc("feof", func(stream any) bool {
		f, ok := stream.(*os.File)
		if !ok || f == nil {
			return false
		}
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return false
		}
		info, err := f.Stat()
		if err != nil {
			return false
		}
		return pos >= info.Size()
	})

	// fseek moves $stream to $offset, counted from the start of the handle, from where it sits when $whence is SEEK_CUR, or from the end when it is SEEK_END; it answers 0 on success and -1 on failure, which is the opposite way round from every other function here and is php's own choice.
	rt.RegisterFunc("fseek", func(stream io.Seeker, offset int64, whence ...int64) int64 {
		if stream == nil {
			return -1
		}
		from := io.SeekStart
		if len(whence) > 0 {
			from = int(whence[0])
		}
		if _, err := stream.Seek(offset, from); err != nil {
			return -1
		}
		return 0
	})

	// ftell returns where $stream sits, counted in bytes from the start, or false when the handle cannot say.
	rt.RegisterFunc("ftell", func(stream io.Seeker) any {
		if stream == nil {
			return false
		}
		pos, err := stream.Seek(0, io.SeekCurrent)
		if err != nil {
			return false
		}
		return pos
	})

	// rewind moves $stream back to its start and reports whether it could.
	rt.RegisterFunc("rewind", func(stream io.Seeker) bool {
		if stream == nil {
			return false
		}
		_, err := stream.Seek(0, io.SeekStart)
		return err == nil
	})
}
