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

	// Driver selects a durable storage for trace detail ("" = none,
	// "disk" = one <ULID>.json per record under Path).
	Driver string `yaml:"driver"`

	// Path is the storage directory for Driver, e.g.
	// /dev/shm/phpscript-trace-detail (tmpfs: survives restarts, no disk IO).
	Path string `yaml:"path"`

	// Sampling is the percentage (0-100) of completed traces written out to
	// the storage driver. The trace is always collected in memory; sampling
	// only gates the write. Values like 0.5 are valid.
	Sampling float64 `yaml:"sampling"`
}

// NewOptions returns the default status page options.
func NewOptions() Options {
	return Options{
		RingBufferSize: 100,
		TopRequests:    20,
		TrackMemoryUse: true,
		Sampling:       100,
	}
}
