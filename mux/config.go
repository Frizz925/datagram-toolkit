package mux

const (
	defaultAcceptBacklog     = 5
	defaultInitialBufferSize = 4096
	defaultReadBufferSize    = 65535
	defaultReadBacklog       = 1024
	defaultWriteBacklog      = 1024

	minBufferSize = 512
)

func DefaultConfig() Config {
	return Config{
		AcceptBacklog:     defaultAcceptBacklog,
		InitialBufferSize: defaultInitialBufferSize,
		ReadBufferSize:    defaultReadBufferSize,
		ReadBacklog:       defaultReadBacklog,
		WriteBacklog:      defaultWriteBacklog,
	}
}

func sanitizeConfig(cfg Config) Config {
	if cfg.AcceptBacklog < 0 {
		cfg.AcceptBacklog = 0
	}
	if cfg.InitialBufferSize < minBufferSize {
		cfg.InitialBufferSize = minBufferSize
	}
	if cfg.ReadBufferSize < minBufferSize {
		cfg.InitialBufferSize = minBufferSize
	}
	if cfg.ReadBacklog < 0 {
		cfg.ReadBacklog = 0
	}
	if cfg.WriteBacklog < 0 {
		cfg.WriteBacklog = 0
	}
	return cfg
}
