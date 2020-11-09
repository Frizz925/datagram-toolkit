package udp

import "log"

const (
	defaultStreamWindowSize = 65535
	defaultStreamBufferSize = 2048
	defaultStreamBacklog    = 5

	minStreamFrameSize  = 32
	minStreamWindowSize = 512
	minStreamBufferSize = 512
)

type StreamConfig struct {
	WindowSize int

	// Peer configurations.
	// If not defined, these configurations would later be set through a handshake mechanism.
	PeerConfig StreamPeerConfig

	// Optional logger for debugging purposes
	Logger *log.Logger

	ReadBufferSize int
	ReadBacklog    int

	WriteBufferSize int
	WriteBacklog    int
}

type StreamPeerConfig struct {
	FrameSize  int
	WindowSize int
}

func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		WindowSize:      defaultStreamWindowSize,
		ReadBufferSize:  defaultStreamBufferSize,
		ReadBacklog:     defaultStreamBacklog,
		WriteBufferSize: defaultStreamBufferSize,
		WriteBacklog:    defaultStreamBacklog,
		Logger:          discardLogger,
	}
}

func sanitizeStreamConfig(cfg StreamConfig) StreamConfig {
	if cfg.PeerConfig.FrameSize < 0 {
		cfg.PeerConfig.FrameSize = 0
	}
	if cfg.PeerConfig.WindowSize < 0 {
		cfg.PeerConfig.WindowSize = 0
	}
	if cfg.WindowSize < minStreamWindowSize {
		cfg.WindowSize = minStreamWindowSize
	}
	if cfg.ReadBufferSize < minStreamBufferSize {
		cfg.ReadBufferSize = minStreamBufferSize
	}
	if cfg.ReadBacklog < 0 {
		cfg.ReadBacklog = 0
	}
	if cfg.WriteBufferSize < minStreamBufferSize {
		cfg.WriteBufferSize = minStreamBufferSize
	}
	if cfg.WriteBacklog < 0 {
		cfg.WriteBacklog = 0
	}
	if cfg.Logger == nil {
		cfg.Logger = discardLogger
	}
	return cfg
}
