package queue

import "github.com/valkey-io/valkey-go"

const (
	StreamTranscode      = "stream:transcode"
	StreamTranscodeDLQ   = "stream:transcode:dlq"
	ConsumerGroup        = "transcoder_group"
	HashTranscodeRetries = "hash:transcode:retries"
)

// QueueManager wraps the Valkey client to share connections.
type QueueManager struct {
	ValkeyClient valkey.Client
}
