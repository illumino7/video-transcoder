package queue

import "github.com/valkey-io/valkey-go"

const (
	StreamTranscode      = "stream:transcode"
	StreamTranscodeDLQ   = "stream:transcode:dlq"
	ConsumerGroup        = "transcoder_group"
	HashTranscodeRetries = "hash:transcode:retries"
)

// QueueManager bundles the Valkey client dependency to provide centralized access
// across HTTP handlers, async workers, and background janitor routines.
type QueueManager struct {
	ValkeyClient valkey.Client
}
