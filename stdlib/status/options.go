package status

// Options configures status page telemetry.
type Options struct {
	// RingBufferSize is the number of completed requests retained for the
	// request log and rolling statistics.
	RingBufferSize int `yaml:"ring_buffer_size"`

	// TopRequests is the maximum number of request groups returned in rolling
	// statistics.
	TopRequests int `yaml:"top_requests"`

	// TrackMemoryUse records process-wide allocation changes for each request.
	TrackMemoryUse bool `yaml:"track_memory_use"`
}

// NewOptions returns the default status page options.
func NewOptions() Options {
	return Options{
		RingBufferSize: 100,
		TopRequests:    20,
		TrackMemoryUse: true,
	}
}
