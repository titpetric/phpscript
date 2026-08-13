package model

// Status describes the current phase of a Runtime. The one-character values
// follow the scoreboard convention used by servers such as lighttpd.
type Status string

const (
	StatusWaiting    Status = "_"
	StatusStarting   Status = "s"
	StatusReading    Status = "R"
	StatusProcessing Status = "P"
	StatusWriting    Status = "W"
	StatusKeepalive  Status = "K"
	StatusClosing    Status = "C"
	StatusError      Status = "E"
)
