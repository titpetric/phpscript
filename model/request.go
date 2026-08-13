package model

import (
	"html/template"
	"runtime"
	"time"
)

// Request describes an active or recently completed request.
type Request struct {
	ID             string        `json:"request_id"`
	Status         Status        `json:"status"`
	Request        string        `json:"request"`
	Hostname       string        `json:"hostname"`
	Filename       string        `json:"filename,omitempty"`
	IncludedFiles  int           `json:"included_files"`
	Method         string        `json:"method"`
	URI            string        `json:"uri"`
	Protocol       string        `json:"protocol"`
	RemoteAddress  string        `json:"remote_address"`
	UserAgent      string        `json:"user_agent,omitempty"`
	StartedAt      time.Time     `json:"started_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Duration       time.Duration `json:"duration_ns"`
	ResponseStatus int           `json:"response_status,omitempty"`
	ResponseBytes  int64         `json:"response_bytes"`
	HeapDelta      int64         `json:"heap_delta_bytes"`
	AllocatedBytes uint64        `json:"allocated_bytes"`
	Allocations    uint64        `json:"allocations"`
	GCCycles       uint32        `json:"gc_cycles"`
	GCPause        time.Duration `json:"gc_pause_ns"`
	Spans          []RequestSpan `json:"spans,omitempty"`

	MemStats  runtime.MemStats `json:"-"`
	ChangedAt time.Time        `json:"-"`
}

func (r *Request) AppendSpan(at time.Time, message string, flags ...Flag) {
	span := RequestSpan{
		ID:      len(r.Spans) + 1,
		Time:    at,
		Type:    SpanType.Internal,
		Message: template.HTML(message),
	}
	for _, flag := range flags {
		switch flag {
		case OpenSpan:
			span.Open = true
		case CloseSpan:
			span.Close = true
		case "":
		default:
			span.Type = flag
		}
	}
	r.Spans = append(r.Spans, span)
}

// RequestStatistic aggregates one method and URI in the rolling window.
type RequestStatistic struct {
	Request               string        `json:"request"`
	Hostname              string        `json:"hostname"`
	Filename              string        `json:"filename,omitempty"`
	AverageIncludedFiles  float64       `json:"average_included_files"`
	Count                 uint64        `json:"count"`
	Share                 float64       `json:"share_percent"`
	AverageDuration       time.Duration `json:"average_duration_ns"`
	AverageResponseBytes  uint64        `json:"average_response_bytes"`
	AverageAllocatedBytes uint64        `json:"average_allocated_bytes"`

	TotalDuration      time.Duration `json:"-"`
	TotalResponseBytes uint64        `json:"-"`
	TotalAllocated     uint64        `json:"-"`
	TotalIncluded      uint64        `json:"-"`
}
