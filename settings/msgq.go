package settings

import "fmt"

// SetMsgqQueueSizeOverride sets a process-local override without changing the
// persisted setting. In particular, loading settings again must not erase it.
func (s *MapdSettings) SetMsgqQueueSizeOverride(size int64) error {
	if size < 0 {
		return fmt.Errorf("msgq queue size must be 0 (auto) or a positive integer, got %d", size)
	}
	s.msgqQueueSizeOverride = &size
	return nil
}

// EffectiveMsgqQueueSize returns the configured reader capacity, or 0 for auto
// detection. An explicit override of 0 takes precedence over the saved setting.
func (s *MapdSettings) EffectiveMsgqQueueSize() (int64, error) {
	size := s.MsgqQueueSize
	if s.msgqQueueSizeOverride != nil {
		size = *s.msgqQueueSizeOverride
	}
	if size < 0 {
		return 0, fmt.Errorf("msgq_queue_size must be 0 (auto) or a positive integer, got %d", size)
	}
	return size, nil
}
