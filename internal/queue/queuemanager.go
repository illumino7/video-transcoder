package queue

import "github.com/valkey-io/valkey-go"

const (
	StreamTranscode      = "stream:transcode"
	StreamTranscodeDLQ   = "stream:transcode:dlq"
	ConsumerGroup        = "transcoder_group"
	HashTranscodeRetries = "hash:transcode:retries"
)

type QueueManager struct {
	ValkeyClient valkey.Client
}
